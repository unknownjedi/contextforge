package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/document"
)

type DocumentRepository interface {
	Create(ctx context.Context, d *ent.Document) (*ent.Document, error)
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Document, error)
	GetByPath(ctx context.Context, projectID, sourceID uuid.UUID, filePath string) (*ent.Document, error)
	ListByProjectID(ctx context.Context, projectID uuid.UUID, filePathFilter string, page, pageSize int) ([]*ent.Document, int, error)
	UpdateContentHashAndChunks(ctx context.Context, id, projectID uuid.UUID, contentHash string, totalChunks int) (*ent.Document, error)
	Delete(ctx context.Context, id, projectID uuid.UUID) error
	DeleteBySourceID(ctx context.Context, projectID, sourceID uuid.UUID) error
}

type EntDocumentRepository struct {
	client *ent.Client
}

func NewDocumentRepository(client *ent.Client) *EntDocumentRepository {
	return &EntDocumentRepository{client: client}
}

func (r *EntDocumentRepository) Create(ctx context.Context, d *ent.Document) (*ent.Document, error) {
	builder := r.client.Document.Create().
		SetProjectID(d.ProjectID).
		SetSourceID(d.SourceID).
		SetFilePath(d.FilePath).
		SetLanguage(d.Language).
		SetContentHash(d.ContentHash).
		SetTotalChunks(d.TotalChunks)

	if d.ID != uuid.Nil {
		builder.SetID(d.ID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating document: %w", err)
	}
	return created, nil
}

func (r *EntDocumentRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Document, error) {
	doc, err := r.client.Document.Query().
		Where(
			document.IDEQ(id),
			document.ProjectIDEQ(projectID),
		).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting document: %w", err)
	}
	return doc, nil
}

func (r *EntDocumentRepository) GetByPath(ctx context.Context, projectID, sourceID uuid.UUID, filePath string) (*ent.Document, error) {
	doc, err := r.client.Document.Query().
		Where(
			document.ProjectIDEQ(projectID),
			document.SourceIDEQ(sourceID),
			document.FilePathEQ(filePath),
		).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting document by path: %w", err)
	}
	return doc, nil
}

func (r *EntDocumentRepository) ListByProjectID(ctx context.Context, projectID uuid.UUID, filePathFilter string, page, pageSize int) ([]*ent.Document, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := r.client.Document.Query().Where(document.ProjectIDEQ(projectID))
	if filePathFilter != "" {
		query = query.Where(document.FilePathContains(filePathFilter))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("counting documents: %w", err)
	}

	items, err := query.Order(ent.Asc(document.FieldFilePath)).
		Offset(offset).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("listing documents: %w", err)
	}

	return items, total, nil
}

func (r *EntDocumentRepository) UpdateContentHashAndChunks(ctx context.Context, id, projectID uuid.UUID, contentHash string, totalChunks int) (*ent.Document, error) {
	doc, err := r.client.Document.UpdateOneID(id).
		Where(document.ProjectIDEQ(projectID)).
		SetContentHash(contentHash).
		SetTotalChunks(totalChunks).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating document: %w", err)
	}
	return doc, nil
}

func (r *EntDocumentRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	deleted, err := r.client.Document.Delete().
		Where(
			document.IDEQ(id),
			document.ProjectIDEQ(projectID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}
	if deleted == 0 {
		return fmt.Errorf("document not found")
	}
	return nil
}

func (r *EntDocumentRepository) DeleteBySourceID(ctx context.Context, projectID, sourceID uuid.UUID) error {
	_, err := r.client.Document.Delete().
		Where(
			document.ProjectIDEQ(projectID),
			document.SourceIDEQ(sourceID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting documents by source: %w", err)
	}
	return nil
}
