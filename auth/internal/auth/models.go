package auth

import (
	"time"

	"github.com/google/uuid"
)

// UserStatus define os estados do usuario dentro do modulo de autenticacao.
type UserStatus string

// AuthProvider define os provedores de autenticacao suportados.
type AuthProvider string

const (
	StatusOnboardingPending UserStatus = "onboarding_pending"
	StatusActive            UserStatus = "active"
	StatusBlocked           UserStatus = "blocked"

	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

// Usuario representa a tabela `usuarios`.
type Usuario struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	Email        string       `json:"email" db:"email"`
	PasswordHash *string      `json:"-" db:"password_hash"`
	Provider     AuthProvider `json:"provider" db:"provider"`
	ProviderID   *string      `json:"-" db:"provider_id"`
	Status       UserStatus   `json:"status" db:"status"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`
}

// RefreshToken representa a tabela `refresh_tokens`.
type RefreshToken struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	TokenHash string     `db:"token_hash"`
	FamilyID  uuid.UUID  `db:"family_id"`
	ExpiresAt time.Time  `db:"expires_at"`
	Revoked   bool       `db:"revoked"`
	RevokedAt *time.Time `db:"revoked_at"`
	CreatedAt time.Time  `db:"created_at"`
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

func (rt *RefreshToken) IsActive() bool {
	return !rt.Revoked && !rt.IsExpired()
}

func (rt *RefreshToken) IsWithinGracePeriod() bool {
	if rt.RevokedAt == nil {
		return false
	}
	gracePeriod := 15 * time.Second
	return time.Since(*rt.RevokedAt) <= gracePeriod
}

type SignupRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleAuthRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

type SignupResponse struct {
	Message string     `json:"message"`
	Token   string     `json:"token"`
	Status  UserStatus `json:"status"`
}

type LoginResponse struct {
	Token  string     `json:"token"`
	Status UserStatus `json:"status"`
}

type GoogleAuthResponse struct {
	Token  string     `json:"token"`
	Status UserStatus `json:"status"`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}

type ActivateUserResponse struct {
	AccessToken string     `json:"access_token"`
	Status      UserStatus `json:"status"`
}
