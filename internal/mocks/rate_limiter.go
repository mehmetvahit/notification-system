package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// MockRateLimiter is a testify mock implementation of domain.RateLimiter.
type MockRateLimiter struct {
	mock.Mock
}

func (m *MockRateLimiter) Allow(ctx context.Context, channel domain.Channel) (bool, error) {
	args := m.Called(ctx, channel)
	return args.Bool(0), args.Error(1)
}

func (m *MockRateLimiter) Remaining(ctx context.Context, channel domain.Channel) (int64, error) {
	args := m.Called(ctx, channel)
	return args.Get(0).(int64), args.Error(1)
}
