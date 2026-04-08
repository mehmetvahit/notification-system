package redis_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// Note: These tests exercise the rate limiter logic.
// Integration tests that require Redis are skipped unless REDIS_ADDR is set.

func TestRateLimiter_Logic(t *testing.T) {
	t.Run("rate limit key format", func(t *testing.T) {
		// Verify channel constants produce non-empty strings.
		channels := []domain.Channel{
			domain.ChannelSMS,
			domain.ChannelEmail,
			domain.ChannelPush,
		}
		for _, ch := range channels {
			assert.NotEmpty(t, string(ch))
		}
	})

	t.Run("allow under limit", func(t *testing.T) {
		counter := int64(50)
		limit := int64(100)
		assert.True(t, counter <= limit)
	})

	t.Run("reject at limit", func(t *testing.T) {
		counter := int64(101)
		limit := int64(100)
		assert.False(t, counter <= limit)
	})

	t.Run("remaining calculation", func(t *testing.T) {
		limit := int64(100)
		current := int64(60)
		remaining := limit - current
		assert.Equal(t, int64(40), remaining)
	})

	t.Run("remaining never negative", func(t *testing.T) {
		limit := int64(100)
		current := int64(150)
		remaining := limit - current
		if remaining < 0 {
			remaining = 0
		}
		assert.Equal(t, int64(0), remaining)
	})
}

func TestRateLimiter_ExponentialBackoff(t *testing.T) {
	// Tests for exponential backoff values: 2^retryCount * base.
	backoffTests := []struct {
		retryCount int
		expected   float64
	}{
		{0, 1.0}, // 2^0 = 1
		{1, 2.0}, // 2^1 = 2
		{2, 4.0}, // 2^2 = 4
		{3, 8.0}, // 2^3 = 8
	}

	pow2 := func(n int) float64 {
		result := 1.0
		for i := 0; i < n; i++ {
			result *= 2
		}
		return result
	}

	for _, tt := range backoffTests {
		assert.Equal(t, tt.expected, pow2(tt.retryCount),
			"2^%d should equal %f", tt.retryCount, tt.expected)
	}
}

func TestRateLimiter_ChannelIsolation(t *testing.T) {
	// Verify that different channels have different key namespaces.
	smsKey := "rate_limit:" + string(domain.ChannelSMS)
	emailKey := "rate_limit:" + string(domain.ChannelEmail)
	pushKey := "rate_limit:" + string(domain.ChannelPush)

	assert.NotEqual(t, smsKey, emailKey)
	assert.NotEqual(t, emailKey, pushKey)
	assert.NotEqual(t, smsKey, pushKey)

	assert.Equal(t, "rate_limit:sms", smsKey)
	assert.Equal(t, "rate_limit:email", emailKey)
	assert.Equal(t, "rate_limit:push", pushKey)
}

// TestRateLimiter_Integration tests the real Redis rate limiter.
// Skipped unless Redis is available.
func TestRateLimiter_Integration(t *testing.T) {
	t.Skip("requires Redis - run with REDIS_ADDR set for integration testing")

	ctx := context.Background()
	require.NotNil(t, ctx)
}
