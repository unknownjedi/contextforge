package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/contextforge/internal/api/handler"
)

type mockChecker struct {
	err error
}

func (m *mockChecker) Check(ctx context.Context) error {
	return m.err
}

func setupHealthRouter(h *handler.HealthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", h.Healthz)
	r.GET("/readyz", h.Readyz)
	return r
}

func TestHealthHandler_Healthz(t *testing.T) {
	h := handler.NewHealthHandler(nil, nil)
	r := setupHealthRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ok", resp["status"])
	assert.NotEmpty(t, resp["timestamp"])

	_, err = time.Parse(time.RFC3339, resp["timestamp"])
	assert.NoError(t, err, "timestamp should parse as RFC3339")
}

func TestHealthHandler_Readyz_AllHealthy(t *testing.T) {
	dbOk := &mockChecker{err: nil}
	redisOk := &mockChecker{err: nil}

	h := handler.NewHealthHandler(dbOk, redisOk)
	r := setupHealthRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "ready", resp["status"])
	assert.Equal(t, "connected", resp["database"])
	assert.Equal(t, "connected", resp["redis"])
}

func TestHealthHandler_Readyz_DatabaseFailure(t *testing.T) {
	dbFail := &mockChecker{err: errors.New("connection refused")}
	redisOk := &mockChecker{err: nil}

	h := handler.NewHealthHandler(dbFail, redisOk)
	r := setupHealthRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "https://contextforge.dev/errors/dependency-unready", resp["type"])
	assert.Equal(t, float64(503), resp["status"])
}
