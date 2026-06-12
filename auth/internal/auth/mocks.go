package auth

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockUsuarioRepository struct {
	mock.Mock
}

func (m *MockUsuarioRepository) Create(ctx context.Context, usuario *Usuario) (*Usuario, error) {
	args := m.Called(ctx, usuario)
	var r0 *Usuario
	if rf, ok := args.Get(0).(func(context.Context, *Usuario) *Usuario); ok {
		r0 = rf(ctx, usuario)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*Usuario)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *Usuario) error); ok {
		r1 = rf(ctx, usuario)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByID(ctx context.Context, id uuid.UUID) (*Usuario, error) {
	args := m.Called(ctx, id)
	var r0 *Usuario
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) *Usuario); ok {
		r0 = rf(ctx, id)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*Usuario)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByEmail(ctx context.Context, email string) (*Usuario, error) {
	args := m.Called(ctx, email)
	var r0 *Usuario
	if rf, ok := args.Get(0).(func(context.Context, string) *Usuario); ok {
		r0 = rf(ctx, email)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*Usuario)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, email)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByProviderID(ctx context.Context, provider AuthProvider, providerID string) (*Usuario, error) {
	args := m.Called(ctx, provider, providerID)
	var r0 *Usuario
	if rf, ok := args.Get(0).(func(context.Context, AuthProvider, string) *Usuario); ok {
		r0 = rf(ctx, provider, providerID)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*Usuario)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, AuthProvider, string) error); ok {
		r1 = rf(ctx, provider, providerID)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status UserStatus) error {
	args := m.Called(ctx, id, status)
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID, UserStatus) error); ok {
		return rf(ctx, id, status)
	}
	return args.Error(0)
}

type MockRefreshTokenRepository struct {
	mock.Mock
}

func (m *MockRefreshTokenRepository) Create(ctx context.Context, token *RefreshToken) (*RefreshToken, error) {
	args := m.Called(ctx, token)
	var r0 *RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, *RefreshToken) *RefreshToken); ok {
		r0 = rf(ctx, token)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*RefreshToken)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *RefreshToken) error); ok {
		r1 = rf(ctx, token)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockRefreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	args := m.Called(ctx, tokenHash)
	var r0 *RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, string) *RefreshToken); ok {
		r0 = rf(ctx, tokenHash)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*RefreshToken)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, tokenHash)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockRefreshTokenRepository) FindActiveByFamilyID(ctx context.Context, familyID uuid.UUID) (*RefreshToken, error) {
	args := m.Called(ctx, familyID)
	var r0 *RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) *RefreshToken); ok {
		r0 = rf(ctx, familyID)
	} else if args.Get(0) != nil {
		r0 = args.Get(0).(*RefreshToken)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, familyID)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockRefreshTokenRepository) RevokeByID(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		return rf(ctx, id)
	}
	return args.Error(0)
}

func (m *MockRefreshTokenRepository) RevokeAllByFamilyID(ctx context.Context, familyID uuid.UUID) error {
	args := m.Called(ctx, familyID)
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		return rf(ctx, familyID)
	}
	return args.Error(0)
}

func (m *MockRefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		return rf(ctx, userID)
	}
	return args.Error(0)
}

func (m *MockRefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	var r0 int64
	if rf, ok := args.Get(0).(func(context.Context) int64); ok {
		r0 = rf(ctx)
	} else {
		r0 = args.Get(0).(int64)
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}
