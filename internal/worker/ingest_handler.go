package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
)

// FileFetcher abstracts retrieving files from a repository source.
type FileFetcher interface {
	FetchFiles(ctx context.Context, source *ent.Source) ([]*ingest.ScannedFile, error)
}

// IngestionPipeline orchestrates the end-to-end repository ingestion and chunk indexing workflow.
type IngestionPipeline struct {
	docRepo      repository.DocumentRepository
	vectorRepo   repository.VectorRepository
	jobRepo      repository.JobRepository
	embedder     provider.EmbeddingProvider
	chunker      *chunk.MultiLanguageChunker
	dedupCache   ingest.DedupCache
	fileFetcher  FileFetcher
	logger       *zap.Logger
}

func NewIngestionPipeline(
	docRepo repository.DocumentRepository,
	vectorRepo repository.VectorRepository,
	jobRepo repository.JobRepository,
	embedder provider.EmbeddingProvider,
	chunker *chunk.MultiLanguageChunker,
	dedupCache ingest.DedupCache,
	fileFetcher FileFetcher,
	logger *zap.Logger,
) *IngestionPipeline {
	if chunker == nil {
		chunker = chunk.NewChunker(chunk.DefaultOptions())
	}
	if dedupCache == nil {
		dedupCache = ingest.NewMemoryDedupCache()
	}
	return &IngestionPipeline{
		docRepo:     docRepo,
		vectorRepo:  vectorRepo,
		jobRepo:     jobRepo,
		embedder:    embedder,
		chunker:     chunker,
		dedupCache:  dedupCache,
		fileFetcher: fileFetcher,
		logger:      logger,
	}
}

// ProcessSyncTask handles execution of an Asynq repo:sync task.
func (p *IngestionPipeline) ProcessSyncTask(ctx context.Context, task *asynq.Task) error {
	var payload queue.RepoSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshaling task payload: %w", err)
	}

	jobID := payload.JobID
	projectID := payload.ProjectID
	sourceID := payload.SourceID

	p.logger.Info("starting ingestion pipeline run",
		zap.String("job_id", jobID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("source_id", sourceID.String()),
	)

	// Update job to running
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, 5, 0, 0)
	}

	// 1. Fetch scanned files from source
	var scannedFiles []*ingest.ScannedFile
	if p.fileFetcher != nil {
		dummySource := &ent.Source{ID: sourceID, ProjectID: projectID}
		files, err := p.fileFetcher.FetchFiles(ctx, dummySource)
		if err != nil {
			if p.jobRepo != nil && jobID != uuid.Nil {
				_ = p.jobRepo.Fail(ctx, jobID, projectID, err.Error())
			}
			return fmt.Errorf("fetching source files: %w", err)
		}
		scannedFiles = files
	}

	totalFiles := len(scannedFiles)
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, 15, 0, totalFiles)
	}

	// 2. Query existing documents for incremental diffing
	var existingDocs []*ent.Document
	if p.docRepo != nil {
		docs, _, err := p.docRepo.ListByProjectID(ctx, projectID, "", 1, 10000)
		if err == nil {
			// Filter by source
			for _, d := range docs {
				if d.SourceID == sourceID {
					existingDocs = append(existingDocs, d)
				}
			}
		}
	}

	diff := ingest.ComputeSyncDiff(ctx, existingDocs, scannedFiles)

	// 3. Process Deleted documents
	for _, del := range diff.Deleted {
		if p.vectorRepo != nil {
			_ = p.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, del.ID)
		}
		if p.docRepo != nil {
			_ = p.docRepo.Delete(ctx, del.ID, projectID)
		}
	}

	// 4. Process Added and Modified files
	filesToProcess := append(diff.Added, diff.Modified...)
	processedCount := 0

	for i, file := range filesToProcess {
		var docID uuid.UUID
		if p.docRepo != nil {
			existingDoc, err := p.docRepo.GetByPath(ctx, projectID, sourceID, file.Path)
			if err == nil && existingDoc != nil {
				docID = existingDoc.ID
				if p.vectorRepo != nil {
					_ = p.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, docID)
				}
			} else {
				docID = uuid.New()
				_, _ = p.docRepo.Create(ctx, &ent.Document{
					ID:          docID,
					ProjectID:   projectID,
					SourceID:    sourceID,
					FilePath:    file.Path,
					Language:    file.Language,
					ContentHash: file.ContentHash,
				})
			}
		} else {
			docID = uuid.New()
		}

		// Chunk file content
		rawChunks := p.chunker.ChunkText(file.Content, file.Language)
		if len(rawChunks) == 0 {
			continue
		}

		var docChunks []*model.DocumentChunk
		for _, rc := range rawChunks {
			docChunks = append(docChunks, &model.DocumentChunk{
				ID:          uuid.New(),
				ProjectID:   projectID,
				DocumentID:  docID,
				ChunkIndex:  rc.Index,
				StartLine:   rc.StartLine,
				EndLine:     rc.EndLine,
				Content:     rc.Content,
				ContentHash: rc.ContentHash,
				TokenCount:  rc.TokenCount,
			})
		}

		// Deduplicate: check cache before embedding
		dedup := ingest.FilterDuplicateChunks(ctx, p.dedupCache, projectID, docChunks)

		// Generate embeddings for unique chunks
		if len(dedup.ChunksToEmbed) > 0 && p.embedder != nil {
			var texts []string
			for _, c := range dedup.ChunksToEmbed {
				texts = append(texts, c.Content)
			}

			embeddings, err := p.embedder.EmbedDocuments(ctx, texts)
			if err != nil {
				p.logger.Error("failed to generate embeddings", zap.Error(err))
			} else {
				for idx, emb := range embeddings {
					dedup.ChunksToEmbed[idx].Embedding = emb
					p.dedupCache.PutEmbedding(ctx, projectID, dedup.ChunksToEmbed[idx].ContentHash, emb)
				}
			}
		}

		// Save chunks to vector repository
		allChunks := append(dedup.ChunksToEmbed, dedup.CachedChunks...)
		if p.vectorRepo != nil && len(allChunks) > 0 {
			_ = p.vectorRepo.UpsertChunks(ctx, allChunks)
		}

		// Update document chunk count
		if p.docRepo != nil {
			_, _ = p.docRepo.UpdateContentHashAndChunks(ctx, docID, projectID, file.ContentHash, len(allChunks))
		}

		processedCount++
		if p.jobRepo != nil && jobID != uuid.Nil && len(filesToProcess) > 0 {
			pct := 20 + int(float64(i+1)/float64(len(filesToProcess))*75.0)
			_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, pct, processedCount, totalFiles)
		}
	}

	// 5. Complete job
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.Complete(ctx, jobID, projectID)
	}

	p.logger.Info("completed ingestion pipeline run",
		zap.String("job_id", jobID.String()),
		zap.Int("processed_files", processedCount),
	)

	return nil
}
