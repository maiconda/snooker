package models

import "github.com/google/uuid"

// ==================== REQUEST DTOs ====================
// Spec: 02-api-endpoints.md

// SignupRequest representa o body de POST /api/v1/auth/signup
type SignupRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

// LoginRequest representa o body de POST /api/v1/auth/login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// GoogleAuthRequest representa o body de POST /api/v1/auth/google
type GoogleAuthRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

// CompleteProfileRequest representa o body de POST /api/v1/profile/complete
type CompleteProfileRequest struct {
	DisplayName string `json:"display_name" binding:"required,min=3,max=50"`
	Bio         string `json:"bio" binding:"omitempty,max=200"`
	PhotoKey    string `json:"photo_key" binding:"omitempty"`
}

// ==================== RESPONSE DTOs ====================

// SignupResponse é a resposta para POST /api/v1/auth/signup (201)
type SignupResponse struct {
	Message string     `json:"message"`
	Token   string     `json:"token"`
	Status  UserStatus `json:"status"`
}

// LoginResponse é a resposta para POST /api/v1/auth/login (200)
type LoginResponse struct {
	Token  string     `json:"token"`
	Status UserStatus `json:"status"`
}

// GoogleAuthResponse é a resposta para POST /api/v1/auth/google (200/201)
type GoogleAuthResponse struct {
	Token  string     `json:"token"`
	Status UserStatus `json:"status"`
}

// RefreshResponse é a resposta para POST /api/v1/auth/refresh (200)
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}

// UploadURLResponse é a resposta para GET /api/v1/profile/upload-url (200)
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	ObjectKey string `json:"object_key"`
}

// CompleteProfileResponse é a resposta para POST /api/v1/profile/complete (200)
type CompleteProfileResponse struct {
	Message string `json:"message"`
	Token   string `json:"token"`
}

// MessageResponse é uma resposta genérica de sucesso com mensagem.
type MessageResponse struct {
	Message string `json:"message"`
}

// UserResponse representa os dados públicos do usuário.
type UserResponse struct {
	ID       uuid.UUID        `json:"id"`
	Email    string           `json:"email"`
	Provider AuthProvider     `json:"provider"`
	Status   UserStatus       `json:"status"`
	Profile  *ProfileResponse `json:"profile,omitempty"`
}

// ProfileResponse representa os dados públicos do perfil.
type ProfileResponse struct {
	DisplayName string `json:"display_name"`
	PhotoURL    string `json:"photo_url"`
	Bio         string `json:"bio"`
}

// ==================== ERROR RESPONSE ====================
// Spec: 02-api-endpoints.md - Error Pattern

// ErrorResponse é a resposta padrão de erro da API.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contém os detalhes do erro.
type ErrorDetail struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

// FieldError representa um erro de validação em campo específico.
type FieldError struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// Business error codes conforme spec 02.
const (
	ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrCodeEmailAlreadyExists = "EMAIL_ALREADY_EXISTS"
	ErrCodeOnboardingPending  = "ONBOARDING_PENDING"
	ErrCodeValidationFailed   = "VALIDATION_FAILED"
	ErrCodeTokenExpired       = "TOKEN_EXPIRED"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
)
