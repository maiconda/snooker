package repository

import (
	"context"

	"github.com/google/uuid"
	"snooker/internal/models"
)

// UsuarioRepository define as operações de persistência para usuarios.
// Spec: 01-database-and-models.md
type UsuarioRepository interface {
	// Create insere um novo usuário e retorna o registro criado.
	Create(ctx context.Context, usuario *models.Usuario) (*models.Usuario, error)

	// FindByID busca um usuário pelo ID.
	FindByID(ctx context.Context, id uuid.UUID) (*models.Usuario, error)

	// FindByEmail busca um usuário pelo email.
	FindByEmail(ctx context.Context, email string) (*models.Usuario, error)

	// FindByProviderID busca um usuário pelo provider + provider_id (Google sub).
	FindByProviderID(ctx context.Context, provider models.AuthProvider, providerID string) (*models.Usuario, error)

	// UpdateStatus atualiza o status do usuário (state machine).
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error
}

// PerfilRepository define as operações de persistência para perfis.
// Spec: 01-database-and-models.md
type PerfilRepository interface {
	// Create insere um novo perfil e retorna o registro criado.
	Create(ctx context.Context, perfil *models.Perfil) (*models.Perfil, error)

	// FindByUserID busca o perfil pelo ID do usuário.
	FindByUserID(ctx context.Context, userID uuid.UUID) (*models.Perfil, error)

	// Update atualiza um perfil existente.
	Update(ctx context.Context, perfil *models.Perfil) (*models.Perfil, error)
}

// RefreshTokenRepository define as operações de persistência para refresh tokens.
// Spec: 01-database-and-models.md, 03-token-and-session.md
type RefreshTokenRepository interface {
	// Create insere um novo refresh token e retorna o registro criado.
	Create(ctx context.Context, token *models.RefreshToken) (*models.RefreshToken, error)

	// FindByTokenHash busca um refresh token pelo hash SHA-256.
	FindByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)

	// FindActiveByFamilyID busca o token ativo mais recente de uma família.
	// Usado no Grace Period para encontrar o token que substituiu o revogado.
	FindActiveByFamilyID(ctx context.Context, familyID uuid.UUID) (*models.RefreshToken, error)

	// RevokeByID revoga um refresh token específico (marca revoked=true, revoked_at=NOW).
	RevokeByID(ctx context.Context, id uuid.UUID) error

	// RevokeAllByFamilyID revoga todos os tokens de uma família (detecção de roubo/reuso).
	RevokeAllByFamilyID(ctx context.Context, familyID uuid.UUID) error

	// RevokeAllByUserID revoga todos os tokens de um usuário (logout global).
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error

	// DeleteExpired remove tokens expirados (cleanup periódico).
	DeleteExpired(ctx context.Context) (int64, error)
}
