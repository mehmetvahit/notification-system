package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"

	domain "github.com/insider/notification-system/internal/domain/notification"
)

// MockProvider is a testify mock implementation of domain.Provider.
type MockProvider struct {
	mock.Mock
}

func (m *MockProvider) Send(ctx context.Context, n *domain.Notification) (string, error) {
	args := m.Called(ctx, n)
	return args.String(0), args.Error(1)
}
