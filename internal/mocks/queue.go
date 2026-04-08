package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// MockQueue is a testify mock implementation of domain.Queue.
type MockQueue struct {
	mock.Mock
}

func (m *MockQueue) Enqueue(ctx context.Context, n *domain.Notification) error {
	args := m.Called(ctx, n)
	return args.Error(0)
}

func (m *MockQueue) Dequeue(ctx context.Context) (*domain.Notification, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Notification), args.Error(1)
}

func (m *MockQueue) Len(ctx context.Context, channel domain.Channel, priority domain.Priority) (int64, error) {
	args := m.Called(ctx, channel, priority)
	return args.Get(0).(int64), args.Error(1)
}
