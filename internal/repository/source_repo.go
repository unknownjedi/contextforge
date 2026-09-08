package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/source"
)

type SourceRepository interface {
	Create(ctx context.Context, s *ent.Source) (*ent.Source, error)
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Source, error)
	ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Source, error)
	UpdateSyncStatus(ctx context.Context, id, projectID uuid.UUID, status source.SyncStatus, commitHash string, syncedAt *time.Time) (*ent.Source, error)
	Delete(ctx context.Context, id, projectID uuid.UUID) error
}

type EntSourceRepository struct {
	client *ent.Client
}

func NewSourceRepository(client *ent.Client) *EntSourceRepository {
	return &EntSourceRepository{client: client}
}

func (r *EntSourceRepository) Create(ctx context.Context, s *ent.Source) (*ent.Source, error) {
	builder := r.client.Source.Create().
		SetProjectID(s.ProjectID).
		SetName(s.Name).
		SetType(s.Type).
		SetRepoOwner(s.RepoOwner).
		SetRepoName(s.RepoName).
		SetBranch(s.Branch).
		SetSyncStatus(s.SyncStatus)

	if s.ID != uuid.Nil {
		builder.SetID(s.ID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating source: %w", err)
	}
	return created, nil
}

func (r *EntSourceRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.Source, error) {
	s, err := r.client.Source.Query().
		Where(
			source.IDEQ(id),
			source.ProjectIDEQ(projectID),
		).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	return s, nil
}

func (r *EntSourceRepository) ListByProjectID(ctx context.Context, projectID uuid.UUID) ([]*ent.Source, error) {
	sources, err := r.client.Source.Query().
		Where(source.ProjectIDEQ(projectID)).
		Order(ent.Asc(source.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	return sources, nil
}

func (r *EntSourceRepository) UpdateSyncStatus(ctx context.Context, id, projectID uuid.UUID, status source.SyncStatus, commitHash string, syncedAt *time.Time) (*ent.Source, error) {
	updater := r.client.Source.UpdateOneID(id).
		Where(source.ProjectIDEQ(projectID)).
		SetSyncStatus(status)

	if commitHash != "" {
		updater.SetLastCommitHash(commitHash)
	}
	if syncedAt != nil {
		updater.SetLastSyncedAt(*syncedAt)
	}

	updated, err := updater.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("updating source sync status: %w", err)
	}
	return updated, nil
}

func (r *EntSourceRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	deleted, err := r.client.Source.Delete().
		Where(
			source.IDEQ(id),
			source.ProjectIDEQ(projectID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("deleting source: %w", err)
	}
	if deleted == 0 {
		return fmt.Errorf("source not found")
	}
	return nil
}
