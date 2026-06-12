package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("registro nao encontrado")
	ErrDuplicateEmail = errors.New("email ja cadastrado")
	ErrDuplicateKey   = errors.New("chave duplicada")
)

type UsuarioRepository interface {
	Create(ctx context.Context, usuario *Usuario) (*Usuario, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Usuario, error)
	FindByEmail(ctx context.Context, email string) (*Usuario, error)
	FindByProviderID(ctx context.Context, provider AuthProvider, providerID string) (*Usuario, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status UserStatus) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) (*RefreshToken, error)
	FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	FindActiveByFamilyID(ctx context.Context, familyID uuid.UUID) (*RefreshToken, error)
	RevokeByID(ctx context.Context, id uuid.UUID) error
	RevokeAllByFamilyID(ctx context.Context, familyID uuid.UUID) error
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}
