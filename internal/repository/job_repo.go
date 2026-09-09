package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/ingestionjob"
)

type JobRepository interface {
	Create(ctx context.Context, j *ent.IngestionJob) (*ent.IngestionJob, error)
	GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.IngestionJob, error)
	GetByIDOnly(ctx context.Context, id uuid.UUID) (*ent.IngestionJob, error)
	ListByProjectID(ctx context.Context, projectID uuid.UUID, limit int) ([]*ent.IngestionJob, error)
	UpdateProgress(ctx context.Context, id, projectID uuid.UUID, percent, processed, total int) error
	Complete(ctx context.Context, id, projectID uuid.UUID) error
	Fail(ctx context.Context, id, projectID uuid.UUID, errorMsg string) error
}

type EntJobRepository struct {
	client *ent.Client
}

func NewJobRepository(client *ent.Client) *EntJobRepository {
	return &EntJobRepository{client: client}
}

func (r *EntJobRepository) Create(ctx context.Context, j *ent.IngestionJob) (*ent.IngestionJob, error) {
	builder := r.client.IngestionJob.Create().
		SetProjectID(j.ProjectID).
		SetSourceID(j.SourceID).
		SetStatus(j.Status).
		SetProgressPercent(j.ProgressPercent).
		SetProcessedFiles(j.ProcessedFiles).
		SetTotalFiles(j.TotalFiles)

	if j.ID != uuid.Nil {
		builder.SetID(j.ID)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating job: %w", err)
	}
	return created, nil
}

func (r *EntJobRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.IngestionJob, error) {
	job, err := r.client.IngestionJob.Query().
		Where(
			ingestionjob.IDEQ(id),
			ingestionjob.ProjectIDEQ(projectID),
		).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting job: %w", err)
	}
	return job, nil
}

func (r *EntJobRepository) GetByIDOnly(ctx context.Context, id uuid.UUID) (*ent.IngestionJob, error) {
	job, err := r.client.IngestionJob.Query().
		Where(ingestionjob.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting job: %w", err)
	}
	return job, nil
}

func (r *EntJobRepository) ListByProjectID(ctx context.Context, projectID uuid.UUID, limit int) ([]*ent.IngestionJob, error) {
	if limit <= 0 {
		limit = 20
	}
	jobs, err := r.client.IngestionJob.Query().
		Where(ingestionjob.ProjectIDEQ(projectID)).
		Order(ent.Desc(ingestionjob.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	return jobs, nil
}

func (r *EntJobRepository) UpdateProgress(ctx context.Context, id, projectID uuid.UUID, percent, processed, total int) error {
	err := r.client.IngestionJob.UpdateOneID(id).
		Where(ingestionjob.ProjectIDEQ(projectID)).
		SetStatus(ingestionjob.StatusRunning).
		SetProgressPercent(percent).
		SetProcessedFiles(processed).
		SetTotalFiles(total).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("updating job progress: %w", err)
	}
	return nil
}

func (r *EntJobRepository) Complete(ctx context.Context, id, projectID uuid.UUID) error {
	now := time.Now()
	err := r.client.IngestionJob.UpdateOneID(id).
		Where(ingestionjob.ProjectIDEQ(projectID)).
		SetStatus(ingestionjob.StatusCompleted).
		SetProgressPercent(100).
		SetFinishedAt(now).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("completing job: %w", err)
	}
	return nil
}

func (r *EntJobRepository) Fail(ctx context.Context, id, projectID uuid.UUID, errorMsg string) error {
	now := time.Now()
	err := r.client.IngestionJob.UpdateOneID(id).
		Where(ingestionjob.ProjectIDEQ(projectID)).
		SetStatus(ingestionjob.StatusFailed).
		SetErrorMessage(errorMsg).
		SetFinishedAt(now).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failing job: %w", err)
	}
	return nil
}
