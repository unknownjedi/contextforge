package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/ingestionjob"
)

type mockJobRepo struct {
	jobs map[uuid.UUID]*ent.IngestionJob
}

func newMockJobRepo() *mockJobRepo {
	return &mockJobRepo{jobs: make(map[uuid.UUID]*ent.IngestionJob)}
}

func (m *mockJobRepo) Create(ctx context.Context, j *ent.IngestionJob) (*ent.IngestionJob, error) {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	m.jobs[j.ID] = j
	return j, nil
}

func (m *mockJobRepo) GetByID(ctx context.Context, id, projectID uuid.UUID) (*ent.IngestionJob, error) {
	j, ok := m.jobs[id]
	if !ok || j.ProjectID != projectID {
		return nil, fmt.Errorf("job not found")
	}
	return j, nil
}

func (m *mockJobRepo) GetByIDOnly(ctx context.Context, id uuid.UUID) (*ent.IngestionJob, error) {
	j, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job not found")
	}
	return j, nil
}

func (m *mockJobRepo) ListByProjectID(ctx context.Context, projectID uuid.UUID, limit int) ([]*ent.IngestionJob, error) {
	var results []*ent.IngestionJob
	for _, j := range m.jobs {
		if j.ProjectID == projectID {
			results = append(results, j)
		}
	}
	return results, nil
}

func (m *mockJobRepo) UpdateProgress(ctx context.Context, id, projectID uuid.UUID, percent, processed, total int) error {
	j, ok := m.jobs[id]
	if !ok || j.ProjectID != projectID {
		return fmt.Errorf("job not found")
	}
	j.ProgressPercent = percent
	j.ProcessedFiles = processed
	j.TotalFiles = total
	return nil
}

func (m *mockJobRepo) Complete(ctx context.Context, id, projectID uuid.UUID) error {
	j, ok := m.jobs[id]
	if !ok || j.ProjectID != projectID {
		return fmt.Errorf("job not found")
	}
	j.Status = ingestionjob.StatusCompleted
	return nil
}

func (m *mockJobRepo) Fail(ctx context.Context, id, projectID uuid.UUID, errorMsg string) error {
	j, ok := m.jobs[id]
	if !ok || j.ProjectID != projectID {
		return fmt.Errorf("job not found")
	}
	j.Status = ingestionjob.StatusFailed
	j.ErrorMessage = errorMsg
	return nil
}

func TestJobHandler_GetJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	projectID := uuid.New()
	jobID := uuid.New()

	jobRepo := newMockJobRepo()
	job := &ent.IngestionJob{
		ID:              jobID,
		ProjectID:       projectID,
		Status:          ingestionjob.StatusRunning,
		ProgressPercent: 50,
		ProcessedFiles:  5,
		TotalFiles:      10,
	}
	_, _ = jobRepo.Create(context.Background(), job)

	jobHandler := handler.NewJobHandler(jobRepo, zap.NewNop())

	router := gin.New()
	router.GET("/projects/:id/jobs/:job_id", jobHandler.GetJob)
	router.GET("/jobs/:job_id", jobHandler.GetJobByID)

	t.Run("GetJob with project scope returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%s/jobs/%s", projectID, jobID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp ent.IngestionJob
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, jobID, resp.ID)
		assert.Equal(t, 50, resp.ProgressPercent)
	})

	t.Run("GetJobByID direct lookup without project ID returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/jobs/%s", jobID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp ent.IngestionJob
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, jobID, resp.ID)
	})

	t.Run("GetJobByID with non-existent job returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/jobs/%s", uuid.New()), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
