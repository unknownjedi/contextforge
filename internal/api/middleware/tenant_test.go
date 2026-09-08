package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/your-org/contextforge/internal/api/middleware"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/ent"
)

type mockProjectRepo struct {
	projects map[uuid.UUID]*ent.Project
}

func (m *mockProjectRepo) Create(ctx context.Context, p *ent.Project) (*ent.Project, error) {
	return nil, nil
}

func (m *mockProjectRepo) GetByID(ctx context.Context, id, ownerUserID uuid.UUID) (*ent.Project, error) {
	p, ok := m.projects[id]
	if !ok || p.OwnerUserID != ownerUserID {
		return nil, errors.New("not found")
	}
	return p, nil
}

func (m *mockProjectRepo) ListByOwner(ctx context.Context, ownerUserID uuid.UUID, page, pageSize int) ([]*ent.Project, int, error) {
	return nil, 0, nil
}

func (m *mockProjectRepo) Update(ctx context.Context, p *ent.Project, ownerUserID uuid.UUID) (*ent.Project, error) {
	return nil, nil
}

func (m *mockProjectRepo) Delete(ctx context.Context, id, ownerUserID uuid.UUID) error {
	return nil
}

func setupTenantRouter(repo *mockProjectRepo, simulateUserID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Inject simulated auth if provided
	r.Use(func(c *gin.Context) {
		if simulateUserID != uuid.Nil {
			c.Set(auth.ContextKeyUserID, simulateUserID)
		}
		c.Next()
	})

	r.GET("/projects/:id/data", middleware.RequireProjectAccess(repo), func(c *gin.Context) {
		p, err := middleware.GetProject(c)
		if err != nil {
			c.String(http.StatusInternalServerError, "err")
			return
		}
		c.JSON(http.StatusOK, gin.H{"name": p.Name})
	})

	return r
}

func TestRequireProjectAccess(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()

	projectA := &ent.Project{
		ID:          uuid.New(),
		Name:        "Project Alpha",
		OwnerUserID: userA,
	}

	repo := &mockProjectRepo{
		projects: map[uuid.UUID]*ent.Project{
			projectA.ID: projectA,
		},
	}

	t.Run("User A accessing owned Project A (Success)", func(t *testing.T) {
		r := setupTenantRouter(repo, userA)
		req := httptest.NewRequest(http.MethodGet, "/projects/"+projectA.ID.String()+"/data", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Project Alpha")
	})

	t.Run("User B accessing Project A (Cross-tenant Isolation 404)", func(t *testing.T) {
		r := setupTenantRouter(repo, userB)
		req := httptest.NewRequest(http.MethodGet, "/projects/"+projectA.ID.String()+"/data", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "https://contextforge.dev/errors/not-found")
	})

	t.Run("Invalid Project UUID returns 400", func(t *testing.T) {
		r := setupTenantRouter(repo, userA)
		req := httptest.NewRequest(http.MethodGet, "/projects/invalid-uuid/data", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Unauthenticated user returns 401", func(t *testing.T) {
		r := setupTenantRouter(repo, uuid.Nil) // No user
		req := httptest.NewRequest(http.MethodGet, "/projects/"+projectA.ID.String()+"/data", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
