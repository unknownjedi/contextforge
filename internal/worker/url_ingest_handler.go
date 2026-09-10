package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/urlfetch"
)

// URLIngestHandler processes URL content synchronization and embedding jobs.
type URLIngestHandler struct {
	sourceRepo repository.SourceRepository
	docRepo    repository.DocumentRepository
	vectorRepo repository.VectorRepository
	jobRepo    repository.JobRepository
	embedder   provider.EmbeddingProvider
	chunker    *chunk.MultiLanguageChunker
	fetcher    urlfetch.URLFetcher
	logger     *zap.Logger
}

// NewURLIngestHandler creates a new URLIngestHandler.
func NewURLIngestHandler(
	sourceRepo repository.SourceRepository,
	docRepo repository.DocumentRepository,
	vectorRepo repository.VectorRepository,
	jobRepo repository.JobRepository,
	embedder provider.EmbeddingProvider,
	chunker *chunk.MultiLanguageChunker,
	fetcher urlfetch.URLFetcher,
	logger *zap.Logger,
) *URLIngestHandler {
	if chunker == nil {
		chunker = chunk.NewChunker(chunk.DefaultOptions())
	}
	if fetcher == nil {
		fetcher = urlfetch.NewFetcher()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &URLIngestHandler{
		sourceRepo: sourceRepo,
		docRepo:    docRepo,
		vectorRepo: vectorRepo,
		jobRepo:    jobRepo,
		embedder:   embedder,
		chunker:    chunker,
		fetcher:    fetcher,
		logger:     logger,
	}
}

// ProcessURLSyncTask handles execution of an Asynq url:sync task.
func (h *URLIngestHandler) ProcessURLSyncTask(ctx context.Context, task *asynq.Task) error {
	var payload queue.URLSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshaling url sync payload: %w", err)
	}

	jobID := payload.JobID
	projectID := payload.ProjectID
	sourceID := payload.SourceID
	targetURL := strings.TrimSpace(payload.URL)

	h.logger.Info("starting URL ingestion pipeline run",
		zap.String("job_id", jobID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("source_id", sourceID.String()),
		zap.String("url", targetURL),
	)

	// Step 1: Update job status to running (5%)
	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 5, 0, 1)
	}

	// Step 2: Fetch source record to update status and/or retrieve URL
	var src *ent.Source
	if h.sourceRepo != nil {
		var err error
		src, err = h.sourceRepo.GetByID(ctx, sourceID, projectID)
		if err != nil {
			errMsg := fmt.Sprintf("getting source record: %v", err)
			h.logger.Error(errMsg, zap.Error(err))
			if h.jobRepo != nil && jobID != uuid.Nil {
				_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
			}
			return fmt.Errorf("getting source from database: %w", err)
		}

		if targetURL == "" {
			targetURL = strings.TrimSpace(src.RepoName)
			if targetURL == "" {
				targetURL = strings.TrimSpace(src.Name)
			}
		}

		_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusSyncing, src.LastCommitHash, nil)
	}

	if targetURL == "" {
		errMsg := "target URL is empty and could not be determined"
		h.logger.Error(errMsg)
		if h.jobRepo != nil && jobID != uuid.Nil {
			_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
		}
		if h.sourceRepo != nil {
			_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
		}
		return errors.New(errMsg)
	}

	// Step 3: Fetch URL content (15% -> 40%)
	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 15, 0, 1)
	}

	fetchResult, err := h.fetcher.Fetch(ctx, targetURL)
	if err != nil {
		errMsg := fmt.Sprintf("fetching URL %s: %v", targetURL, err)
		h.logger.Error(errMsg, zap.Error(err))
		if h.jobRepo != nil && jobID != uuid.Nil {
			_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
		}
		if h.sourceRepo != nil {
			_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
		}
		return fmt.Errorf("fetching URL %s: %w", targetURL, err)
	}

	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 40, 0, 1)
	}

	// Step 4: Chunk content
	content := fetchResult.Content
	rawChunks := h.chunker.ChunkText(content, "markdown")
	if len(rawChunks) == 0 && len(strings.TrimSpace(content)) > 0 {
		rawChunks = []chunk.Chunk{
			{
				Index:       0,
				StartLine:   1,
				EndLine:     len(strings.Split(content, "\n")),
				Content:     content,
				ContentHash: fetchResult.ContentHash,
				TokenCount:  len(strings.Fields(content)),
			},
		}
	}

	// Step 5: Upsert Document record in docRepo
	var docID uuid.UUID
	var isExisting bool
	if h.docRepo != nil {
		existingDoc, err := h.docRepo.GetByPath(ctx, projectID, sourceID, targetURL)
		if err == nil && existingDoc != nil {
			docID = existingDoc.ID
			isExisting = true
		} else {
			docID = uuid.New()
			if _, err := h.docRepo.Create(ctx, &ent.Document{
				ID:          docID,
				ProjectID:   projectID,
				SourceID:    sourceID,
				FilePath:    targetURL,
				Language:    "markdown",
				ContentHash: fetchResult.ContentHash,
				TotalChunks: len(rawChunks),
			}); err != nil {
				errMsg := fmt.Sprintf("creating document record: %v", err)
				h.logger.Error(errMsg, zap.Error(err))
				if h.jobRepo != nil && jobID != uuid.Nil {
					_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
				}
				if h.sourceRepo != nil {
					_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
				}
				return fmt.Errorf("creating document record: %w", err)
			}
		}
	} else {
		docID = uuid.New()
	}

	// Step 6: Build document chunks (50%)
	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 50, 0, 1)
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

	// Step 7: Generate embeddings for chunks (70%)
	if len(docChunks) > 0 {
		if h.embedder == nil {
			errMsg := fmt.Sprintf("embedding provider is not configured for indexing %s", targetURL)
			h.logger.Error(errMsg)
			if h.jobRepo != nil && jobID != uuid.Nil {
				_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
			}
			if h.sourceRepo != nil {
				_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
			}
			return fmt.Errorf("embedding provider required for %s", targetURL)
		}

		var texts []string
		for _, c := range docChunks {
			texts = append(texts, c.Content)
		}

		embeddings, err := h.embedder.EmbedDocuments(ctx, texts)
		if err != nil {
			errMsg := fmt.Sprintf("generating embeddings failed for %s: %v", targetURL, err)
			h.logger.Error(errMsg, zap.Error(err))
			if h.jobRepo != nil && jobID != uuid.Nil {
				_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
			}
			if h.sourceRepo != nil {
				_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
			}
			return fmt.Errorf("generating embeddings for %s: %w", targetURL, err)
		}

		if len(embeddings) != len(docChunks) {
			errMsg := fmt.Sprintf("embedding count mismatch for %s: expected %d, got %d", targetURL, len(docChunks), len(embeddings))
			h.logger.Error(errMsg)
			if h.jobRepo != nil && jobID != uuid.Nil {
				_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
			}
			if h.sourceRepo != nil {
				_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
			}
			return fmt.Errorf("embedding count mismatch for %s: %d != %d", targetURL, len(docChunks), len(embeddings))
		}

		for idx, emb := range embeddings {
			docChunks[idx].Embedding = emb
		}
	}

	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 85, 0, 1)
	}

	// Step 8: Save chunks to vector repository
	if isExisting && h.vectorRepo != nil {
		_ = h.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, docID)
	}

	if h.vectorRepo != nil && len(docChunks) > 0 {
		if err := h.vectorRepo.UpsertChunks(ctx, docChunks); err != nil {
			errMsg := fmt.Sprintf("vector upsert failed for %s: %v", targetURL, err)
			h.logger.Error(errMsg, zap.Error(err))
			if h.jobRepo != nil && jobID != uuid.Nil {
				_ = h.jobRepo.Fail(ctx, jobID, projectID, errMsg)
			}
			if h.sourceRepo != nil {
				_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusFailed, "", nil)
			}
			return fmt.Errorf("upserting chunks for %s: %w", targetURL, err)
		}
	}

	// Step 9: Update document chunk count and content hash
	if h.docRepo != nil {
		_, _ = h.docRepo.UpdateContentHashAndChunks(ctx, docID, projectID, fetchResult.ContentHash, len(docChunks))
	}

	// Step 10: Complete job & mark source synced
	now := time.Now()
	if h.sourceRepo != nil {
		_, _ = h.sourceRepo.UpdateSyncStatus(ctx, sourceID, projectID, source.SyncStatusSynced, fetchResult.ContentHash, &now)
	}
	if h.jobRepo != nil && jobID != uuid.Nil {
		_ = h.jobRepo.UpdateProgress(ctx, jobID, projectID, 100, 1, 1)
		_ = h.jobRepo.Complete(ctx, jobID, projectID)
	}

	h.logger.Info("completed URL ingestion pipeline run",
		zap.String("job_id", jobID.String()),
		zap.String("url", targetURL),
		zap.Int("chunks", len(docChunks)),
	)

	return nil
}
