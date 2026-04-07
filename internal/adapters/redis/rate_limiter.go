package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

const (
	rateLimitKeyPrefix   = "rate_limit"
	rateLimitWindowSecs  = 1           // 1-second sliding window
	rateLimitMaxRequests = int64(100)  // 100 requests per second per channel
)

// RateLimiter implements domain.RateLimiter using a Redis sliding-window counter.
// It uses an atomic Lua script to increment a per-channel counter and set TTL atomically.
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter creates a new Redis-backed sliding window rate limiter.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// luaScript atomically increments the key and sets its expiry on first access.
var luaScript = redis.NewScript(`
local key = KEYS[1]
local window = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])

local current = redis.call("INCR", key)
if current == 1 then
    redis.call("EXPIRE", key, window)
end
return current
`)

// Allow returns true if the channel is under the rate limit for the current window.
func (r *RateLimiter) Allow(ctx context.Context, channel domain.Channel) (bool, error) {
	key := rateLimitKey(channel)
	result, err := luaScript.Run(ctx, r.client,
		[]string{key},
		rateLimitWindowSecs,
		rateLimitMaxRequests,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("rate limiter lua script: %w", err)
	}
	return result <= rateLimitMaxRequests, nil
}

// Remaining returns how many requests are still allowed in the current window.
func (r *RateLimiter) Remaining(ctx context.Context, channel domain.Channel) (int64, error) {
	key := rateLimitKey(channel)
	current, err := r.client.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return rateLimitMaxRequests, nil
		}
		return 0, fmt.Errorf("get rate limit counter: %w", err)
	}
	remaining := rateLimitMaxRequests - current
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// resetKey is used in tests to clean up the rate limit key.
func (r *RateLimiter) resetKey(ctx context.Context, channel domain.Channel) error {
	return r.client.Del(ctx, rateLimitKey(channel)).Err()
}

// setCounter sets the rate limit counter to a specific value (used in tests).
func (r *RateLimiter) setCounter(ctx context.Context, channel domain.Channel, count int64) error {
	key := rateLimitKey(channel)
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, count, time.Duration(rateLimitWindowSecs)*time.Second)
	_, err := pipe.Exec(ctx)
	return err
}

func rateLimitKey(channel domain.Channel) string {
	return fmt.Sprintf("%s:%s", rateLimitKeyPrefix, channel)
}
