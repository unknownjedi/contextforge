package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	TypeRepoSync     = "repo:sync"
	TypeDocEmbed     = "doc:embed"
	TypeDatabaseSync = "database:sync"
	TypeURLSync      = "url:sync"

	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// URLSyncPayload contains parameters for web URL content synchronization.
type URLSyncPayload struct {
	JobID     uuid.UUID `json:"job_id"`
	ProjectID uuid.UUID `json:"project_id"`
	SourceID  uuid.UUID `json:"source_id"`
	URL       string    `json:"url"`
}

// DatabaseSyncPayload contains parameters for external database synchronization.
type DatabaseSyncPayload struct {
	JobID     uuid.UUID `json:"job_id"`
	ProjectID uuid.UUID `json:"project_id"`
	SourceID  uuid.UUID `json:"source_id"`
	Mode      string    `json:"mode,omitempty"`
}

// RepoSyncPayload contains parameters for repository synchronization task.
type RepoSyncPayload struct {
	JobID      uuid.UUID `json:"job_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	SourceID   uuid.UUID `json:"source_id"`
	ForceFull  bool      `json:"force_full"`
	CommitHash string    `json:"commit_hash,omitempty"`
}

// DocEmbedPayload contains parameters for document chunk embedding task.
type DocEmbedPayload struct {
	JobID      uuid.UUID `json:"job_id"`
	ProjectID  uuid.UUID `json:"project_id"`
	DocumentID uuid.UUID `json:"document_id"`
}

// NewRepoSyncTask creates an Asynq task for repository syncing with exponential retries and unique key.
func NewRepoSyncTask(payload RepoSyncPayload) (*asynq.Task, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling repo sync payload: %w", err)
	}

	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Minute),
	}

	if payload.CommitHash != "" {
		// Enforce task deduplication if syncing identical commit
		uniqueKey := fmt.Sprintf("sync:%s:%s", payload.SourceID, payload.CommitHash)
		opts = append(opts, asynq.Unique(5*time.Minute), asynq.TaskID(uniqueKey))
	}

	return asynq.NewTask(TypeRepoSync, bytes, opts...), nil
}

// NewDocEmbedTask creates an Asynq task for document embedding.
func NewDocEmbedTask(payload DocEmbedPayload) (*asynq.Task, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling doc embed payload: %w", err)
	}

	return asynq.NewTask(TypeDocEmbed, bytes,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Minute),
	), nil
}

// NewDatabaseSyncTask creates an Asynq task for database schema and data synchronization.
func NewDatabaseSyncTask(payload DatabaseSyncPayload) (*asynq.Task, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling database sync payload: %w", err)
	}

	return asynq.NewTask(TypeDatabaseSync, bytes,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
	), nil
}

// NewURLSyncTask creates an Asynq task for web URL content synchronization.
func NewURLSyncTask(payload URLSyncPayload) (*asynq.Task, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling url sync payload: %w", err)
	}

	return asynq.NewTask(TypeURLSync, bytes,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Minute),
	), nil
}

