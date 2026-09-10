package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/docparser"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
)

func setupUploadRouter(h *handler.DocumentUploadHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/projects/:id/documents/upload", h.UploadDocument)
	r.POST("/projects/:id/documents", h.UploadDocument)
	return r
}

func createMultipartRequest(url, fieldName, filename string, fileContent []byte) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(fileContent); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func TestDocumentUploadHandler_MarkdownUpload(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	mdContent := "# Architecture Design\n\nThis is a production-grade architecture spec for ContextForge.\n\n## Components\n- Ingestion\n- RAG Service"
	req, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "architecture.md", []byte(mdContent))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, resp.Document.ID)
	assert.Equal(t, projectID, resp.Document.ProjectID)
	assert.Equal(t, "architecture.md", resp.Document.FilePath)
	assert.Equal(t, "markdown", resp.Document.Language)
	assert.Greater(t, resp.ChunkCount, 0)
	assert.Equal(t, resp.ChunkCount, resp.Document.TotalChunks)

	// Verify upload source was created
	sources, err := sourceRepo.ListByProjectID(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "Manual Uploads", sources[0].Name)
	assert.Equal(t, "upload", sources[0].Type)

	// Verify chunks stored in vector repository
	chunks, err := vectorRepo.GetChunksByDocumentID(context.Background(), projectID, resp.Document.ID)
	require.NoError(t, err)
	assert.Equal(t, resp.ChunkCount, len(chunks))
	for _, c := range chunks {
		assert.Equal(t, 768, len(c.Embedding))
		assert.NotEmpty(t, c.Content)
	}
}

func TestDocumentUploadHandler_PlainTextUpload(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	txtContent := "ContextForge text log file line 1\nContextForge text log file line 2"
	req, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents", projectID), "file", "notes.txt", []byte(txtContent))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "notes.txt", resp.Document.FilePath)
	assert.Equal(t, "text", resp.Document.Language)
	assert.Greater(t, resp.ChunkCount, 0)
}

func TestDocumentUploadHandler_JSONUpload(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	jsonContent := `{"server":{"port":8080,"name":"ContextForge"},"enabled":true}`
	req, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "schema.json", []byte(jsonContent))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "schema.json", resp.Document.FilePath)
	assert.Equal(t, "json", resp.Document.Language)
}

func TestDocumentUploadHandler_CSVUpload(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	csvContent := "ID,Name,Role\n101,Alice,Architect\n102,Bob,Engineer"
	req, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "employees.csv", []byte(csvContent))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "employees.csv", resp.Document.FilePath)
	assert.Equal(t, "csv", resp.Document.Language)

	// Verify chunk has markdown table
	chunks, err := vectorRepo.GetChunksByDocumentID(context.Background(), projectID, resp.Document.ID)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
	assert.Contains(t, chunks[0].Content, "| ID | Name | Role |")
}

func TestDocumentUploadHandler_PDFUpload(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	pdfStream := "BT /F1 12 Tf (Manual Document Upload PDF Ingestion) Tj ET"
	pdfContent := fmt.Sprintf("%%PDF-1.4\n1 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%%%EOF", len(pdfStream), pdfStream)

	req, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "guide.pdf", []byte(pdfContent))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "guide.pdf", resp.Document.FilePath)
	assert.Equal(t, "pdf", resp.Document.Language)
	assert.Greater(t, resp.ChunkCount, 0)
}

func TestDocumentUploadHandler_PathTraversalPrevention(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	req, err := createMultipartRequest(
		fmt.Sprintf("/projects/%s/documents/upload", projectID),
		"file",
		"../../../../etc/passwd.txt",
		[]byte("safe content"),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.DocumentUploadResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Verify path was sanitized to base filename
	assert.Equal(t, "passwd.txt", resp.Document.FilePath)
	assert.False(t, strings.Contains(resp.Document.FilePath, ".."))
	assert.False(t, strings.Contains(resp.Document.FilePath, "/"))
}

func TestDocumentUploadHandler_ReuploadUpdatesDocument(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	// First upload
	req1, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "version.txt", []byte("Version 1.0 Content"))
	require.NoError(t, err)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusCreated, w1.Code)

	var resp1 handler.DocumentUploadResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)

	// Second upload with updated content
	req2, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "version.txt", []byte("Version 2.0 Content Updated"))
	require.NoError(t, err)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusCreated, w2.Code)

	var resp2 handler.DocumentUploadResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)

	// Document ID should be preserved
	assert.Equal(t, resp1.Document.ID, resp2.Document.ID)
	// Content hash should change
	assert.NotEqual(t, resp1.Document.ContentHash, resp2.Document.ContentHash)

	// Source count should still be 1
	sources, err := sourceRepo.ListByProjectID(context.Background(), projectID)
	require.NoError(t, err)
	assert.Len(t, sources, 1)
}

func TestDocumentUploadHandler_ValidationErrors(t *testing.T) {
	projectID := uuid.New()
	docRepo := newMockDocRepo()
	sourceRepo := newMockSourceRepo()
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	h := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, zap.NewNop())
	router := setupUploadRouter(h)

	// 1. Invalid project ID
	req, err := createMultipartRequest("/projects/invalid-uuid/documents/upload", "file", "doc.txt", []byte("content"))
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid project ID")

	// 2. Missing file field
	reqEmpty, err := http.NewRequest(http.MethodPost, fmt.Sprintf("/projects/%s/documents/upload", projectID), strings.NewReader(""))
	require.NoError(t, err)
	reqEmpty.Header.Set("Content-Type", "multipart/form-data")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, reqEmpty)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 3. Unsupported extension (.exe)
	reqUnsupported, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "binary.exe", []byte("executable"))
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, reqUnsupported)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unsupported file extension")

	// 4. Empty file
	reqEmptyFile, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "empty.txt", []byte{})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, reqEmptyFile)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), docparser.ErrEmptyFile.Error())

	// 5. Oversized file (>10MB)
	oversized := bytes.Repeat([]byte("X"), int(docparser.MaxFileSize)+10)
	reqOversized, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "large.txt", oversized)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, reqOversized)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), docparser.ErrFileTooLarge.Error())

	// 6. Invalid UTF-8 text file
	reqInvalidUTF8, err := createMultipartRequest(fmt.Sprintf("/projects/%s/documents/upload", projectID), "file", "broken.txt", []byte{0xff, 0xfe, 0xfd})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, reqInvalidUTF8)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), docparser.ErrInvalidUTF8.Error())
}
