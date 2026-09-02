package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a Redis client.
type Client struct {
	*goredis.Client
}

// NewClient creates a new Redis client.
func NewClient(ctx context.Context, redisURL string) (*Client, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := goredis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{Client: client}, nil
}

// Close closes the Redis client.
func (c *Client) Close() error {
	return c.Client.Close()
}

// Ping checks Redis connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}
