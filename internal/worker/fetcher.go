package worker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ingest"
)

// PRIssueFetcher abstracts retrieval of pull requests and issues.
type PRIssueFetcher interface {
	FetchPullRequests(ctx context.Context, owner, repo string, limit int) ([]*ingest.ScannedFile, error)
	FetchIssues(ctx context.Context, owner, repo string, limit int) ([]*ingest.ScannedFile, error)
}

// GitHubFileFetcher retrieves repository files via GitHub archive tarballs or shallow git clone.
type GitHubFileFetcher struct {
	patToken       string
	httpClient     *http.Client
	filter         *ingest.FileFilter
	logger         *zap.Logger
	prIssueFetcher PRIssueFetcher
	tarballURLFunc func(owner, repo, branch string) []string
}

// NewGitHubFileFetcher constructs a new GitHubFileFetcher.
func NewGitHubFileFetcher(patToken string, logger *zap.Logger) *GitHubFileFetcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	trimmedToken := strings.TrimSpace(patToken)
	return &GitHubFileFetcher{
		patToken: trimmedToken,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
		filter:         ingest.NewFileFilter(ingest.DefaultScannerOptions()),
		logger:         logger,
		prIssueFetcher: ingest.NewGitHubPRIssueFetcher(trimmedToken, logger),
	}
}

// SetPRIssueFetcher sets a custom PRIssueFetcher (useful for testing or customized fetching).
func (f *GitHubFileFetcher) SetPRIssueFetcher(fetcher PRIssueFetcher) {
	f.prIssueFetcher = fetcher
}

// SetTarballURLFunc sets a custom URL resolver for archive tarballs (primarily for testing).
func (f *GitHubFileFetcher) SetTarballURLFunc(fn func(owner, repo, branch string) []string) {
	f.tarballURLFunc = fn
}

// SetHTTPClient sets a custom HTTP client.
func (f *GitHubFileFetcher) SetHTTPClient(client *http.Client) {
	if client != nil {
		f.httpClient = client
	}
}

// FetchFiles discovers and retrieves all indexable source files for the given repository source.
func (f *GitHubFileFetcher) FetchFiles(ctx context.Context, source *ent.Source) ([]*ingest.ScannedFile, error) {
	if source == nil {
		return nil, fmt.Errorf("source cannot be nil")
	}

	owner := strings.TrimSpace(source.RepoOwner)
	repo := strings.TrimSpace(source.RepoName)

	if owner == "" || repo == "" {
		parts := strings.Split(strings.TrimSpace(source.Name), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			owner = parts[0]
			repo = parts[1]
		}
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("source %s missing repo_owner or repo_name", source.ID)
	}

	branch := strings.TrimSpace(source.Branch)
	if branch == "" {
		branch = "main"
	}

	f.logger.Info("fetching repository files",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.String("branch", branch),
	)

	var files []*ingest.ScannedFile

	// Strategy 1: Attempt fast HTTP tarball extraction (zero external CLI dependencies)
	tbFiles, err := f.fetchViaTarball(ctx, owner, repo, branch)
	if err == nil && len(tbFiles) > 0 {
		f.logger.Info("successfully fetched repository via tarball archive",
			zap.String("repo", owner+"/"+repo),
			zap.Int("files_count", len(tbFiles)),
		)
		files = tbFiles
	} else if branch == "main" {
		// If branch was "main" and failed, also try "master"
		tbMasterFiles, errMaster := f.fetchViaTarball(ctx, owner, repo, "master")
		if errMaster == nil && len(tbMasterFiles) > 0 {
			f.logger.Info("successfully fetched repository via master branch tarball archive",
				zap.String("repo", owner+"/"+repo),
				zap.Int("files_count", len(tbMasterFiles)),
			)
			files = tbMasterFiles
		}
	}

	if len(files) == 0 {
		if err != nil {
			f.logger.Warn("tarball fetch unsuccessful, attempting git clone fallback",
				zap.String("repo", owner+"/"+repo),
				zap.Error(err),
			)
		}

		// Strategy 2: Fallback to local git shallow clone if git binary is available
		cloneFiles, cloneErr := f.fetchViaGitClone(ctx, owner, repo, branch)
		if cloneErr == nil && len(cloneFiles) > 0 {
			f.logger.Info("successfully fetched repository via git clone",
				zap.String("repo", owner+"/"+repo),
				zap.Int("files_count", len(cloneFiles)),
			)
			files = cloneFiles
		} else {
			if cloneErr != nil {
				f.logger.Error("git clone fallback failed",
					zap.String("repo", owner+"/"+repo),
					zap.Error(cloneErr),
				)
			}
			return nil, fmt.Errorf("failed to fetch repository files: tarball error: %v, clone error: %v", err, cloneErr)
		}
	}

	// Ingest Pull Requests and Issues (up to 30 items each) and append to files list
	if f.prIssueFetcher != nil {
		prFiles, prErr := f.prIssueFetcher.FetchPullRequests(ctx, owner, repo, 30)
		if prErr != nil {
			f.logger.Warn("failed to fetch pull requests, continuing without PRs",
				zap.String("repo", owner+"/"+repo),
				zap.Error(prErr),
			)
		} else if len(prFiles) > 0 {
			f.logger.Info("successfully fetched pull requests",
				zap.String("repo", owner+"/"+repo),
				zap.Int("prs_count", len(prFiles)),
			)
			files = append(files, prFiles...)
		}

		issueFiles, issueErr := f.prIssueFetcher.FetchIssues(ctx, owner, repo, 30)
		if issueErr != nil {
			f.logger.Warn("failed to fetch issues, continuing without issues",
				zap.String("repo", owner+"/"+repo),
				zap.Error(issueErr),
			)
		} else if len(issueFiles) > 0 {
			f.logger.Info("successfully fetched issues",
				zap.String("repo", owner+"/"+repo),
				zap.Int("issues_count", len(issueFiles)),
			)
			files = append(files, issueFiles...)
		}
	}

	return files, nil
}

func (f *GitHubFileFetcher) fetchViaTarball(ctx context.Context, owner, repo, branch string) ([]*ingest.ScannedFile, error) {
	// GitHub tarball URLs
	urls := []string{
		fmt.Sprintf("https://codeload.github.com/%s/%s/tar.gz/refs/heads/%s", owner, repo, branch),
		fmt.Sprintf("https://api.github.com/repos/%s/%s/tarball/%s", owner, repo, branch),
	}
	if f.tarballURLFunc != nil {
		urls = f.tarballURLFunc(owner, repo, branch)
	}

	var lastErr error
	for _, tarURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, tarURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("User-Agent", "ContextForge-Worker")
		if f.patToken != "" {
			req.Header.Set("Authorization", "Bearer "+f.patToken)
		}

		resp, err := f.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("http status %d from %s", resp.StatusCode, tarURL)
			continue
		}

		files, extractErr := f.extractTarball(resp.Body)
		_ = resp.Body.Close()
		if extractErr != nil {
			lastErr = extractErr
			continue
		}

		return files, nil
	}

	return nil, lastErr
}

func (f *GitHubFileFetcher) extractTarball(r io.Reader) ([]*ingest.ScannedFile, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("opening gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var files []*ingest.ScannedFile

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar archive: %w", err)
		}

		// Skip non-regular files
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// GitHub tarball entries have format: <owner>-<repo>-<sha>/path/to/file
		idx := strings.Index(hdr.Name, "/")
		if idx == -1 {
			continue
		}
		relPath := hdr.Name[idx+1:]
		if relPath == "" {
			continue
		}

		// Check ignore filters
		if !f.filter.ShouldIndex(relPath, hdr.Size) {
			continue
		}

		// Read file contents (limit to 1MB)
		buf, err := io.ReadAll(io.LimitReader(tr, 1024*1024))
		if err != nil {
			continue
		}

		content := string(buf)
		if ingest.IsBinaryContent(content) || strings.TrimSpace(content) == "" {
			continue
		}

		files = append(files, &ingest.ScannedFile{
			Path:        relPath,
			Language:    ingest.DetectLanguage(relPath),
			Content:     content,
			ContentHash: ingest.ComputeHash(content),
			SizeBytes:   int64(len(buf)),
		})
	}

	return files, nil
}

func (f *GitHubFileFetcher) fetchViaGitClone(ctx context.Context, owner, repo, branch string) ([]*ingest.ScannedFile, error) {
	tmpDir, err := os.MkdirTemp("", "cf-clone-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp clone dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	if f.patToken != "" {
		cloneURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", f.patToken, owner, repo)
	}

	// Try cloning the specified branch
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branch, cloneURL, tmpDir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Fallback: try default branch clone without --branch flag
		cmdFallback := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, tmpDir)
		cmdFallback.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if fallbackOutput, fallbackErr := cmdFallback.CombinedOutput(); fallbackErr != nil {
			return nil, fmt.Errorf("git clone failed: %s (fallback: %s)", strings.TrimSpace(string(output)), strings.TrimSpace(string(fallbackOutput)))
		}
	}

	var files []*ingest.ScannedFile

	walkErr := filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if !f.filter.ShouldIndex(relPath, info.Size()) {
			return nil
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(contentBytes)
		if ingest.IsBinaryContent(content) || strings.TrimSpace(content) == "" {
			return nil
		}

		files = append(files, &ingest.ScannedFile{
			Path:        relPath,
			Language:    ingest.DetectLanguage(relPath),
			Content:     content,
			ContentHash: ingest.ComputeHash(content),
			SizeBytes:   int64(len(contentBytes)),
		})

		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("walking cloned repository: %w", walkErr)
	}

	return files, nil
}
