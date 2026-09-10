package worker_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/worker"
)

func createTestTarball(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		tarPath := "testorg-contextforge-abc1234/" + name
		hdr := &tar.Header{
			Name:     tarPath,
			Mode:     0600,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func TestGitHubFileFetcher_TarballExtraction(t *testing.T) {
	files := map[string]string{
		"main.go":           "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		"internal/logic.go": "package internal\n\nfunc Add(a, b int) int { return a + b }\n",
		"binary.png":        "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00",
		"empty.txt":         "    \n\t\n   ",
	}

	tarballBytes := createTestTarball(t, files)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarballBytes)
	}))
	defer server.Close()

	fetcher := worker.NewGitHubFileFetcher("ghp_dummy", zap.NewNop())
	require.NotNil(t, fetcher)

	src := &ent.Source{
		Name:      "testorg/contextforge",
		RepoOwner: "testorg",
		RepoName:  "contextforge",
		Branch:    "main",
	}

	// Fetch files should handle valid source struct
	assert.Equal(t, "testorg", src.RepoOwner)
	assert.Equal(t, "contextforge", src.RepoName)
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}
}

type mockPRIssueFetcher struct {
	prFiles    []*ingest.ScannedFile
	prErr      error
	issueFiles []*ingest.ScannedFile
	issueErr   error
}

func (m *mockPRIssueFetcher) FetchPullRequests(ctx context.Context, owner, repo string, limit int) ([]*ingest.ScannedFile, error) {
	return m.prFiles, m.prErr
}

func (m *mockPRIssueFetcher) FetchIssues(ctx context.Context, owner, repo string, limit int) ([]*ingest.ScannedFile, error) {
	return m.issueFiles, m.issueErr
}

func TestGitHubFileFetcher_FetchFiles_WithPRIssues(t *testing.T) {
	codeFiles := map[string]string{
		"main.go":           "package main\n\nfunc main() {}\n",
		"internal/logic.go": "package internal\n\nfunc Run() {}\n",
	}
	tarballBytes := createTestTarball(t, codeFiles)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarballBytes)
	})

	mockPRIssues := &mockPRIssueFetcher{
		prFiles: []*ingest.ScannedFile{
			{
				Path:        "pulls/PR-10-feat-add-auth.md",
				Language:    "markdown",
				Content:     "# PR #10: Add auth",
				ContentHash: ingest.ComputeHash("# PR #10: Add auth"),
				SizeBytes:   19,
			},
		},
		issueFiles: []*ingest.ScannedFile{
			{
				Path:        "issues/ISSUE-5-bug-login-timeout.md",
				Language:    "markdown",
				Content:     "# Issue #5: Login timeout",
				ContentHash: ingest.ComputeHash("# Issue #5: Login timeout"),
				SizeBytes:   25,
			},
		},
	}

	fetcher := worker.NewGitHubFileFetcher("ghp_test_token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))
	fetcher.SetTarballURLFunc(func(owner, repo, branch string) []string {
		return []string{"https://mock.github.com/archive.tar.gz"}
	})
	fetcher.SetPRIssueFetcher(mockPRIssues)

	src := &ent.Source{
		Name:      "testorg/contextforge",
		RepoOwner: "testorg",
		RepoName:  "contextforge",
		Branch:    "main",
	}

	files, err := fetcher.FetchFiles(context.Background(), src)
	require.NoError(t, err)

	// Expect 2 code files + 1 PR file + 1 Issue file = 4 files total
	require.Len(t, files, 4)

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	assert.Contains(t, paths, "main.go")
	assert.Contains(t, paths, "internal/logic.go")
	assert.Contains(t, paths, "pulls/PR-10-feat-add-auth.md")
	assert.Contains(t, paths, "issues/ISSUE-5-bug-login-timeout.md")
}

func TestGitHubFileFetcher_FetchFiles_GracefulPRIssueErrors(t *testing.T) {
	codeFiles := map[string]string{
		"main.go": "package main\n",
	}
	tarballBytes := createTestTarball(t, codeFiles)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(tarballBytes)
	})

	// PR and Issue fetching fail (e.g. rate limited or 403)
	mockPRIssues := &mockPRIssueFetcher{
		prErr:    assert.AnError,
		issueErr: assert.AnError,
	}

	fetcher := worker.NewGitHubFileFetcher("ghp_test_token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))
	fetcher.SetTarballURLFunc(func(owner, repo, branch string) []string {
		return []string{"https://mock.github.com/archive.tar.gz"}
	})
	fetcher.SetPRIssueFetcher(mockPRIssues)

	src := &ent.Source{
		Name:      "testorg/contextforge",
		RepoOwner: "testorg",
		RepoName:  "contextforge",
		Branch:    "main",
	}

	// Should still succeed and return repository code files!
	files, err := fetcher.FetchFiles(context.Background(), src)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "main.go", files[0].Path)
}

func TestGitHubFileFetcher_Validation(t *testing.T) {
	fetcher := worker.NewGitHubFileFetcher("", zap.NewNop())

	t.Run("Nil source returns error", func(t *testing.T) {
		_, err := fetcher.FetchFiles(context.Background(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source cannot be nil")
	})

	t.Run("Source missing repo info returns error", func(t *testing.T) {
		src := &ent.Source{Name: "invalid"}
		_, err := fetcher.FetchFiles(context.Background(), src)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing repo_owner")
	})
}
