package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/project"
)

type ProjectRepository interface {
	Create(ctx context.Context, p *ent.Project) (*ent.Project, error)
	GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error)
	ListByOwner(ctx context.Context, ownerUserID uuid.UUID, page, pageSize int) ([]*ent.Project, int, error)
	Update(ctx context.Context, p *ent.Project, ownerUserID uuid.UUID) (*ent.Project, error)
	Delete(ctx context.Context, id, ownerUserID uuid.UUID) error
}

type EntProjectRepository struct {
	client *ent.Client
}

func NewProjectRepository(client *ent.Client) *EntProjectRepository {
	return &EntProjectRepository{client: client}
}

func (r *EntProjectRepository) Create(ctx context.Context, p *ent.Project) (*ent.Project, error) {
	builder := r.client.Project.Create().
		SetName(p.Name).
		SetDescription(p.Description).
		SetOwnerUserID(p.OwnerUserID).
		SetEmbeddingProvider(p.EmbeddingProvider).
		SetEmbeddingModel(p.EmbeddingModel).
		SetEmbeddingDimension(p.EmbeddingDimension).
		SetLlmProvider(p.LlmProvider).
		SetLlmModel(p.LlmModel)

	if p.ID != uuid.Nil {
		builder.SetID(p.ID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return created, nil
}

// GetByID retrieves a project strictly verifying that ownerUserID matches the project owner.
func (r *EntProjectRepository) GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error) {
	p, err := r.client.Project.Query().
		Where(
			project.IDEQ(id),
			project.OwnerUserIDEQ(ownerUserID),
		).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting project: %w", err)
	}
	return p, nil
}

// ListByOwner returns paginated projects owned strictly by ownerUserID.
func (r *EntProjectRepository) ListByOwner(ctx context.Context, ownerUserID uuid.UUID, page, pageSize int) ([]*ent.Project, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := r.client.Project.Query().Where(project.OwnerUserIDEQ(ownerUserID))

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("counting projects: %w", err)
	}

	items, err := query.Order(ent.Desc(project.FieldCreatedAt)).
		Offset(offset).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("listing projects: %w", err)
	}

	return items, total, nil
}

func (r *EntProjectRepository) Update(ctx context.Context, p *ent.Project, ownerUserID uuid.UUID) (*ent.Project, error) {
	updater := r.client.Project.UpdateOneID(p.ID).
		Where(project.OwnerUserIDEQ(ownerUserID)).
		SetName(p.Name).
		SetDescription(p.Description).
		SetLlmProvider(p.LlmProvider).
		SetLlmModel(p.LlmModel)

	updated, err := updater.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating project: %w", err)
	}
	return updated, nil
}

func (r *EntProjectRepository) Delete(ctx context.Context, id, ownerUserID uuid.UUID) error {
	deletedCount, err := r.client.Project.Delete().
		Where(
			project.IDEQ(id),
			project.OwnerUserIDEQ(ownerUserID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting project: %w", err)
	}
	if deletedCount == 0 {
		return fmt.Errorf("project not found or unauthorized")
	}
	return nil
}
