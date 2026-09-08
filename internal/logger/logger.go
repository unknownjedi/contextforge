package logger

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const (
	CorrelationIDKey contextKey = "correlation_id"
	TraceIDKey       contextKey = "trace_id"
	ProjectIDKey     contextKey = "project_id"
	UserIDKey        contextKey = "user_id"
	LoggerKey        contextKey = "zap_logger"
)

const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderTraceID       = "X-Trace-ID"
	HeaderProjectID     = "X-Project-ID"
	HeaderUserID        = "X-User-ID"
)

// New creates a new zap.Logger configured for production or development.
// If env == "production", a JSON encoder is used. Otherwise, a console encoder is used.
func New(env string, levelStr string) (*zap.Logger, error) {
	return NewWithWriter(env, levelStr, zapcore.AddSync(os.Stdout))
}

// NewWithWriter allows specifying a custom WriteSyncer (useful for testing or custom destinations).
func NewWithWriter(env string, levelStr string, ws zapcore.WriteSyncer) (*zap.Logger, error) {
	level := ParseLevel(levelStr)

	var encoder zapcore.Encoder
	if strings.ToLower(env) == "production" {
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "timestamp"
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
		encCfg.EncodeDuration = zapcore.MillisDurationEncoder
		encCfg.CallerKey = "caller"
		encCfg.EncodeCaller = zapcore.ShortCallerEncoder
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encCfg := zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encCfg.EncodeDuration = zapcore.StringDurationEncoder
		encCfg.CallerKey = "caller"
		encCfg.EncodeCaller = zapcore.ShortCallerEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	core := zapcore.NewCore(encoder, ws, level)
	log := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return log, nil
}

// ParseLevel parses a string log level into zapcore.Level. Defaults to InfoLevel.
func ParseLevel(lvl string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(lvl)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Context injection helpers

// WithCorrelationID stores a correlation ID in context.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// GetCorrelationID extracts correlation ID from context.
func GetCorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return v
	}
	return ""
}

// WithTraceID stores a trace ID in context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID extracts trace ID from context.
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}

// WithProjectID stores a project ID in context.
func WithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, ProjectIDKey, projectID)
}

// GetProjectID extracts project ID from context.
func GetProjectID(ctx context.Context) string {
	if v, ok := ctx.Value(ProjectIDKey).(string); ok {
		return v
	}
	return ""
}

// WithUserID stores a user ID in context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID extracts user ID from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

// WithLogger stores a *zap.Logger in context.
func WithLogger(ctx context.Context, log *zap.Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, log)
}

// WithContextFields inspects context for correlation_id, trace_id, project_id, and user_id,
// and returns a logger enriched with any present fields.
func WithContextFields(ctx context.Context, log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.L()
	}

	var fields []zap.Field
	if cid := GetCorrelationID(ctx); cid != "" {
		fields = append(fields, zap.String("correlation_id", cid))
	}
	if tid := GetTraceID(ctx); tid != "" {
		fields = append(fields, zap.String("trace_id", tid))
	}
	if pid := GetProjectID(ctx); pid != "" {
		fields = append(fields, zap.String("project_id", pid))
	}
	if uid := GetUserID(ctx); uid != "" {
		fields = append(fields, zap.String("user_id", uid))
	}

	if len(fields) > 0 {
		return log.With(fields...)
	}
	return log
}

// FromContext returns the logger stored in context, or the global logger,
// automatically enriched with contextual fields (correlation_id, trace_id, project_id, user_id).
func FromContext(ctx context.Context) *zap.Logger {
	var base *zap.Logger
	if log, ok := ctx.Value(LoggerKey).(*zap.Logger); ok && log != nil {
		base = log
	} else {
		base = zap.L()
	}
	return WithContextFields(ctx, base)
}

// GetGinLogger retrieves the contextual logger from gin.Context.
func GetGinLogger(c *gin.Context) *zap.Logger {
	if val, exists := c.Get(string(LoggerKey)); exists {
		if log, ok := val.(*zap.Logger); ok {
			return log
		}
	}
	return FromContext(c.Request.Context())
}

// Middleware returns a Gin middleware that handles correlation IDs and logs HTTP requests.
func Middleware(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Resolve or generate correlation ID
		correlationID := c.GetHeader(HeaderCorrelationID)
		if correlationID == "" {
			correlationID = c.GetHeader(HeaderRequestID)
		}
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Ensure correlation ID is in response headers
		c.Writer.Header().Set(HeaderCorrelationID, correlationID)

		// 2. Resolve additional contextual identifiers from headers if available
		traceID := c.GetHeader(HeaderTraceID)
		projectID := c.GetHeader(HeaderProjectID)
		userID := c.GetHeader(HeaderUserID)

		// 3. Inject into request Context
		ctx := c.Request.Context()
		ctx = WithCorrelationID(ctx, correlationID)
		if traceID != "" {
			ctx = WithTraceID(ctx, traceID)
		}
		if projectID != "" {
			ctx = WithProjectID(ctx, projectID)
		}
		if userID != "" {
			ctx = WithUserID(ctx, userID)
		}

		// Create request-scoped logger enriched with contextual fields
		reqLogger := WithContextFields(ctx, log)
		ctx = WithLogger(ctx, reqLogger)
		c.Request = c.Request.WithContext(ctx)

		// Set in Gin context as well
		c.Set(string(LoggerKey), reqLogger)
		c.Set(string(CorrelationIDKey), correlationID)

		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int64("duration_ms", duration.Milliseconds()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		switch {
		case status >= http.StatusInternalServerError:
			reqLogger.Error("server error processing request", fields...)
		case status >= http.StatusBadRequest:
			reqLogger.Warn("client error processing request", fields...)
		default:
			reqLogger.Info("request completed", fields...)
		}
	}
}
