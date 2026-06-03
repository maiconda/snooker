package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"snooker/internal/models"
)

// MockUsuarioRepository é um mock para UsuarioRepository.
type MockUsuarioRepository struct {
	mock.Mock
}

func (m *MockUsuarioRepository) Create(ctx context.Context, usuario *models.Usuario) (*models.Usuario, error) {
	args := m.Called(ctx, usuario)
	var r0 *models.Usuario
	if rf, ok := args.Get(0).(func(context.Context, *models.Usuario) *models.Usuario); ok {
		r0 = rf(ctx, usuario)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Usuario)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *models.Usuario) error); ok {
		r1 = rf(ctx, usuario)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Usuario, error) {
	args := m.Called(ctx, id)
	var r0 *models.Usuario
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) *models.Usuario); ok {
		r0 = rf(ctx, id)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Usuario)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByEmail(ctx context.Context, email string) (*models.Usuario, error) {
	args := m.Called(ctx, email)
	var r0 *models.Usuario
	if rf, ok := args.Get(0).(func(context.Context, string) *models.Usuario); ok {
		r0 = rf(ctx, email)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Usuario)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, email)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) FindByProviderID(ctx context.Context, provider models.AuthProvider, providerID string) (*models.Usuario, error) {
	args := m.Called(ctx, provider, providerID)
	var r0 *models.Usuario
	if rf, ok := args.Get(0).(func(context.Context, models.AuthProvider, string) *models.Usuario); ok {
		r0 = rf(ctx, provider, providerID)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Usuario)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, models.AuthProvider, string) error); ok {
		r1 = rf(ctx, provider, providerID)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockUsuarioRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
	args := m.Called(ctx, id, status)
	var r0 error
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID, models.UserStatus) error); ok {
		r0 = rf(ctx, id, status)
	} else {
		r0 = args.Error(0)
	}
	return r0
}

// MockPerfilRepository é um mock para PerfilRepository.
type MockPerfilRepository struct {
	mock.Mock
}

func (m *MockPerfilRepository) Create(ctx context.Context, perfil *models.Perfil) (*models.Perfil, error) {
	args := m.Called(ctx, perfil)
	var r0 *models.Perfil
	if rf, ok := args.Get(0).(func(context.Context, *models.Perfil) *models.Perfil); ok {
		r0 = rf(ctx, perfil)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Perfil)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *models.Perfil) error); ok {
		r1 = rf(ctx, perfil)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockPerfilRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*models.Perfil, error) {
	args := m.Called(ctx, userID)
	var r0 *models.Perfil
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) *models.Perfil); ok {
		r0 = rf(ctx, userID)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Perfil)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, userID)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockPerfilRepository) Update(ctx context.Context, perfil *models.Perfil) (*models.Perfil, error) {
	args := m.Called(ctx, perfil)
	var r0 *models.Perfil
	if rf, ok := args.Get(0).(func(context.Context, *models.Perfil) *models.Perfil); ok {
		r0 = rf(ctx, perfil)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.Perfil)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *models.Perfil) error); ok {
		r1 = rf(ctx, perfil)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

// MockRefreshTokenRepository é um mock para RefreshTokenRepository.
type MockRefreshTokenRepository struct {
	mock.Mock
}

func (m *MockRefreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error) {
	args := m.Called(ctx, token)
	var r0 *models.RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, *models.RefreshToken) *models.RefreshToken); ok {
		r0 = rf(ctx, token)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.RefreshToken)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, *models.RefreshToken) error); ok {
		r1 = rf(ctx, token)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockRefreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	args := m.Called(ctx, tokenHash)
	var r0 *models.RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, string) *models.RefreshToken); ok {
		r0 = rf(ctx, tokenHash)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.RefreshToken)
		}
	}
	var r1 error
	if rf, ok := args.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, tokenHash)
	} else {
		r1 = args.Error(1)
	}
	return r0, r1
}

func (m *MockRefreshTokenRepository) FindActiveByFamilyID(ctx context.Context, familyID uuid.UUID) (*models.RefreshToken, error) {
	args := m.Called(ctx, familyID)
	var r0 *models.RefreshToken
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) *models.RefreshToken); ok {
		r0 = rf(ctx, familyID)
	} else {
		if args.Get(0) != nil {
			r0 = args.Get(0).(*models.RefreshToken)
		}
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
	var r0 error
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = args.Error(0)
	}
	return r0
}

func (m *MockRefreshTokenRepository) RevokeAllByFamilyID(ctx context.Context, familyID uuid.UUID) error {
	args := m.Called(ctx, familyID)
	var r0 error
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, familyID)
	} else {
		r0 = args.Error(0)
	}
	return r0
}

func (m *MockRefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	var r0 error
	if rf, ok := args.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, userID)
	} else {
		r0 = args.Error(0)
	}
	return r0
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
