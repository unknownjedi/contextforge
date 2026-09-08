package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/databasesource"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/service"
)

// DatabaseIngestionPipeline handles asynchronous ingestion and indexing of external databases.
type DatabaseIngestionPipeline struct {
	client        *ent.Client
	registry      *connector.ConnectorRegistry
	encryptionKey []byte
	docRepo       repository.DocumentRepository
	vectorRepo    repository.VectorRepository
	jobRepo       repository.JobRepository
	embedder      provider.EmbeddingProvider
	chunker       *chunk.MultiLanguageChunker
	normalizer    *service.KnowledgeNormalizer
	logger        *zap.Logger
}

// NewDatabaseIngestionPipeline constructs a DatabaseIngestionPipeline.
func NewDatabaseIngestionPipeline(
	client *ent.Client,
	registry *connector.ConnectorRegistry,
	encryptionKey []byte,
	docRepo repository.DocumentRepository,
	vectorRepo repository.VectorRepository,
	jobRepo repository.JobRepository,
	embedder provider.EmbeddingProvider,
	chunker *chunk.MultiLanguageChunker,
	logger *zap.Logger,
) *DatabaseIngestionPipeline {
	if registry == nil {
		registry = connector.DefaultRegistry()
	}
	if chunker == nil {
		chunker = chunk.NewChunker(chunk.DefaultOptions())
	}
	return &DatabaseIngestionPipeline{
		client:        client,
		registry:      registry,
		encryptionKey: encryptionKey,
		docRepo:       docRepo,
		vectorRepo:    vectorRepo,
		jobRepo:       jobRepo,
		embedder:      embedder,
		chunker:       chunker,
		normalizer:    service.NewKnowledgeNormalizer(),
		logger:        logger,
	}
}

// ProcessDatabaseSyncTask processes an Asynq database:sync task.
func (p *DatabaseIngestionPipeline) ProcessDatabaseSyncTask(ctx context.Context, task *asynq.Task) error {
	var payload queue.DatabaseSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshaling database sync payload: %w", err)
	}

	jobID := payload.JobID
	projectID := payload.ProjectID
	sourceID := payload.SourceID

	p.logger.Info("starting database sync pipeline run",
		zap.String("job_id", jobID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("source_id", sourceID.String()),
	)

	// Update job status to running (5%)
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, 5, 0, 0)
	}

	// 1. Fetch DatabaseSource record
	dbSrc, err := p.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.SourceIDEQ(sourceID),
		).
		WithSource().
		Only(ctx)
	if err != nil {
		p.failJob(ctx, jobID, projectID, sourceID, fmt.Errorf("loading database source: %w", err))
		return fmt.Errorf("loading database source: %w", err)
	}

	// 2. Decrypt credentials
	decryptedURL, err := crypto.Decrypt(dbSrc.EncryptedConnectionURL, p.encryptionKey)
	if err != nil {
		p.failJob(ctx, jobID, projectID, sourceID, fmt.Errorf("decrypting credentials: %w", err))
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	// 3. Resolve connector
	conn, err := p.registry.Get(connector.DatabaseType(dbSrc.DatabaseType))
	if err != nil {
		p.failJob(ctx, jobID, projectID, sourceID, err)
		return err
	}

	cfg, err := conn.ParseURL(string(decryptedURL))
	if err != nil {
		p.failJob(ctx, jobID, projectID, sourceID, conn.SanitizeError(err))
		return conn.SanitizeError(err)
	}

	// 4. Parse extraction options from configuration
	extractOpts := connector.DefaultExtractionConfig()
	if dbSrc.Configuration != nil {
		if mode, ok := dbSrc.Configuration["mode"].(string); ok && mode != "" {
			extractOpts.Mode = mode
		}
		if schemas, ok := dbSrc.Configuration["schemas"].([]any); ok {
			for _, s := range schemas {
				if str, ok := s.(string); ok && str != "" {
					extractOpts.Schemas = append(extractOpts.Schemas, str)
				}
			}
		}
		if tables, ok := dbSrc.Configuration["tables"].([]any); ok {
			for _, t := range tables {
				if str, ok := t.(string); ok && str != "" {
					extractOpts.Tables = append(extractOpts.Tables, str)
				}
			}
		}
		if excluded, ok := dbSrc.Configuration["excluded_columns"].(map[string]any); ok {
			for tbl, cols := range excluded {
				if colList, ok := cols.([]any); ok {
					var strCols []string
					for _, c := range colList {
						if cStr, ok := c.(string); ok {
							strCols = append(strCols, cStr)
						}
					}
					extractOpts.ExcludedColumns[tbl] = strCols
				}
			}
		}
	}

	if payload.Mode != "" {
		extractOpts.Mode = payload.Mode
	}

	// 5. Extract database schema
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, 20, 0, 0)
	}

	meta, err := conn.ExtractSchema(ctx, cfg, extractOpts)
	if err != nil {
		p.failJob(ctx, jobID, projectID, sourceID, conn.SanitizeError(err))
		return conn.SanitizeError(err)
	}

	// 6. Generate schema documents
	var scannedFiles []*ingest.ScannedFile
	schemaDocs := p.normalizer.GenerateSchemaDocuments(meta)
	scannedFiles = append(scannedFiles, schemaDocs...)

	// 7. If row-level ingestion is enabled, extract rows for configured tables
	if extractOpts.Mode == "schema_and_data" {
		for _, schema := range meta.Schemas {
			for _, table := range schema.Tables {
				excludedCols := extractOpts.ExcludedColumns[table.Name]
				allowedCols := connector.FilterSensitiveColumns(table.Columns, excludedCols, true)
				if len(allowedCols) == 0 {
					continue
				}

				batchSize := 1000
				maxRows := 10000
				offset := 0
				batchIndex := 1

				for offset < maxRows {
					rows, err := conn.ExtractRows(ctx, cfg, schema.Name, table.Name, allowedCols, batchSize, offset)
					if err != nil || len(rows) == 0 {
						break
					}

					rowDoc := p.normalizer.GenerateRowBatchDocument(
						meta.DatabaseType, meta.DatabaseName, schema.Name, table.Name, batchIndex, rows,
					)
					if rowDoc != nil {
						scannedFiles = append(scannedFiles, rowDoc)
					}

					offset += len(rows)
					batchIndex++
					if len(rows) < batchSize {
						break
					}
				}
			}
		}
	}

	totalFiles := len(scannedFiles)
	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, 40, 0, totalFiles)
	}

	// 8. Query existing documents for incremental diffing
	var existingDocs []*ent.Document
	if p.docRepo != nil {
		docs, _, err := p.docRepo.ListByProjectID(ctx, projectID, "", 1, 10000)
		if err == nil {
			for _, d := range docs {
				if d.SourceID == sourceID {
					existingDocs = append(existingDocs, d)
				}
			}
		}
	}

	diff := ingest.ComputeSyncDiff(ctx, existingDocs, scannedFiles)

	// 9. Process Deleted documents
	for _, del := range diff.Deleted {
		if p.vectorRepo != nil {
			_ = p.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, del.ID)
		}
		if p.docRepo != nil {
			_ = p.docRepo.Delete(ctx, del.ID, projectID)
		}
	}

	// 10. Process Added and Modified files
	filesToProcess := append(diff.Added, diff.Modified...)
	processedCount := 0

	for i, file := range filesToProcess {
		if strings.TrimSpace(file.Content) == "" {
			processedCount++
			continue
		}

		rawChunks := p.chunker.ChunkText(file.Content, file.Language)
		if len(rawChunks) == 0 {
			processedCount++
			continue
		}

		var docID uuid.UUID
		isExisting := false
		if p.docRepo != nil {
			existingDoc, err := p.docRepo.GetByPath(ctx, projectID, sourceID, file.Path)
			if err == nil && existingDoc != nil {
				docID = existingDoc.ID
				isExisting = true
			} else {
				docID = uuid.New()
				if _, err := p.docRepo.Create(ctx, &ent.Document{
					ID:          docID,
					ProjectID:   projectID,
					SourceID:    sourceID,
					FilePath:    file.Path,
					Language:    file.Language,
					ContentHash: file.ContentHash,
				}); err != nil {
					p.logger.Error("failed to create document record for database table",
						zap.Error(err),
						zap.String("path", file.Path),
					)
					processedCount++
					continue
				}
			}
		} else {
			docID = uuid.New()
		}

		// Prepare document chunks
		var docChunks []*model.DocumentChunk
		var chunkTexts []string

		for _, rc := range rawChunks {
			cHash := sha256.Sum256([]byte(rc.Content))
			cHashStr := hex.EncodeToString(cHash[:])

			docChunks = append(docChunks, &model.DocumentChunk{
				ID:          uuid.New(),
				ProjectID:   projectID,
				DocumentID:  docID,
				ChunkIndex:  rc.Index,
				StartLine:   rc.StartLine,
				EndLine:     rc.EndLine,
				Content:     rc.Content,
				ContentHash: cHashStr,
				TokenCount:  rc.TokenCount,
			})
			chunkTexts = append(chunkTexts, rc.Content)
		}

		// Generate embeddings
		if p.embedder != nil && len(chunkTexts) > 0 {
			embeddings, err := p.embedder.EmbedDocuments(ctx, chunkTexts)
			if err != nil {
				p.logger.Error("failed to generate embeddings for database table chunk",
					zap.Error(err),
					zap.String("path", file.Path),
				)
			} else {
				for ci, emb := range embeddings {
					if ci < len(docChunks) {
						docChunks[ci].Embedding = emb
					}
				}
			}
		}

		// Delete old chunks for existing document before saving newly embedded chunks
		if isExisting && p.vectorRepo != nil {
			_ = p.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, docID)
		}

		// Upsert vectors
		if p.vectorRepo != nil && len(docChunks) > 0 {
			if err := p.vectorRepo.UpsertChunks(ctx, docChunks); err != nil {
				p.logger.Error("failed to upsert database table vectors",
					zap.Error(err),
					zap.String("path", file.Path),
				)
			}
		}

		// Update document chunk count and content hash for existing document
		if isExisting && p.docRepo != nil {
			_, _ = p.docRepo.UpdateContentHashAndChunks(ctx, docID, projectID, file.ContentHash, len(docChunks))
		}

		processedCount++
		if p.jobRepo != nil && jobID != uuid.Nil && totalFiles > 0 {
			progress := 40 + int(float64(i+1)/float64(len(filesToProcess))*55.0)
			_ = p.jobRepo.UpdateProgress(ctx, jobID, projectID, progress, processedCount, totalFiles)
		}
	}

	// 11. Finalize source and job status
	now := time.Now()
	_ = p.client.Source.UpdateOneID(sourceID).
		SetSyncStatus(source.SyncStatusSynced).
		SetLastSyncedAt(now).
		Exec(ctx)

	_ = p.client.DatabaseSource.UpdateOneID(dbSrc.ID).
		SetStatus("ready").
		SetLastSyncedAt(now).
		SetLastError("").
		Exec(ctx)

	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.Complete(ctx, jobID, projectID)
	}

	p.logger.Info("database sync completed successfully",
		zap.String("project_id", projectID.String()),
		zap.String("source_id", sourceID.String()),
		zap.Int("tables_indexed", totalFiles),
	)

	return nil
}

func (p *DatabaseIngestionPipeline) failJob(ctx context.Context, jobID, projectID, sourceID uuid.UUID, err error) {
	p.logger.Error("database ingestion failed",
		zap.String("job_id", jobID.String()),
		zap.String("source_id", sourceID.String()),
		zap.Error(err),
	)

	if p.jobRepo != nil && jobID != uuid.Nil {
		_ = p.jobRepo.Fail(ctx, jobID, projectID, err.Error())
	}

	_ = p.client.Source.UpdateOneID(sourceID).
		SetSyncStatus(source.SyncStatusFailed).
		Exec(ctx)

	_ = p.client.DatabaseSource.Update().
		Where(databasesource.SourceIDEQ(sourceID)).
		SetStatus("failed").
		SetLastError(err.Error()).
		Exec(ctx)
}
