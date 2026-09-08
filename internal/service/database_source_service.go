package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/databasesource"
	"github.com/your-org/contextforge/internal/ent/ingestionjob"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
)

// CreateDatabaseSourceRequest holds payload to register a new external database source.
type CreateDatabaseSourceRequest struct {
	Name          string         `json:"name" binding:"required"`
	DatabaseType  string         `json:"database_type" binding:"required"`
	ConnectionURL string         `json:"connection_url" binding:"required"`
	Configuration map[string]any `json:"configuration,omitempty"`
}

// UpdateDatabaseSourceRequest holds payload to update configuration or name.
type UpdateDatabaseSourceRequest struct {
	Name          *string        `json:"name,omitempty"`
	Configuration map[string]any `json:"configuration,omitempty"`
}

// DatabaseSourceResponse is the sanitized public representation of a database source.
// Never exposes password, encrypted credentials, or raw connection strings.
type DatabaseSourceResponse struct {
	ID           uuid.UUID      `json:"id"`
	SourceID     uuid.UUID      `json:"source_id"`
	ProjectID    uuid.UUID      `json:"project_id"`
	Name         string         `json:"name"`
	DatabaseType string         `json:"database_type"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	DatabaseName string         `json:"database_name"`
	Username     string         `json:"username"`
	Configuration map[string]any `json:"configuration"`
	Status       string         `json:"status"`
	LastError    string         `json:"last_error,omitempty"`
	LastSyncedAt *time.Time     `json:"last_synced_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// DatabaseSourceService orchestrates external database connections, metadata introspection, and sync tasks.
type DatabaseSourceService struct {
	client          *ent.Client
	registry        *connector.ConnectorRegistry
	encryptionKey   []byte
	queueClient     *queue.Client
	jobRepo         repository.JobRepository
	allowPrivateIPs bool
	logger          *zap.Logger
}

// NewDatabaseSourceService constructs a DatabaseSourceService.
func NewDatabaseSourceService(
	client *ent.Client,
	registry *connector.ConnectorRegistry,
	encryptionKey []byte,
	queueClient *queue.Client,
	jobRepo repository.JobRepository,
	allowPrivateIPs bool,
	logger *zap.Logger,
) *DatabaseSourceService {
	if registry == nil {
		registry = connector.DefaultRegistry()
	}
	return &DatabaseSourceService{
		client:          client,
		registry:        registry,
		encryptionKey:   encryptionKey,
		queueClient:     queueClient,
		jobRepo:         jobRepo,
		allowPrivateIPs: allowPrivateIPs,
		logger:          logger,
	}
}

// TestConnection verifies an ad-hoc connection URL without storing it.
func (s *DatabaseSourceService) TestConnection(ctx context.Context, dbTypeStr, rawURL string) (*connector.ConnectionTestResult, error) {
	conn, err := s.registry.Get(connector.DatabaseType(dbTypeStr))
	if err != nil {
		return nil, err
	}

	cfg, err := conn.ParseURL(rawURL)
	if err != nil {
		return nil, conn.SanitizeError(err)
	}

	// Validate target network for SSRF
	if cfg.Type != connector.TypeSQLite {
		if err := connector.ValidateNetworkTarget(cfg.Host, cfg.Port, s.allowPrivateIPs); err != nil {
			return nil, err
		}
	}

	return conn.TestConnection(ctx, cfg)
}

// CreateDatabaseSource registers and stores a new database source with encrypted credentials.
func (s *DatabaseSourceService) CreateDatabaseSource(ctx context.Context, projectID uuid.UUID, req CreateDatabaseSourceRequest) (*DatabaseSourceResponse, error) {
	conn, err := s.registry.Get(connector.DatabaseType(req.DatabaseType))
	if err != nil {
		return nil, fmt.Errorf("invalid database type: %w", err)
	}

	cfg, err := conn.ParseURL(req.ConnectionURL)
	if err != nil {
		return nil, conn.SanitizeError(err)
	}

	// SSRF validation
	if cfg.Type != connector.TypeSQLite {
		if err := connector.ValidateNetworkTarget(cfg.Host, cfg.Port, s.allowPrivateIPs); err != nil {
			return nil, err
		}
	}

	// Test connection before persisting
	testRes, err := conn.TestConnection(ctx, cfg)
	if err != nil || (testRes != nil && !testRes.Success) {
		errMsg := "database connection failed"
		if testRes != nil && testRes.ErrorMessage != "" {
			errMsg = testRes.ErrorMessage
		} else if err != nil {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("connection test failed: %s", errMsg)
	}

	// Encrypt connection URL with AES-256-GCM
	encryptedURL, err := crypto.Encrypt([]byte(req.ConnectionURL), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting connection credentials: %w", err)
	}

	configMap := req.Configuration
	if configMap == nil {
		configMap = make(map[string]any)
	}

	// Persist in transaction
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Create Source umbrella entry
	src, err := tx.Source.Create().
		SetProjectID(projectID).
		SetName(req.Name).
		SetType("database").
		SetSyncStatus(source.SyncStatusIdle).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating source record: %w", err)
	}

	// 2. Create DatabaseSource entry
	dbSrc, err := tx.DatabaseSource.Create().
		SetProjectID(projectID).
		SetSourceID(src.ID).
		SetDatabaseType(req.DatabaseType).
		SetHost(cfg.Host).
		SetPort(cfg.Port).
		SetDatabaseName(cfg.Database).
		SetUsername(cfg.Username).
		SetEncryptedConnectionURL(encryptedURL).
		SetConfiguration(configMap).
		SetStatus("configured").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating database source record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing source creation: %w", err)
	}

	return s.toResponse(src, dbSrc), nil
}

// GetDatabaseSource retrieves a sanitized database source by projectID and sourceID (or id).
func (s *DatabaseSourceService) GetDatabaseSource(ctx context.Context, projectID, sourceID uuid.UUID) (*DatabaseSourceResponse, error) {
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		WithSource().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	return s.toResponse(dbSrc.Edges.Source, dbSrc), nil
}

// ListDatabaseSources retrieves all database sources for a project.
func (s *DatabaseSourceService) ListDatabaseSources(ctx context.Context, projectID uuid.UUID) ([]*DatabaseSourceResponse, error) {
	dbSources, err := s.client.DatabaseSource.Query().
		Where(databasesource.ProjectIDEQ(projectID)).
		WithSource().
		Order(ent.Desc(databasesource.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	var results []*DatabaseSourceResponse
	for _, ds := range dbSources {
		results = append(results, s.toResponse(ds.Edges.Source, ds))
	}
	return results, nil
}

// UpdateDatabaseSource updates the configuration or display name of a database source.
func (s *DatabaseSourceService) UpdateDatabaseSource(ctx context.Context, projectID, sourceID uuid.UUID, req UpdateDatabaseSourceRequest) (*DatabaseSourceResponse, error) {
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		WithSource().
		Only(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if req.Name != nil && *req.Name != "" {
		_, err = tx.Source.Update().
			Where(source.IDEQ(dbSrc.SourceID), source.ProjectIDEQ(projectID)).
			SetName(*req.Name).
			Save(ctx)
		if err != nil {
			return nil, err
		}
	}

	updateQuery := tx.DatabaseSource.UpdateOneID(dbSrc.ID)
	if req.Configuration != nil {
		updateQuery.SetConfiguration(req.Configuration)
	}

	updatedDBSrc, err := updateQuery.Save(ctx)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	updatedSrc, err := s.client.Source.Query().
		Where(source.IDEQ(dbSrc.SourceID), source.ProjectIDEQ(projectID)).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	return s.toResponse(updatedSrc, updatedDBSrc), nil
}

// DeleteDatabaseSource removes a database source and triggers CASCADE deletion of documents, chunks, and jobs.
func (s *DatabaseSourceService) DeleteDatabaseSource(ctx context.Context, projectID, sourceID uuid.UUID) error {
	// Verify project ownership and locate record
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		Only(ctx)
	if err != nil {
		return err
	}

	// Explicitly delete DatabaseSource record
	_, _ = s.client.DatabaseSource.Delete().
		Where(
			databasesource.IDEQ(dbSrc.ID),
			databasesource.ProjectIDEQ(projectID),
		).
		Exec(ctx)

	// Deleting the parent Source row cascades in PostgreSQL to documents, chunks, and jobs
	_, err = s.client.Source.Delete().
		Where(
			source.IDEQ(dbSrc.SourceID),
			source.ProjectIDEQ(projectID),
		).
		Exec(ctx)
	return err
}

// TestStoredConnection tests the connection of an existing database source.
func (s *DatabaseSourceService) TestStoredConnection(ctx context.Context, projectID, sourceID uuid.UUID) (*connector.ConnectionTestResult, error) {
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	decryptedURL, err := crypto.Decrypt(dbSrc.EncryptedConnectionURL, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting connection credentials: %w", err)
	}

	return s.TestConnection(ctx, dbSrc.DatabaseType, string(decryptedURL))
}

// GetMetadata introspects and returns catalog metadata for a stored database source.
func (s *DatabaseSourceService) GetMetadata(ctx context.Context, projectID, sourceID uuid.UUID) (*connector.DatabaseMetadata, error) {
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		Only(ctx)
	if err != nil {
		return nil, err
	}

	decryptedURL, err := crypto.Decrypt(dbSrc.EncryptedConnectionURL, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting connection credentials: %w", err)
	}

	conn, err := s.registry.Get(connector.DatabaseType(dbSrc.DatabaseType))
	if err != nil {
		return nil, err
	}

	cfg, err := conn.ParseURL(string(decryptedURL))
	if err != nil {
		return nil, conn.SanitizeError(err)
	}

	// SSRF validation before metadata introspection
	if cfg.Type != connector.TypeSQLite {
		if err := connector.ValidateNetworkTarget(cfg.Host, cfg.Port, s.allowPrivateIPs); err != nil {
			return nil, err
		}
	}

	return conn.DiscoverMetadata(ctx, cfg)
}

// TriggerSync enqueues an asynchronous database:sync background ingestion job.
func (s *DatabaseSourceService) TriggerSync(ctx context.Context, projectID, sourceID uuid.UUID) (uuid.UUID, error) {
	dbSrc, err := s.client.DatabaseSource.Query().
		Where(
			databasesource.ProjectIDEQ(projectID),
			databasesource.Or(
				databasesource.SourceIDEQ(sourceID),
				databasesource.IDEQ(sourceID),
			),
		).
		Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	// Create IngestionJob record
	jobID := uuid.New()
	if s.jobRepo != nil {
		job := &ent.IngestionJob{
			ID:              jobID,
			ProjectID:       projectID,
			SourceID:        dbSrc.SourceID,
			Status:          ingestionjob.StatusPending,
			ProgressPercent: 0,
		}
		if _, err := s.jobRepo.Create(ctx, job); err != nil {
			return uuid.Nil, fmt.Errorf("creating ingestion job: %w", err)
		}
	}

	// Update source status to syncing
	_ = s.client.Source.Update().
		Where(source.IDEQ(dbSrc.SourceID), source.ProjectIDEQ(projectID)).
		SetSyncStatus(source.SyncStatusSyncing).
		Exec(ctx)

	_ = s.client.DatabaseSource.UpdateOneID(dbSrc.ID).
		SetStatus("syncing").
		Exec(ctx)

	// Enqueue Asynq task
	if s.queueClient != nil {
		_, err := s.queueClient.EnqueueDatabaseSync(ctx, queue.DatabaseSyncPayload{
			JobID:     jobID,
			ProjectID: projectID,
			SourceID:  dbSrc.SourceID,
			Mode:      "schema",
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("enqueuing database sync task: %w", err)
		}
	}

	return jobID, nil
}

func (s *DatabaseSourceService) toResponse(src *ent.Source, dbSrc *ent.DatabaseSource) *DatabaseSourceResponse {
	name := ""
	if src != nil {
		name = src.Name
	}
	cfg := dbSrc.Configuration
	if cfg == nil {
		cfg = make(map[string]any)
	}

	return &DatabaseSourceResponse{
		ID:            dbSrc.ID,
		SourceID:      dbSrc.SourceID,
		ProjectID:     dbSrc.ProjectID,
		Name:          name,
		DatabaseType:  dbSrc.DatabaseType,
		Host:          dbSrc.Host,
		Port:          dbSrc.Port,
		DatabaseName:  dbSrc.DatabaseName,
		Username:      dbSrc.Username,
		Configuration: cfg,
		Status:        dbSrc.Status,
		LastError:     dbSrc.LastError,
		LastSyncedAt:  dbSrc.LastSyncedAt,
		CreatedAt:     dbSrc.CreatedAt,
		UpdatedAt:     dbSrc.UpdatedAt,
	}
}
