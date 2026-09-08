package queue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// Client wraps Asynq client for task enqueueing.
type Client struct {
	asynqClient *asynq.Client
}

// NewClient initializes a new Asynq queue client connecting to Redis.
func NewClient(redisURL string) (*Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL for asynq: %w", err)
	}

	client := asynq.NewClient(opt)
	return &Client{asynqClient: client}, nil
}

// EnqueueRepoSync schedules a repository synchronization task.
func (c *Client) EnqueueRepoSync(ctx context.Context, payload RepoSyncPayload) (*asynq.TaskInfo, error) {
	task, err := NewRepoSyncTask(payload)
	if err != nil {
		return nil, err
	}

	info, err := c.asynqClient.EnqueueContext(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("enqueuing repo sync task: %w", err)
	}
	return info, nil
}

// EnqueueDocEmbed schedules a document embedding task.
func (c *Client) EnqueueDocEmbed(ctx context.Context, payload DocEmbedPayload) (*asynq.TaskInfo, error) {
	task, err := NewDocEmbedTask(payload)
	if err != nil {
		return nil, err
	}

	info, err := c.asynqClient.EnqueueContext(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("enqueuing doc embed task: %w", err)
	}
	return info, nil
}

// EnqueueDatabaseSync schedules a database synchronization task.
func (c *Client) EnqueueDatabaseSync(ctx context.Context, payload DatabaseSyncPayload) (*asynq.TaskInfo, error) {
	task, err := NewDatabaseSyncTask(payload)
	if err != nil {
		return nil, err
	}

	info, err := c.asynqClient.EnqueueContext(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("enqueuing database sync task: %w", err)
	}
	return info, nil
}

// Close gracefully terminates the Asynq client.
func (c *Client) Close() error {
	if c.asynqClient != nil {
		return c.asynqClient.Close()
	}
	return nil
}
