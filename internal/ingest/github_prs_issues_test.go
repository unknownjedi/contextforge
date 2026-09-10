package ingest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ingest"
)

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple title",
			input:    "Add user authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "title with symbols and punctuation",
			input:    "feat(auth): Add OAuth2 & PKCE support! [WIP]",
			expected: "feat-auth-add-oauth2-pkce-support-wip",
		},
		{
			name:     "multiple dashes and spaces",
			input:    "fix --- database   connection timeout...",
			expected: "fix-database-connection-timeout",
		},
		{
			name:     "empty or only punctuation",
			input:    "!!!???---",
			expected: "untitled",
		},
		{
			name:     "very long title gets truncated to max 50 chars",
			input:    "This is an extremely long pull request title that exceeds fifty characters by a lot",
			expected: "this-is-an-extremely-long-pull-request-title-that",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ingest.SanitizeTitle(tt.input))
		})
	}
}

func TestDeterminePRState(t *testing.T) {
	now := time.Now()

	t.Run("Merged takes precedence over closed state", func(t *testing.T) {
		pr := &ingest.GitHubPullRequest{
			State:    "closed",
			MergedAt: &now,
		}
		assert.Equal(t, "merged", ingest.DeterminePRState(pr))
	})

	t.Run("Closed without merged date", func(t *testing.T) {
		pr := &ingest.GitHubPullRequest{
			State: "closed",
		}
		assert.Equal(t, "closed", ingest.DeterminePRState(pr))
	})

	t.Run("Draft PR", func(t *testing.T) {
		pr := &ingest.GitHubPullRequest{
			State: "open",
			Draft: true,
		}
		assert.Equal(t, "draft", ingest.DeterminePRState(pr))
	})

	t.Run("Normal open PR", func(t *testing.T) {
		pr := &ingest.GitHubPullRequest{
			State: "open",
		}
		assert.Equal(t, "open", ingest.DeterminePRState(pr))
	})
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
			resp := rec.Result()
			return resp, nil
		}),
	}
}

func TestFetchPullRequests_Success(t *testing.T) {
	createdAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	mergedAt := time.Date(2025, 1, 16, 12, 30, 0, 0, time.UTC)

	mockPRs := []ingest.GitHubPullRequest{
		{
			Number:    42,
			Title:     "Add GitHub PR and Issue Ingestion",
			User:      ingest.GitHubUser{Login: "octocat", ID: 101},
			State:     "closed",
			Body:      "This PR adds support for indexing GitHub Pull Requests and Issues as markdown documents.",
			HTMLURL:   "https://github.com/testorg/testrepo/pull/42",
			CreatedAt: createdAt,
			MergedAt:  &mergedAt,
			Head:      ingest.GitHubBranchRef{Ref: "feat/pr-ingest", Sha: "abcdef1"},
			Base:      ingest.GitHubBranchRef{Ref: "main", Sha: "1234567"},
			Labels: []ingest.GitHubLabel{
				{ID: 1, Name: "enhancement"},
				{ID: 2, Name: "backend"},
			},
			Additions:    120,
			Deletions:    15,
			ChangedFiles: 4,
		},
		{
			Number:    43,
			Title:     "Fix Memory Leak in Chunker",
			User:      ingest.GitHubUser{Login: "gopher", ID: 102},
			State:     "open",
			Body:      "Resolves memory leak by reusing buffer pool.",
			HTMLURL:   "https://github.com/testorg/testrepo/pull/43",
			CreatedAt: createdAt.Add(24 * time.Hour),
			Head:      ingest.GitHubBranchRef{Ref: "fix/chunker-leak"},
			Base:      ingest.GitHubBranchRef{Ref: "main"},
		},
	}

	issueComments := []ingest.GitHubComment{
		{
			ID:        1001,
			User:      ingest.GitHubUser{Login: "reviewer1"},
			Body:      "Looks great! Verified edge cases.",
			CreatedAt: createdAt.Add(2 * time.Hour),
		},
	}

	reviewComments := []ingest.GitHubComment{
		{
			ID:        2001,
			User:      ingest.GitHubUser{Login: "reviewer2"},
			Body:      "Consider checking if content is empty here.",
			CreatedAt: createdAt.Add(3 * time.Hour),
			Path:      "internal/ingest/scanner.go",
			Line:      85,
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Verify User-Agent and Accept headers
		assert.Equal(t, "ContextForge-Ingest", r.Header.Get("User-Agent"))
		assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))
		assert.Equal(t, "Bearer test-pat-token", r.Header.Get("Authorization"))

		switch {
		case r.URL.Path == "/repos/testorg/testrepo/pulls":
			assert.Equal(t, "all", r.URL.Query().Get("state"))
			_ = json.NewEncoder(w).Encode(mockPRs)

		case r.URL.Path == "/repos/testorg/testrepo/issues/42/comments":
			_ = json.NewEncoder(w).Encode(issueComments)

		case r.URL.Path == "/repos/testorg/testrepo/pulls/42/comments":
			_ = json.NewEncoder(w).Encode(reviewComments)

		case strings.HasSuffix(r.URL.Path, "/comments"):
			_ = json.NewEncoder(w).Encode([]ingest.GitHubComment{})

		default:
			http.NotFound(w, r)
		}
	})

	fetcher := ingest.NewGitHubPRIssueFetcher("test-pat-token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))

	files, err := fetcher.FetchPullRequests(context.Background(), "testorg", "testrepo", 10)
	require.NoError(t, err)
	require.Len(t, files, 2)

	// Verify PR 42 (merged, with discussion and review comments)
	pr42 := files[0]
	assert.Equal(t, "pulls/PR-42-add-github-pr-and-issue-ingestion.md", pr42.Path)
	assert.Equal(t, "markdown", pr42.Language)
	assert.Equal(t, ingest.ComputeHash(pr42.Content), pr42.ContentHash)
	assert.Equal(t, int64(len(pr42.Content)), pr42.SizeBytes)

	// Check content format
	assert.Contains(t, pr42.Content, "# PR #42: Add GitHub PR and Issue Ingestion")
	assert.Contains(t, pr42.Content, "- **Author**: @octocat")
	assert.Contains(t, pr42.Content, "- **State**: merged")
	assert.Contains(t, pr42.Content, "- **Merged**: 2025-01-16T12:30:00Z")
	assert.Contains(t, pr42.Content, "- **Branch**: `feat/pr-ingest` -> `main`")
	assert.Contains(t, pr42.Content, "- **Diff Summary**: +120 / -15 across 4 files")
	assert.Contains(t, pr42.Content, "- **Labels**: enhancement, backend")
	assert.Contains(t, pr42.Content, "## Description\n\nThis PR adds support for indexing GitHub Pull Requests")
	assert.Contains(t, pr42.Content, "### @reviewer1")
	assert.Contains(t, pr42.Content, "Looks great! Verified edge cases.")
	assert.Contains(t, pr42.Content, "### @reviewer2 on 2025-01-15T13:00:00Z - review on `internal/ingest/scanner.go` (line 85):")
	assert.Contains(t, pr42.Content, "Consider checking if content is empty here.")

	// Verify PR 43 (open, no comments)
	pr43 := files[1]
	assert.Equal(t, "pulls/PR-43-fix-memory-leak-in-chunker.md", pr43.Path)
	assert.Equal(t, "markdown", pr43.Language)
	assert.Contains(t, pr43.Content, "- **State**: open")
	assert.Contains(t, pr43.Content, "Resolves memory leak by reusing buffer pool.")
}

func TestFetchIssues_Success(t *testing.T) {
	createdAt := time.Date(2025, 2, 1, 9, 0, 0, 0, time.UTC)

	// Note: GitHub /issues endpoint returns both issues and PRs (PRs have pull_request != nil)
	rawIssues := []ingest.GitHubIssue{
		{
			Number:    10,
			Title:     "Bug: Database connection pool exhausted under load",
			User:      ingest.GitHubUser{Login: "user-alpha"},
			State:     "open",
			Body:      "We observe database connection starvation when concurrent workers spike above 50.",
			HTMLURL:   "https://github.com/testorg/testrepo/issues/10",
			CreatedAt: createdAt,
			Labels: []ingest.GitHubLabel{
				{ID: 1, Name: "bug"},
				{ID: 2, Name: "database"},
			},
		},
		{
			// This item is a PR returned by /issues endpoint -> must be filtered out!
			Number: 11,
			Title:  "Fix connection pool leak (PR)",
			User:   ingest.GitHubUser{Login: "user-beta"},
			State:  "open",
			Body:   "PR body",
			PullRequest: &struct {
				URL      string `json:"url"`
				HTMLURL  string `json:"html_url"`
				DiffURL  string `json:"diff_url"`
				PatchURL string `json:"patch_url"`
			}{
				URL: "https://api.github.com/repos/testorg/testrepo/pulls/11",
			},
		},
		{
			Number:    12,
			Title:     "Feature Request: Add Redis caching layer",
			User:      ingest.GitHubUser{Login: "user-gamma"},
			State:     "closed",
			Body:      "Support Redis as an L2 cache for chunk embeddings.",
			HTMLURL:   "https://github.com/testorg/testrepo/issues/12",
			CreatedAt: createdAt.Add(12 * time.Hour),
			Labels: []ingest.GitHubLabel{
				{ID: 3, Name: "feature"},
			},
		},
	}

	issueComments := []ingest.GitHubComment{
		{
			ID:        501,
			User:      ingest.GitHubUser{Login: "maintainer"},
			Body:      "Investigating. Could be related to unclosed transactions.",
			CreatedAt: createdAt.Add(1 * time.Hour),
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repos/testorg/testrepo/issues":
			assert.Equal(t, "all", r.URL.Query().Get("state"))
			_ = json.NewEncoder(w).Encode(rawIssues)

		case r.URL.Path == "/repos/testorg/testrepo/issues/10/comments":
			_ = json.NewEncoder(w).Encode(issueComments)

		case strings.HasSuffix(r.URL.Path, "/comments"):
			_ = json.NewEncoder(w).Encode([]ingest.GitHubComment{})

		default:
			http.NotFound(w, r)
		}
	})

	fetcher := ingest.NewGitHubPRIssueFetcher("test-token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))

	files, err := fetcher.FetchIssues(context.Background(), "testorg", "testrepo", 10)
	require.NoError(t, err)

	// Ensure PR #11 was filtered out, leaving exactly 2 issues (#10 and #12)
	require.Len(t, files, 2)

	// Verify Issue 10
	iss10 := files[0]
	assert.Equal(t, "issues/ISSUE-10-bug-database-connection-pool-exhausted-under-load.md", iss10.Path)
	assert.Equal(t, "markdown", iss10.Language)
	assert.Equal(t, ingest.ComputeHash(iss10.Content), iss10.ContentHash)
	assert.Equal(t, int64(len(iss10.Content)), iss10.SizeBytes)

	assert.Contains(t, iss10.Content, "# Issue #10: Bug: Database connection pool exhausted under load")
	assert.Contains(t, iss10.Content, "- **Author**: @user-alpha")
	assert.Contains(t, iss10.Content, "- **State**: open")
	assert.Contains(t, iss10.Content, "- **Labels**: bug, database")
	assert.Contains(t, iss10.Content, "We observe database connection starvation")
	assert.Contains(t, iss10.Content, "## Discussion Comments")
	assert.Contains(t, iss10.Content, "### @maintainer")
	assert.Contains(t, iss10.Content, "Investigating. Could be related to unclosed transactions.")

	// Verify Issue 12
	iss12 := files[1]
	assert.Equal(t, "issues/ISSUE-12-feature-request-add-redis-caching-layer.md", iss12.Path)
	assert.Contains(t, iss12.Content, "- **State**: closed")
	assert.Contains(t, iss12.Content, "- **Labels**: feature")
}

func TestFetchPullRequests_ErrorHandling(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	})

	fetcher := ingest.NewGitHubPRIssueFetcher("invalid-token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))

	files, err := fetcher.FetchPullRequests(context.Background(), "testorg", "testrepo", 10)
	assert.Error(t, err)
	assert.Nil(t, files)
	assert.Contains(t, err.Error(), "status 401")
	assert.Contains(t, err.Error(), "Bad credentials")
}

func TestFetchIssues_ErrorHandling(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})

	fetcher := ingest.NewGitHubPRIssueFetcher("test-token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))

	files, err := fetcher.FetchIssues(context.Background(), "testorg", "testrepo", 10)
	assert.Error(t, err)
	assert.Nil(t, files)
	assert.Contains(t, err.Error(), "status 403")
	assert.Contains(t, err.Error(), "API rate limit exceeded")
}

func TestCommentsFailure_GracefulDegradation(t *testing.T) {
	// Even if comments endpoint returns 404 or 500, PR/issue fetch should still succeed!
	mockPRs := []ingest.GitHubPullRequest{
		{
			Number: 5,
			Title:  "Refactor logging",
			User:   ingest.GitHubUser{Login: "dev"},
			State:  "open",
			Body:   "Switches to zap logger.",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/testorg/testrepo/pulls" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mockPRs)
			return
		}

		// Comment endpoints return 500 Internal Server Error
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal server error"}`))
	})

	fetcher := ingest.NewGitHubPRIssueFetcher("test-token", zap.NewNop())
	fetcher.SetHTTPClient(newMockHTTPClient(handler))

	files, err := fetcher.FetchPullRequests(context.Background(), "testorg", "testrepo", 10)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "pulls/PR-5-refactor-logging.md", files[0].Path)
	assert.Contains(t, files[0].Content, "Switches to zap logger.")
}

func TestValidation(t *testing.T) {
	fetcher := ingest.NewGitHubPRIssueFetcher("", zap.NewNop())

	t.Run("Empty owner returns error", func(t *testing.T) {
		_, err := fetcher.FetchPullRequests(context.Background(), "", "repo", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner and repo cannot be empty")

		_, err = fetcher.FetchIssues(context.Background(), "", "repo", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner and repo cannot be empty")
	})

	t.Run("Empty repo returns error", func(t *testing.T) {
		_, err := fetcher.FetchPullRequests(context.Background(), "owner", "", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner and repo cannot be empty")

		_, err = fetcher.FetchIssues(context.Background(), "owner", "", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner and repo cannot be empty")
	})
}
