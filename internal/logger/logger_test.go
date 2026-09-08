package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/your-org/contextforge/internal/logger"
)

type syncer struct {
	*bytes.Buffer
}

func (s syncer) Sync() error { return nil }

func TestNew_ProductionJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	ws := syncer{Buffer: buf}

	log, err := logger.NewWithWriter("production", "info", ws)
	require.NoError(t, err)
	require.NotNil(t, log)

	log.Info("test production message", zap.String("service", "contextforge"))

	var entry map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "info", entry["level"])
	assert.Equal(t, "test production message", entry["msg"])
	assert.Equal(t, "contextforge", entry["service"])
	assert.NotEmpty(t, entry["timestamp"])
}

func TestNew_DevelopmentConsole(t *testing.T) {
	buf := &bytes.Buffer{}
	ws := syncer{Buffer: buf}

	log, err := logger.NewWithWriter("development", "debug", ws)
	require.NoError(t, err)
	require.NotNil(t, log)

	log.Debug("debug message")
	assert.Contains(t, buf.String(), "debug message")
}

func TestParseLevel(t *testing.T) {
	assert.Equal(t, zapcore.DebugLevel, logger.ParseLevel("debug"))
	assert.Equal(t, zapcore.InfoLevel, logger.ParseLevel("info"))
	assert.Equal(t, zapcore.WarnLevel, logger.ParseLevel("warn"))
	assert.Equal(t, zapcore.WarnLevel, logger.ParseLevel("warning"))
	assert.Equal(t, zapcore.ErrorLevel, logger.ParseLevel("error"))
	assert.Equal(t, zapcore.InfoLevel, logger.ParseLevel("unknown"))
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	ctx = logger.WithCorrelationID(ctx, "corr-123")
	ctx = logger.WithTraceID(ctx, "trace-456")
	ctx = logger.WithProjectID(ctx, "proj-789")
	ctx = logger.WithUserID(ctx, "user-abc")

	assert.Equal(t, "corr-123", logger.GetCorrelationID(ctx))
	assert.Equal(t, "trace-456", logger.GetTraceID(ctx))
	assert.Equal(t, "proj-789", logger.GetProjectID(ctx))
	assert.Equal(t, "user-abc", logger.GetUserID(ctx))

	buf := &bytes.Buffer{}
	log, _ := logger.NewWithWriter("production", "info", syncer{buf})

	enriched := logger.WithContextFields(ctx, log)
	enriched.Info("context test")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)

	assert.Equal(t, "corr-123", entry["correlation_id"])
	assert.Equal(t, "trace-456", entry["trace_id"])
	assert.Equal(t, "proj-789", entry["project_id"])
	assert.Equal(t, "user-abc", entry["user_id"])
}

func TestMiddleware_CorrelationID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}
	log, _ := logger.NewWithWriter("production", "info", syncer{buf})

	r := gin.New()
	r.Use(logger.Middleware(log))
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 1. Without correlation header (middleware generates one)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	corrID := w.Header().Get(logger.HeaderCorrelationID)
	assert.NotEmpty(t, corrID)

	// 2. With client-supplied correlation header
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set(logger.HeaderCorrelationID, "client-corr-xyz")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	assert.Equal(t, "client-corr-xyz", w2.Header().Get(logger.HeaderCorrelationID))
}
