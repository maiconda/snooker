package storage

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockStorage é uma implementação mockada da interface Storage para testes.
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) EnsureBucket(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) GeneratePresignedUploadURL(ctx context.Context) (string, string, error) {
	args := m.Called(ctx)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockStorage) ConsolidateAvatar(ctx context.Context, tempKey string, userID uuid.UUID) (string, error) {
	args := m.Called(ctx, tempKey, userID)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) GetPublicURL(objectKey string) string {
	args := m.Called(objectKey)
	return args.String(0)
}
