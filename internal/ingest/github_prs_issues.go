package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// GitHubUser represents author information in GitHub API responses.
type GitHubUser struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

// GitHubLabel represents a repository label.
type GitHubLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// GitHubBranchRef represents branch metadata for PR head/base.
type GitHubBranchRef struct {
	Ref string `json:"ref"`
	Sha string `json:"sha"`
}

// GitHubPullRequest represents the JSON model returned by GitHub Pull Request REST endpoints.
type GitHubPullRequest struct {
	Number            int             `json:"number"`
	Title             string          `json:"title"`
	User              GitHubUser      `json:"user"`
	State             string          `json:"state"`
	Body              string          `json:"body"`
	HTMLURL           string          `json:"html_url"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ClosedAt          *time.Time      `json:"closed_at"`
	MergedAt          *time.Time      `json:"merged_at"`
	Draft             bool            `json:"draft"`
	Head              GitHubBranchRef `json:"head"`
	Base              GitHubBranchRef `json:"base"`
	Labels            []GitHubLabel   `json:"labels"`
	CommentsURL       string          `json:"comments_url"`
	ReviewCommentsURL string          `json:"review_comments_url"`
	Comments          int             `json:"comments"`
	ReviewComments    int             `json:"review_comments"`
	Additions         int             `json:"additions"`
	Deletions         int             `json:"deletions"`
	ChangedFiles      int             `json:"changed_files"`
}

// GitHubIssue represents the JSON model returned by GitHub Issues REST endpoints.
type GitHubIssue struct {
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	User        GitHubUser    `json:"user"`
	State       string        `json:"state"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ClosedAt    *time.Time    `json:"closed_at"`
	Labels      []GitHubLabel `json:"labels"`
	CommentsURL string        `json:"comments_url"`
	Comments    int           `json:"comments"`
	PullRequest *struct {
		URL      string `json:"url"`
		HTMLURL  string `json:"html_url"`
		DiffURL  string `json:"diff_url"`
		PatchURL string `json:"patch_url"`
	} `json:"pull_request"`
}

// GitHubComment represents an issue discussion comment or PR review comment.
type GitHubComment struct {
	ID        int64      `json:"id"`
	User      GitHubUser `json:"user"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	HTMLURL   string     `json:"html_url"`
	Path      string     `json:"path,omitempty"`
	Line      int        `json:"line,omitempty"`
}

// GitHubPRIssueFetcher retrieves pull requests and issues via the GitHub REST API.
type GitHubPRIssueFetcher struct {
	httpClient *http.Client
	patToken   string
	baseURL    string
	logger     *zap.Logger
}

// NewGitHubPRIssueFetcher creates a new GitHubPRIssueFetcher.
func NewGitHubPRIssueFetcher(patToken string, logger *zap.Logger) *GitHubPRIssueFetcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GitHubPRIssueFetcher{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		patToken: strings.TrimSpace(patToken),
		baseURL:  "https://api.github.com",
		logger:   logger,
	}
}

// SetBaseURL configures the base URL for the GitHub REST API (useful for testing or GitHub Enterprise).
func (f *GitHubPRIssueFetcher) SetBaseURL(baseURL string) {
	f.baseURL = strings.TrimRight(baseURL, "/")
}

// SetHTTPClient overrides the internal HTTP client.
func (f *GitHubPRIssueFetcher) SetHTTPClient(client *http.Client) {
	if client != nil {
		f.httpClient = client
	}
}

// SanitizeTitle converts a title string into a clean, filesystem-safe kebab-case slug.
func SanitizeTitle(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	lastWasDash := false
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasDash = false
		} else if !lastWasDash && b.Len() > 0 {
			b.WriteRune('-')
			lastWasDash = true
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > 50 {
		res = strings.TrimRight(res[:50], "-")
	}
	if res == "" {
		return "untitled"
	}
	return res
}

// DeterminePRState calculates normalized PR state: "open", "merged", or "closed".
func DeterminePRState(pr *GitHubPullRequest) string {
	if pr.MergedAt != nil && !pr.MergedAt.IsZero() {
		return "merged"
	}
	state := strings.ToLower(strings.TrimSpace(pr.State))
	if state == "closed" {
		return "closed"
	}
	if pr.Draft {
		return "draft"
	}
	if state == "" {
		return "open"
	}
	return state
}

// DetermineIssueState calculates normalized issue state: "open" or "closed".
func DetermineIssueState(issue *GitHubIssue) string {
	state := strings.ToLower(strings.TrimSpace(issue.State))
	if state == "" {
		return "open"
	}
	return state
}

// FormatPullRequestMarkdown renders a normalized markdown document for a pull request.
func FormatPullRequestMarkdown(pr *GitHubPullRequest, comments []*GitHubComment) string {
	var sb strings.Builder

	state := DeterminePRState(pr)
	fmt.Fprintf(&sb, "# PR #%d: %s\n\n", pr.Number, pr.Title)
	sb.WriteString("- **Type**: Pull Request\n")
	fmt.Fprintf(&sb, "- **Number**: #%d\n", pr.Number)
	if pr.User.Login != "" {
		fmt.Fprintf(&sb, "- **Author**: @%s\n", pr.User.Login)
	}
	fmt.Fprintf(&sb, "- **State**: %s\n", state)
	if !pr.CreatedAt.IsZero() {
		fmt.Fprintf(&sb, "- **Created**: %s\n", pr.CreatedAt.UTC().Format(time.RFC3339))
	}
	if pr.MergedAt != nil && !pr.MergedAt.IsZero() {
		fmt.Fprintf(&sb, "- **Merged**: %s\n", pr.MergedAt.UTC().Format(time.RFC3339))
	} else if pr.ClosedAt != nil && !pr.ClosedAt.IsZero() {
		fmt.Fprintf(&sb, "- **Closed**: %s\n", pr.ClosedAt.UTC().Format(time.RFC3339))
	}
	if pr.Head.Ref != "" || pr.Base.Ref != "" {
		fmt.Fprintf(&sb, "- **Branch**: `%s` -> `%s`\n", pr.Head.Ref, pr.Base.Ref)
	}
	if pr.Additions > 0 || pr.Deletions > 0 || pr.ChangedFiles > 0 {
		fmt.Fprintf(&sb, "- **Diff Summary**: +%d / -%d across %d files\n", pr.Additions, pr.Deletions, pr.ChangedFiles)
	}
	if len(pr.Labels) > 0 {
		var labelNames []string
		for _, l := range pr.Labels {
			if l.Name != "" {
				labelNames = append(labelNames, l.Name)
			}
		}
		if len(labelNames) > 0 {
			fmt.Fprintf(&sb, "- **Labels**: %s\n", strings.Join(labelNames, ", "))
		}
	}
	if pr.HTMLURL != "" {
		fmt.Fprintf(&sb, "- **URL**: %s\n", pr.HTMLURL)
	}

	sb.WriteString("\n## Description\n\n")
	body := strings.TrimSpace(pr.Body)
	if body == "" {
		sb.WriteString("*No description provided.*\n")
	} else {
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	if len(comments) > 0 {
		sb.WriteString("\n## Discussion & Review Comments\n\n")
		for _, c := range comments {
			if c == nil || strings.TrimSpace(c.Body) == "" {
				continue
			}
			author := c.User.Login
			if author == "" {
				author = "anonymous"
			}
			dateStr := ""
			if !c.CreatedAt.IsZero() {
				dateStr = " on " + c.CreatedAt.UTC().Format(time.RFC3339)
			}

			if c.Path != "" {
				lineInfo := ""
				if c.Line > 0 {
					lineInfo = fmt.Sprintf(" (line %d)", c.Line)
				}
				fmt.Fprintf(&sb, "### @%s%s - review on `%s`%s:\n\n", author, dateStr, c.Path, lineInfo)
			} else {
				fmt.Fprintf(&sb, "### @%s%s:\n\n", author, dateStr)
			}
			sb.WriteString(strings.TrimSpace(c.Body))
			sb.WriteString("\n\n---\n\n")
		}
	}

	return sb.String()
}

// FormatIssueMarkdown renders a normalized markdown document for an issue.
func FormatIssueMarkdown(issue *GitHubIssue, comments []*GitHubComment) string {
	var sb strings.Builder

	state := DetermineIssueState(issue)
	fmt.Fprintf(&sb, "# Issue #%d: %s\n\n", issue.Number, issue.Title)
	sb.WriteString("- **Type**: Issue\n")
	fmt.Fprintf(&sb, "- **Number**: #%d\n", issue.Number)
	if issue.User.Login != "" {
		fmt.Fprintf(&sb, "- **Author**: @%s\n", issue.User.Login)
	}
	fmt.Fprintf(&sb, "- **State**: %s\n", state)
	if !issue.CreatedAt.IsZero() {
		fmt.Fprintf(&sb, "- **Created**: %s\n", issue.CreatedAt.UTC().Format(time.RFC3339))
	}
	if issue.ClosedAt != nil && !issue.ClosedAt.IsZero() {
		fmt.Fprintf(&sb, "- **Closed**: %s\n", issue.ClosedAt.UTC().Format(time.RFC3339))
	}

	labelStr := "None"
	if len(issue.Labels) > 0 {
		var labelNames []string
		for _, l := range issue.Labels {
			if l.Name != "" {
				labelNames = append(labelNames, l.Name)
			}
		}
		if len(labelNames) > 0 {
			labelStr = strings.Join(labelNames, ", ")
		}
	}
	fmt.Fprintf(&sb, "- **Labels**: %s\n", labelStr)

	if issue.HTMLURL != "" {
		fmt.Fprintf(&sb, "- **URL**: %s\n", issue.HTMLURL)
	}

	sb.WriteString("\n## Description\n\n")
	body := strings.TrimSpace(issue.Body)
	if body == "" {
		sb.WriteString("*No description provided.*\n")
	} else {
		sb.WriteString(body)
		sb.WriteString("\n")
	}

	if len(comments) > 0 {
		sb.WriteString("\n## Discussion Comments\n\n")
		for _, c := range comments {
			if c == nil || strings.TrimSpace(c.Body) == "" {
				continue
			}
			author := c.User.Login
			if author == "" {
				author = "anonymous"
			}
			dateStr := ""
			if !c.CreatedAt.IsZero() {
				dateStr = " on " + c.CreatedAt.UTC().Format(time.RFC3339)
			}

			fmt.Fprintf(&sb, "### @%s%s:\n\n", author, dateStr)
			sb.WriteString(strings.TrimSpace(c.Body))
			sb.WriteString("\n\n---\n\n")
		}
	}

	return sb.String()
}

func (f *GitHubPRIssueFetcher) doRequest(ctx context.Context, endpoint string) (*http.Response, error) {
	url := endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		url = fmt.Sprintf("%s/%s", strings.TrimRight(f.baseURL, "/"), strings.TrimLeft(endpoint, "/"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ContextForge-Ingest")
	if f.patToken != "" {
		req.Header.Set("Authorization", "Bearer "+f.patToken)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}

func (f *GitHubPRIssueFetcher) getComments(ctx context.Context, endpoint string) []*GitHubComment {
	resp, err := f.doRequest(ctx, endpoint)
	if err != nil {
		f.logger.Debug("failed to fetch comments", zap.String("endpoint", endpoint), zap.Error(err))
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.logger.Debug("non-200 status fetching comments", zap.String("endpoint", endpoint), zap.Int("status", resp.StatusCode))
		return nil
	}

	var comments []*GitHubComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		f.logger.Debug("failed to decode comments", zap.String("endpoint", endpoint), zap.Error(err))
		return nil
	}

	return comments
}

func (f *GitHubPRIssueFetcher) fetchPRComments(ctx context.Context, owner, repo string, prNumber int) []*GitHubComment {
	var allComments []*GitHubComment

	// 1. Issue conversation comments: /repos/{owner}/{repo}/issues/{number}/comments
	issueCommentsEndpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=10", owner, repo, prNumber)
	if comments := f.getComments(ctx, issueCommentsEndpoint); len(comments) > 0 {
		allComments = append(allComments, comments...)
	}

	// 2. Diff review comments: /repos/%s/%s/pulls/{number}/comments
	reviewCommentsEndpoint := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments?per_page=10", owner, repo, prNumber)
	if comments := f.getComments(ctx, reviewCommentsEndpoint); len(comments) > 0 {
		allComments = append(allComments, comments...)
	}

	return allComments
}

// FetchPullRequests fetches pull requests for a repository and returns normalized markdown ScannedFiles.
func (f *GitHubPRIssueFetcher) FetchPullRequests(ctx context.Context, owner, repo string, limit int) ([]*ScannedFile, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo cannot be empty")
	}
	if limit <= 0 {
		limit = 30
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=%d", owner, repo, perPage)
	resp, err := f.doRequest(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetching pull requests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api error fetching pull requests (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var pulls []*GitHubPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&pulls); err != nil {
		return nil, fmt.Errorf("decoding pull requests response: %w", err)
	}

	var files []*ScannedFile
	for _, pr := range pulls {
		if pr == nil {
			continue
		}
		if len(files) >= limit {
			break
		}

		comments := f.fetchPRComments(ctx, owner, repo, pr.Number)
		content := FormatPullRequestMarkdown(pr, comments)
		path := fmt.Sprintf("pulls/PR-%d-%s.md", pr.Number, SanitizeTitle(pr.Title))

		files = append(files, &ScannedFile{
			Path:        path,
			Language:    "markdown",
			Content:     content,
			ContentHash: ComputeHash(content),
			SizeBytes:   int64(len(content)),
		})
	}

	return files, nil
}

// FetchIssues fetches issues for a repository (excluding PRs) and returns normalized markdown ScannedFiles.
func (f *GitHubPRIssueFetcher) FetchIssues(ctx context.Context, owner, repo string, limit int) ([]*ScannedFile, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo cannot be empty")
	}
	if limit <= 0 {
		limit = 30
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=all&per_page=%d", owner, repo, perPage)
	resp, err := f.doRequest(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetching issues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("github api error fetching issues (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var issues []*GitHubIssue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("decoding issues response: %w", err)
	}

	var files []*ScannedFile
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		// Filter out PRs returned by the issues endpoint
		if issue.PullRequest != nil {
			continue
		}
		if len(files) >= limit {
			break
		}

		commentsEndpoint := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=10", owner, repo, issue.Number)
		comments := f.getComments(ctx, commentsEndpoint)

		content := FormatIssueMarkdown(issue, comments)
		path := fmt.Sprintf("issues/ISSUE-%d-%s.md", issue.Number, SanitizeTitle(issue.Title))

		files = append(files, &ScannedFile{
			Path:        path,
			Language:    "markdown",
			Content:     content,
			ContentHash: ComputeHash(content),
			SizeBytes:   int64(len(content)),
		})
	}

	return files, nil
}
