package ingest

import (
	"context"

	"github.com/your-org/contextforge/internal/ent"
)

// DiffResult partitions files into synchronization change sets.
type DiffResult struct {
	Added     []*ScannedFile
	Modified  []*ScannedFile
	Unchanged []*ScannedFile
	Deleted   []*ent.Document
}

// ComputeSyncDiff calculates incremental differences between scanned files and current database documents.
func ComputeSyncDiff(
	ctx context.Context,
	existingDocs []*ent.Document,
	scannedFiles []*ScannedFile,
) *DiffResult {
	res := &DiffResult{}

	// Index existing documents by file path
	docByPath := make(map[string]*ent.Document, len(existingDocs))
	for _, d := range existingDocs {
		docByPath[d.FilePath] = d
	}

	// Track seen paths from scanner
	seenPaths := make(map[string]bool, len(scannedFiles))

	for _, file := range scannedFiles {
		seenPaths[file.Path] = true
		existing, found := docByPath[file.Path]

		if !found {
			res.Added = append(res.Added, file)
		} else if existing.ContentHash != file.ContentHash {
			res.Modified = append(res.Modified, file)
		} else {
			res.Unchanged = append(res.Unchanged, file)
		}
	}

	// Any existing document not present in scanned files is marked Deleted
	for _, d := range existingDocs {
		if !seenPaths[d.FilePath] {
			res.Deleted = append(res.Deleted, d)
		}
	}

	return res
}
