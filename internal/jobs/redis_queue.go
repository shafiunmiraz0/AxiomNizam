package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue errors
var (
	ErrQueueEmpty = fmt.Errorf("queue is empty")
)

// RedisQueue implements Queue interface using Redis backend
type RedisQueue struct {
	client        *redis.Client
	queueKey      string
	dlqKey        string
	processingKey string
	mu            sync.RWMutex
}

// NewRedisQueue creates a new Redis-backed queue
func NewRedisQueue(redisAddr string) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisQueue{
		client:        client,
		queueKey:      "axiom:jobs:queue",
		dlqKey:        "axiom:jobs:dlq",
		processingKey: "axiom:jobs:processing",
	}, nil
}

// Submit adds a job to the queue
func (rq *RedisQueue) Submit(ctx context.Context, job *Job) error {
	if job == nil {
		return ErrInvalidJob
	}

	// Serialize job
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	// Add to queue using sorted set for priority handling
	score := float64(time.Now().UnixNano())/1e9 - float64(job.Priority)
	if err := rq.client.ZAdd(ctx, rq.queueKey, redis.Z{
		Score:  score,
		Member: job.ID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add to queue: %w", err)
	}

	// Store job data in hash
	if err := rq.client.HSet(ctx, fmt.Sprintf("job:%s", job.ID), "data", data).Err(); err != nil {
		return fmt.Errorf("failed to store job data: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("Job submitted: %s (priority: %d)", job.ID, job.Priority))
	return nil
}

// Get retrieves a job from the queue
func (rq *RedisQueue) Get(ctx context.Context, jobID string) (*Job, error) {
	// Get job data from hash
	result, err := rq.client.HGet(ctx, fmt.Sprintf("job:%s", jobID), "data").Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrJobNotFound
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job Job
	if err := json.Unmarshal([]byte(result), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// UpdateStatus updates job status
func (rq *RedisQueue) UpdateStatus(ctx context.Context, jobID string, status JobStatus) error {
	job, err := rq.Get(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = status

	// Update job data
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	if err := rq.client.HSet(ctx, fmt.Sprintf("job:%s", jobID), "data", data).Err(); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("Job status updated: %s -> %s", jobID, status))
	return nil
}

// Delete removes a job from queue and storage
func (rq *RedisQueue) Delete(ctx context.Context, jobID string) error {
	// Remove from queue
	if err := rq.client.ZRem(ctx, rq.queueKey, jobID).Err(); err != nil {
		return fmt.Errorf("failed to remove from queue: %w", err)
	}

	// Remove from processing
	rq.client.ZRem(ctx, rq.processingKey, jobID)

	// Delete job data
	if err := rq.client.Del(ctx, fmt.Sprintf("job:%s", jobID)).Err(); err != nil {
		return fmt.Errorf("failed to delete job data: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("Job deleted: %s", jobID))
	return nil
}

// GetSize returns the number of jobs in queue
func (rq *RedisQueue) GetSize(ctx context.Context) (int, error) {
	size, err := rq.client.ZCard(ctx, rq.queueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue size: %w", err)
	}

	return int(size), nil
}

// Peek returns the next job without removing it
func (rq *RedisQueue) Peek(ctx context.Context) (*Job, error) {
	// Get highest priority job (lowest score)
	results, err := rq.client.ZRange(ctx, rq.queueKey, 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to peek queue: %w", err)
	}

	if len(results) == 0 {
		return nil, ErrQueueEmpty
	}

	return rq.Get(ctx, results[0])
}

// Dequeue retrieves and removes the next job
func (rq *RedisQueue) Dequeue(ctx context.Context) (*Job, error) {
	// Use Lua script for atomic dequeue
	script := redis.NewScript(`
		local job_id = redis.call('zrange', KEYS[1], 0, 0)[1]
		if not job_id then
			return nil
		end
		redis.call('zrem', KEYS[1], job_id)
		redis.call('zadd', KEYS[2], ARGV[1], job_id)
		return job_id
	`)

	result, err := script.Run(ctx, rq.client, []string{rq.queueKey, rq.processingKey}, time.Now().UnixNano()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue: %w", err)
	}

	jobID, ok := result.(string)
	if !ok || jobID == "" {
		return nil, ErrQueueEmpty
	}

	job, err := rq.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	job.Status = JobStatusRunning
	job.StartedAt = time.Now()

	// Update job data
	data, _ := json.Marshal(job)
	rq.client.HSet(ctx, fmt.Sprintf("job:%s", jobID), "data", data)

	logging.Z().Info(fmt.Sprintf("Job dequeued: %s", jobID))
	return job, nil
}

// Requeue moves a job back to queue
func (rq *RedisQueue) Requeue(ctx context.Context, jobID string) error {
	// Remove from processing
	rq.client.ZRem(ctx, rq.processingKey, jobID)

	// Add back to queue
	score := float64(time.Now().UnixNano()) / 1e9
	if err := rq.client.ZAdd(ctx, rq.queueKey, redis.Z{
		Score:  score,
		Member: jobID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to requeue: %w", err)
	}

	job, _ := rq.Get(ctx, jobID)
	if job != nil {
		job.Status = JobStatusPending
		job.Retries++
		data, _ := json.Marshal(job)
		rq.client.HSet(ctx, fmt.Sprintf("job:%s", jobID), "data", data)
	}

	logging.Z().Info(fmt.Sprintf("Job requeued: %s", jobID))
	return nil
}

// GetByStatus returns jobs with specific status
func (rq *RedisQueue) GetByStatus(ctx context.Context, status JobStatus, limit int) ([]*Job, error) {
	// Get all jobs from queue (this is less efficient than in-memory)
	jobs := make([]*Job, 0, limit)

	// Scan through queue
	iter := rq.client.Scan(ctx, 0, "job:*", int64(limit*2)).Iterator()
	for iter.Next(ctx) && len(jobs) < limit {
		jobID := iter.Val()[4:] // Remove "job:" prefix

		job, err := rq.Get(ctx, jobID)
		if err == nil && job.Status == status {
			jobs = append(jobs, job)
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to get jobs by status: %w", err)
	}

	return jobs, nil
}

// GetByType returns jobs with specific type
func (rq *RedisQueue) GetByType(ctx context.Context, jobType JobType, limit int) ([]*Job, error) {
	jobs := make([]*Job, 0, limit)

	iter := rq.client.Scan(ctx, 0, "job:*", int64(limit*2)).Iterator()
	for iter.Next(ctx) && len(jobs) < limit {
		jobID := iter.Val()[4:] // Remove "job:" prefix

		job, err := rq.Get(ctx, jobID)
		if err == nil && job.Type == jobType {
			jobs = append(jobs, job)
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to get jobs by type: %w", err)
	}

	return jobs, nil
}

// Clear removes all jobs from queue
func (rq *RedisQueue) Clear(ctx context.Context) error {
	// Get all job IDs
	jobIDs, err := rq.client.ZRange(ctx, rq.queueKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}

	// Delete all job data
	for _, jobID := range jobIDs {
		rq.client.Del(ctx, fmt.Sprintf("job:%s", jobID))
	}

	// Clear queue and processing sets
	rq.client.Del(ctx, rq.queueKey)
	rq.client.Del(ctx, rq.processingKey)

	logging.Z().Info(fmt.Sprintf("Queue cleared (%d jobs)", len(jobIDs)))
	return nil
}

// GetProcessing returns jobs currently being processed
func (rq *RedisQueue) GetProcessing(ctx context.Context) ([]*Job, error) {
	jobIDs, err := rq.client.ZRange(ctx, rq.processingKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get processing jobs: %w", err)
	}

	jobs := make([]*Job, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		job, err := rq.Get(ctx, jobID)
		if err == nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// MoveToDeadLetter moves a job to the dead letter queue
func (rq *RedisQueue) MoveToDeadLetter(ctx context.Context, jobID string) error {
	job, err := rq.Get(ctx, jobID)
	if err != nil {
		return err
	}

	// Remove from processing
	rq.client.ZRem(ctx, rq.processingKey, jobID)

	// Add to DLQ
	score := float64(time.Now().UnixNano()) / 1e9
	if err := rq.client.ZAdd(ctx, rq.dlqKey, redis.Z{
		Score:  score,
		Member: jobID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to move to DLQ: %w", err)
	}

	job.Status = JobStatusFailed
	data, _ := json.Marshal(job)
	rq.client.HSet(ctx, fmt.Sprintf("job:%s", jobID), "data", data)

	logging.Z().Info(fmt.Sprintf("Job moved to DLQ: %s", jobID))
	return nil
}

// GetDeadLetterCount returns count of jobs in DLQ
func (rq *RedisQueue) GetDeadLetterCount(ctx context.Context) (int, error) {
	count, err := rq.client.ZCard(ctx, rq.dlqKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get DLQ count: %w", err)
	}

	return int(count), nil
}

// GetStats returns queue statistics
func (rq *RedisQueue) GetStats(ctx context.Context) map[string]interface{} {
	size, _ := rq.GetSize(ctx)
	processing, _ := rq.client.ZCard(ctx, rq.processingKey).Result()
	dlq, _ := rq.client.ZCard(ctx, rq.dlqKey).Result()

	info := rq.client.Info(ctx, "stats").Val()
	keys, _ := rq.client.DBSize(ctx).Result()

	return map[string]interface{}{
		"queued":     size,
		"processing": processing,
		"dlq":        dlq,
		"total_keys": keys,
		"redis_info": info,
	}
}

// Close closes the Redis connection
func (rq *RedisQueue) Close() error {
	return rq.client.Close()
}

// RedisQueueCluster extends RedisQueue for cluster support
type RedisQueueCluster struct {
	client   *redis.ClusterClient
	queueKey string
}

// NewRedisQueueCluster creates a Redis cluster-backed queue
func NewRedisQueueCluster(nodes []string) (*RedisQueueCluster, error) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: nodes,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis cluster: %w", err)
	}

	return &RedisQueueCluster{
		client:   client,
		queueKey: "axiom:jobs:queue",
	}, nil
}

// Submit adds a job to the cluster queue
func (rqc *RedisQueueCluster) Submit(ctx context.Context, job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	score := float64(time.Now().UnixNano())/1e9 - float64(job.Priority)
	if err := rqc.client.ZAdd(ctx, rqc.queueKey, redis.Z{
		Score:  score,
		Member: job.ID,
	}).Err(); err != nil {
		return fmt.Errorf("failed to add to cluster queue: %w", err)
	}

	if err := rqc.client.HSet(ctx, fmt.Sprintf("job:%s", job.ID), "data", data).Err(); err != nil {
		return fmt.Errorf("failed to store job in cluster: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("Job submitted to cluster: %s", job.ID))
	return nil
}

// Close closes the cluster connection
func (rqc *RedisQueueCluster) Close() error {
	return rqc.client.Close()
}
