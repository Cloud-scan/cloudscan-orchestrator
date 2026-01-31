package interfaces

import (
	"context"
	"time"
)

// CacheService defines the interface for caching operations using Redis
type CacheService interface {
	// Set stores a value in cache with expiration
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error

	// Get retrieves a value from cache
	Get(ctx context.Context, key string) (string, error)

	// Delete removes a value from cache
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists in cache
	Exists(ctx context.Context, key string) (bool, error)

	// SetNX sets a value only if the key doesn't exist (for distributed locking)
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)

	// Increment increments a counter
	Increment(ctx context.Context, key string) (int64, error)

	// Decrement decrements a counter
	Decrement(ctx context.Context, key string) (int64, error)

	// GetTTL gets the remaining time to live of a key
	GetTTL(ctx context.Context, key string) (time.Duration, error)

	// Ping checks if Redis is reachable
	Ping(ctx context.Context) error
}

// QueueService defines the interface for job queue operations using Redis
type QueueService interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, queueName string, jobData interface{}) error

	// Dequeue removes and returns a job from the queue (blocking)
	Dequeue(ctx context.Context, queueName string, timeout time.Duration) (string, error)

	// GetQueueDepth returns the number of items in the queue
	GetQueueDepth(ctx context.Context, queueName string) (int64, error)

	// ClearQueue removes all items from the queue
	ClearQueue(ctx context.Context, queueName string) error
}

// Common queue names
const (
	ScanQueue = "scans:queue"       // Queue for pending scans
	JobQueue  = "jobs:queue"        // Queue for Kubernetes jobs
)

// Common cache key prefixes
const (
	CachePrefixScan     = "scan:"
	CachePrefixProject  = "project:"
	CachePrefixOrg      = "org:"
	CachePrefixJobLock  = "lock:job:"
	CachePrefixRateLimit = "ratelimit:"
)