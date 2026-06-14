package cache

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements Cache interface using Redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(config *CacheConfig) (*RedisCache, error) {
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 6379
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 1 * time.Hour
	}
	if config.PoolSize == 0 {
		config.PoolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MaxRetries:   3,
		MinIdleConns: 2,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client: client,
	}, nil
}

// Get retrieves a value from cache
func (r *RedisCache) Get(ctx context.Context, key string) (interface{}, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrKeyNotFound
		}
		logging.Z().Info(fmt.Sprintf("Error getting key %s: %v", key, err))
		return nil, fmt.Errorf("failed to get key: %w", err)
	}
	return val, nil
}

// Set stores a value in cache with TTL
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidDuration
	}
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error setting key %s: %v", key, err))
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

// Delete removes a key from cache
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error deleting key %s: %v", key, err))
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

// Clear removes all keys from cache (use with caution)
func (r *RedisCache) Clear(ctx context.Context) error {
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error clearing cache: %v", err))
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	return nil
}

// Exists checks if a key exists in cache
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Error checking existence of key %s: %v", key, err))
		return false, fmt.Errorf("failed to check key existence: %w", err)
	}
	return exists > 0, nil
}

// GetString retrieves a string value from cache
func (r *RedisCache) GetString(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrKeyNotFound
		}
		logging.Z().Info(fmt.Sprintf("Error getting string key %s: %v", key, err))
		return "", fmt.Errorf("failed to get string: %w", err)
	}
	return val, nil
}

// SetString stores a string value in cache
func (r *RedisCache) SetString(ctx context.Context, key string, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidDuration
	}
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error setting string key %s: %v", key, err))
		return fmt.Errorf("failed to set string: %w", err)
	}
	return nil
}

// GetJSON retrieves and unmarshals a JSON value from cache
func (r *RedisCache) GetJSON(ctx context.Context, key string, target interface{}) error {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrKeyNotFound
		}
		logging.Z().Info(fmt.Sprintf("Error getting JSON key %s: %v", key, err))
		return fmt.Errorf("failed to get JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(val), target); err != nil {
		logging.Z().Info(fmt.Sprintf("Error unmarshaling JSON for key %s: %v", key, err))
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// SetJSON marshals and stores a JSON value in cache
func (r *RedisCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidDuration
	}
	data, err := json.Marshal(value)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Error marshaling JSON for key %s: %v", key, err))
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error setting JSON key %s: %v", key, err))
		return fmt.Errorf("failed to set JSON: %w", err)
	}
	return nil
}

// IncrementCounter increments a counter by given amount
func (r *RedisCache) IncrementCounter(ctx context.Context, key string, amount int64) error {
	if err := r.client.IncrBy(ctx, key, amount).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error incrementing counter %s: %v", key, err))
		return fmt.Errorf("failed to increment counter: %w", err)
	}
	return nil
}

// DecrementCounter decrements a counter by given amount
func (r *RedisCache) DecrementCounter(ctx context.Context, key string, amount int64) error {
	if err := r.client.DecrBy(ctx, key, amount).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error decrementing counter %s: %v", key, err))
		return fmt.Errorf("failed to decrement counter: %w", err)
	}
	return nil
}

// GetCounter gets a counter value
func (r *RedisCache) GetCounter(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrKeyNotFound
		}
		logging.Z().Info(fmt.Sprintf("Error getting counter %s: %v", key, err))
		return 0, fmt.Errorf("failed to get counter: %w", err)
	}
	return val, nil
}

// SetCounter sets a counter value
func (r *RedisCache) SetCounter(ctx context.Context, key string, value int64, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidDuration
	}
	if err := r.client.Set(ctx, key, strconv.FormatInt(value, 10), ttl).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error setting counter %s: %v", key, err))
		return fmt.Errorf("failed to set counter: %w", err)
	}
	return nil
}

// Health checks if Redis is healthy
func (r *RedisCache) Health(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		logging.Z().Info(fmt.Sprintf("Health check failed: %v", err))
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	if err := r.client.Close(); err != nil {
		logging.Z().Info(fmt.Sprintf("Error closing Redis connection: %v", err))
		return fmt.Errorf("failed to close Redis connection: %w", err)
	}
	return nil
}
