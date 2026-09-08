package queue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// WorkerServer wraps Asynq server instance for processing background jobs.
type WorkerServer struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *zap.Logger
}

// WorkerConfig holds configuration for Asynq worker server.
type WorkerConfig struct {
	RedisURL    string
	Concurrency int
}

// NewWorkerServer initializes the Asynq worker server with priority queues.
func NewWorkerServer(cfg WorkerConfig, logger *zap.Logger) (*WorkerServer, error) {
	opt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL for worker: %w", err)
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	srv := asynq.NewServer(
		opt,
		asynq.Config{
			Concurrency: concurrency,
			Queues: map[string]int{
				QueueCritical: 6,
				QueueDefault:  3,
				QueueLow:      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				logger.Error("asynq task execution failed",
					zap.String("type", task.Type()),
					zap.Error(err),
				)
			}),
		},
	)

	return &WorkerServer{
		server: srv,
		mux:    asynq.NewServeMux(),
		logger: logger,
	}, nil
}

// RegisterHandler registers a task handler function for a specific task type.
func (w *WorkerServer) RegisterHandler(taskType string, handler asynq.HandlerFunc) {
	w.mux.HandleFunc(taskType, handler)
}

// Start runs the worker processing loop (blocking).
func (w *WorkerServer) Start() error {
	return w.server.Run(w.mux)
}

// Shutdown initiates graceful shutdown of the worker server.
func (w *WorkerServer) Shutdown() {
	w.server.Shutdown()
}
