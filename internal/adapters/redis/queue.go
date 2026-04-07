package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

const queueKeyPrefix = "notifications:queue"

// Queue implements domain.Queue using Redis sorted sets for priority-ordered delivery.
// Key pattern: notifications:queue:{channel}:{priority}
// Score is Unix nanoseconds to preserve FIFO ordering within same priority.
type Queue struct {
	client *redis.Client
}

// NewQueue creates a new Redis-backed priority queue.
func NewQueue(client *redis.Client) *Queue {
	return &Queue{client: client}
}

// Enqueue serialises the notification and pushes it into the appropriate sorted set.
func (q *Queue) Enqueue(ctx context.Context, n *domain.Notification) error {
	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	key := queueKey(n.Channel, n.Priority)
	score := float64(time.Now().UnixNano())

	return q.client.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: string(data),
	}).Err()
}

// Dequeue pops the highest-priority, oldest notification across all channels.
// Priority order: high -> normal -> low.
func (q *Queue) Dequeue(ctx context.Context) (*domain.Notification, error) {
	channels := []domain.Channel{domain.ChannelSMS, domain.ChannelEmail, domain.ChannelPush}
	priorities := []domain.Priority{domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow}

	for _, priority := range priorities {
		for _, channel := range channels {
			key := queueKey(channel, priority)

			// Atomically pop the member with lowest score (oldest).
			results, err := q.client.ZPopMin(ctx, key, 1).Result()
			if err != nil {
				if err == redis.Nil {
					continue
				}
				return nil, fmt.Errorf("zpopmin %s: %w", key, err)
			}
			if len(results) == 0 {
				continue
			}

			var n domain.Notification
			if err := json.Unmarshal([]byte(results[0].Member.(string)), &n); err != nil {
				return nil, fmt.Errorf("unmarshal notification: %w", err)
			}
			return &n, nil
		}
	}

	return nil, nil // queue empty
}

// Len returns the number of items in a specific channel+priority queue.
func (q *Queue) Len(ctx context.Context, channel domain.Channel, priority domain.Priority) (int64, error) {
	key := queueKey(channel, priority)
	return q.client.ZCard(ctx, key).Result()
}

func queueKey(channel domain.Channel, priority domain.Priority) string {
	return fmt.Sprintf("%s:%s:%s", queueKeyPrefix, channel, priority)
}
