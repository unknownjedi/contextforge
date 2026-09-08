package ingest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ingest"
)

func TestComputeSyncDiff(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()

	existingDocs := []*ent.Document{
		{
			ID:          uuid.New(),
			ProjectID:   projectID,
			SourceID:    sourceID,
			FilePath:    "main.go",
			ContentHash: "hash_main_v1",
		},
		{
			ID:          uuid.New(),
			ProjectID:   projectID,
			SourceID:    sourceID,
			FilePath:    "utils.go",
			ContentHash: "hash_utils_v1",
		},
		{
			ID:          uuid.New(),
			ProjectID:   projectID,
			SourceID:    sourceID,
			FilePath:    "old_deleted.go",
			ContentHash: "hash_old_v1",
		},
	}

	scannedFiles := []*ingest.ScannedFile{
		// Unchanged file (same path, same hash)
		{
			Path:        "main.go",
			ContentHash: "hash_main_v1",
		},
		// Modified file (same path, different hash)
		{
			Path:        "utils.go",
			ContentHash: "hash_utils_v2_updated",
		},
		// Added file (new path)
		{
			Path:        "new_feature.go",
			ContentHash: "hash_new_feature_v1",
		},
		// old_deleted.go is absent from scannedFiles
	}

	diff := ingest.ComputeSyncDiff(ctx, existingDocs, scannedFiles)

	// Assert Added
	assert.Len(t, diff.Added, 1)
	assert.Equal(t, "new_feature.go", diff.Added[0].Path)

	// Assert Modified
	assert.Len(t, diff.Modified, 1)
	assert.Equal(t, "utils.go", diff.Modified[0].Path)

	// Assert Unchanged
	assert.Len(t, diff.Unchanged, 1)
	assert.Equal(t, "main.go", diff.Unchanged[0].Path)

	// Assert Deleted
	assert.Len(t, diff.Deleted, 1)
	assert.Equal(t, "old_deleted.go", diff.Deleted[0].FilePath)
}
