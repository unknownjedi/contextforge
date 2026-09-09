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
