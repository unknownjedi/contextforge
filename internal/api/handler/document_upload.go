package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/docparser"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
)

// DocumentUploadResponse encapsulates the newly created document and indexed chunk count.
type DocumentUploadResponse struct {
	Document   *ent.Document `json:"document"`
	ChunkCount int           `json:"chunk_count"`
}

// Embedder defines the batch embedding contract for document chunk indexing.
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// providerEmbedderAdapter adapts provider.EmbeddingProvider to the Embedder interface.
type providerEmbedderAdapter struct {
	provider provider.EmbeddingProvider
}

func (a *providerEmbedderAdapter) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if batcher, ok := a.provider.(Embedder); ok {
		return batcher.EmbedBatch(ctx, texts)
	}
	return a.provider.EmbedDocuments(ctx, texts)
}

// DocumentUploadHandler processes manual document file uploads.
type DocumentUploadHandler struct {
	docRepo    repository.DocumentRepository
	sourceRepo repository.SourceRepository
	vectorRepo repository.VectorRepository
	embedder   Embedder
	chunker    chunk.Chunker
	parser     docparser.Parser
	logger     *zap.Logger
}

// NewDocumentUploadHandler constructs a DocumentUploadHandler.
func NewDocumentUploadHandler(
	docRepo repository.DocumentRepository,
	sourceRepo repository.SourceRepository,
	vectorRepo repository.VectorRepository,
	embedder provider.EmbeddingProvider,
	chunker chunk.Chunker,
	logger *zap.Logger,
) *DocumentUploadHandler {
	if chunker == nil {
		chunker = chunk.NewChunker(chunk.DefaultOptions())
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	var emb Embedder
	if embedder != nil {
		if b, ok := embedder.(Embedder); ok {
			emb = b
		} else {
			emb = &providerEmbedderAdapter{provider: embedder}
		}
	}

	return &DocumentUploadHandler{
		docRepo:    docRepo,
		sourceRepo: sourceRepo,
		vectorRepo: vectorRepo,
		embedder:   emb,
		chunker:    chunker,
		parser:     docparser.NewParser(),
		logger:     logger,
	}
}

// SetParser allows overriding the parser (useful in unit testing).
func (h *DocumentUploadHandler) SetParser(p docparser.Parser) {
	h.parser = p
}

// UploadDocument handles POST /projects/:id/documents/upload and POST /projects/:id/documents.
func (h *DocumentUploadHandler) UploadDocument(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
		return
	}

	// Sanitize filename and prevent path traversal
	cleanedFilename := filepath.Base(filepath.Clean(fileHeader.Filename))
	if cleanedFilename == "" || cleanedFilename == "." || cleanedFilename == "/" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}

	ext := strings.ToLower(filepath.Ext(cleanedFilename))
	var lang string
	switch ext {
	case ".md", ".markdown":
		lang = "markdown"
	case ".json":
		lang = "json"
	case ".csv":
		lang = "csv"
	case ".pdf":
		lang = "pdf"
	case ".txt", ".text":
		lang = "text"
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("unsupported file extension %q (supported: .md, .txt, .json, .csv, .pdf)", ext),
		})
		return
	}

	// Size limit pre-check if provided by header
	if fileHeader.Size > docparser.MaxFileSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": docparser.ErrFileTooLarge.Error()})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		h.logger.Error("failed to open uploaded file", zap.Error(err), zap.String("filename", cleanedFilename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer func() { _ = file.Close() }()

	// Parse file content with docparser
	ctx := c.Request.Context()
	content, err := h.parser.Parse(ctx, cleanedFilename, file)
	if err != nil {
		if errors.Is(err, docparser.ErrFileTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, docparser.ErrEmptyFile) || errors.Is(err, docparser.ErrNoExtractableText) ||
			errors.Is(err, docparser.ErrInvalidUTF8) || errors.Is(err, docparser.ErrUnsupportedFormat) ||
			errors.Is(err, docparser.ErrInvalidPDF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("parsing document: %v", err)})
		return
	}

	if strings.TrimSpace(content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document contains no extractable text"})
		return
	}

	// Find or create an upload source for the project
	sourceID, err := h.getOrCreateUploadSource(ctx, projectID)
	if err != nil {
		h.logger.Error("failed to ensure manual upload source", zap.Error(err), zap.String("project_id", projectID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize upload source"})
		return
	}

	// Compute content hash
	contentHash := chunk.HashContent(content)

	// Create or update Document record
	doc, isExisting, err := h.getOrCreateDocument(ctx, projectID, sourceID, cleanedFilename, lang, contentHash)
	if err != nil {
		h.logger.Error("failed to create or retrieve document record", zap.Error(err), zap.String("filename", cleanedFilename))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store document metadata"})
		return
	}

	// Chunk content using chunker
	rawChunks := h.chunker.ChunkText(content, lang)
	if len(rawChunks) == 0 {
		rawChunks = []chunk.Chunk{
			{
				Index:       0,
				StartLine:   1,
				EndLine:     max(1, len(strings.Split(content, "\n"))),
				Content:     content,
				ContentHash: contentHash,
				TokenCount:  chunk.EstimateTokens(content),
			},
		}
	}

	// Generate embeddings using embedder.EmbedBatch
	texts := make([]string, len(rawChunks))
	for i, rc := range rawChunks {
		texts[i] = rc.Content
	}

	var embeddings [][]float32
	if h.embedder != nil && len(texts) > 0 {
		embeddings, err = h.embedder.EmbedBatch(ctx, texts)
		if err != nil {
			h.logger.Error("failed to generate embeddings", zap.Error(err), zap.String("filename", cleanedFilename))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate embeddings for document"})
			return
		}
	}

	now := time.Now()
	docChunks := make([]*model.DocumentChunk, len(rawChunks))
	for i, rc := range rawChunks {
		var emb []float32
		if i < len(embeddings) {
			emb = embeddings[i]
		}
		docChunks[i] = &model.DocumentChunk{
			ID:          uuid.New(),
			ProjectID:   projectID,
			DocumentID:  doc.ID,
			ChunkIndex:  rc.Index,
			StartLine:   rc.StartLine,
			EndLine:     rc.EndLine,
			Content:     rc.Content,
			ContentHash: rc.ContentHash,
			TokenCount:  rc.TokenCount,
			Embedding:   emb,
			CreatedAt:   now,
		}
	}

	// Store chunks in vector repository
	if h.vectorRepo != nil {
		if isExisting {
			_ = h.vectorRepo.DeleteChunksByDocumentID(ctx, projectID, doc.ID)
		}
		if len(docChunks) > 0 {
			if err := h.vectorRepo.UpsertChunks(ctx, docChunks); err != nil {
				h.logger.Error("failed to upsert vector chunks", zap.Error(err), zap.String("filename", cleanedFilename))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store document chunks"})
				return
			}
		}
	}

	// Update document total chunks and hash
	if h.docRepo != nil {
		updatedDoc, err := h.docRepo.UpdateContentHashAndChunks(ctx, doc.ID, projectID, contentHash, len(docChunks))
		if err == nil && updatedDoc != nil {
			doc = updatedDoc
		} else {
			doc.TotalChunks = len(docChunks)
			doc.ContentHash = contentHash
		}
	}

	h.logger.Info("successfully uploaded and indexed document",
		zap.String("project_id", projectID.String()),
		zap.String("doc_id", doc.ID.String()),
		zap.String("filename", cleanedFilename),
		zap.Int("chunk_count", len(docChunks)),
	)

	c.JSON(http.StatusCreated, DocumentUploadResponse{
		Document:   doc,
		ChunkCount: len(docChunks),
	})
}

// getOrCreateUploadSource locates or creates the manual upload Source entity for a project.
func (h *DocumentUploadHandler) getOrCreateUploadSource(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if h.sourceRepo == nil {
		return uuid.Nil, fmt.Errorf("source repository not configured")
	}

	sources, err := h.sourceRepo.ListByProjectID(ctx, projectID)
	if err == nil {
		for _, s := range sources {
			if s.Type == "upload" || s.Name == "Manual Uploads" {
				return s.ID, nil
			}
		}
	}

	now := time.Now()
	newSource, err := h.sourceRepo.Create(ctx, &ent.Source{
		ID:           uuid.New(),
		ProjectID:    projectID,
		Name:         "Manual Uploads",
		Type:         "upload",
		SyncStatus:   source.SyncStatusSynced,
		LastSyncedAt: &now,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating upload source: %w", err)
	}

	return newSource.ID, nil
}

// getOrCreateDocument returns existing document or creates a new one.
func (h *DocumentUploadHandler) getOrCreateDocument(
	ctx context.Context,
	projectID, sourceID uuid.UUID,
	filename, lang, contentHash string,
) (*ent.Document, bool, error) {
	if h.docRepo == nil {
		return &ent.Document{
			ID:          uuid.New(),
			ProjectID:   projectID,
			SourceID:    sourceID,
			FilePath:    filename,
			Language:    lang,
			ContentHash: contentHash,
			TotalChunks: 0,
		}, false, nil
	}

	existingDoc, err := h.docRepo.GetByPath(ctx, projectID, sourceID, filename)
	if err == nil && existingDoc != nil {
		return existingDoc, true, nil
	}

	docID := uuid.New()
	created, err := h.docRepo.Create(ctx, &ent.Document{
		ID:          docID,
		ProjectID:   projectID,
		SourceID:    sourceID,
		FilePath:    filename,
		Language:    lang,
		ContentHash: contentHash,
		TotalChunks: 0,
	})
	if err != nil {
		return nil, false, fmt.Errorf("creating document entity: %w", err)
	}

	return created, false, nil
}
