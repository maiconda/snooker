package models

import (
	"time"

	"github.com/google/uuid"
)

// UserStatus define os estados do usuário conforme spec 01.
type UserStatus string

// AuthProvider define os tipos de provider de autenticação suportados.
type AuthProvider string

const (
	StatusOnboardingPending UserStatus = "onboarding_pending"
	StatusActive            UserStatus = "active"
	StatusBlocked           UserStatus = "blocked"

	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

// Usuario representa a tabela `usuarios` no banco de dados.
// Spec: 01-database-and-models.md
type Usuario struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	Email        string       `json:"email" db:"email"`
	PasswordHash *string      `json:"-" db:"password_hash"` // NULL para provider google
	Provider     AuthProvider `json:"provider" db:"provider"`
	Status       UserStatus   `json:"status" db:"status"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`
}

// Perfil representa a tabela `perfis` no banco de dados.
// Spec: 01-database-and-models.md
type Perfil struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Bio         string    `json:"bio" db:"bio"`
	PhotoURL    string    `json:"photo_url" db:"photo_url"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// RefreshToken representa a tabela `refresh_tokens` no banco de dados.
// Spec: 01-database-and-models.md, 03-token-and-session.md
type RefreshToken struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	TokenHash string     `db:"token_hash"` // SHA-256 hash, nunca plaintext
	FamilyID  uuid.UUID  `db:"family_id"`  // Agrupamento por dispositivo/sessão
	ExpiresAt time.Time  `db:"expires_at"`
	Revoked   bool       `db:"revoked"`
	RevokedAt *time.Time `db:"revoked_at"` // Para controle de Grace Period
	CreatedAt time.Time  `db:"created_at"`
}

// IsExpired verifica se o refresh token expirou.
func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

// IsActive verifica se o refresh token é utilizável (não revogado e não expirado).
func (rt *RefreshToken) IsActive() bool {
	return !rt.Revoked && !rt.IsExpired()
}

// IsWithinGracePeriod verifica se o token foi revogado dentro do grace period (15s).
// Usado para lidar com requisições concorrentes de refresh (spec 03).
func (rt *RefreshToken) IsWithinGracePeriod() bool {
	if rt.RevokedAt == nil {
		return false
	}
	gracePeriod := 15 * time.Second
	return time.Since(*rt.RevokedAt) <= gracePeriod
}
