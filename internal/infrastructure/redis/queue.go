package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const investigationJobQueue = "opspilot:investigation:jobs"

// JobQueue implements Redis-backed job notification.
type JobQueue struct {
	client *Client
}

// NewJobQueue creates a new job notification queue.
func NewJobQueue(client *Client) *JobQueue {
	return &JobQueue{client: client}
}

// Notify pushes a job ID onto the notification queue.
func (q *JobQueue) Notify(ctx context.Context, jobID uuid.UUID) error {
	if err := q.client.LPush(ctx, investigationJobQueue, jobID.String()).Err(); err != nil {
		return fmt.Errorf("notify job: %w", err)
	}
	return nil
}

// WaitForJob blocks until a job notification arrives or context is cancelled.
func (q *JobQueue) WaitForJob(ctx context.Context) error {
	_, err := q.client.BRPop(ctx, 5*time.Second, investigationJobQueue).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil // timeout, caller may poll DB
		}
		return fmt.Errorf("wait for job: %w", err)
	}
	return nil
}
