package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
	"snooker/internal/models"
	"snooker/internal/repository"
)

func TestAuthService_ValidatePassword(t *testing.T) {
	service := NewAuthService(nil, nil, "client-id")

	tests := []struct {
		name     string
		password string
		isValid  bool
	}{
		{"valid password", "StrongPass123!", true},
		{"too short", "Pass1!", false},
		{"no uppercase", "strongpass123!", false},
		{"no lowercase", "STRONGPASS123!", false},
		{"no number", "StrongPass!", false},
		{"no special", "StrongPass123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := service.ValidatePassword(tt.password)
			if tt.isValid {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
			}
		})
	}
}

func TestAuthService_Signup_Success(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	tokenRepo := new(repository.MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &models.SignupRequest{
		Email:    "newuser@example.com",
		Password: "SecurePassword123!",
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(nil, repository.ErrNotFound)

	var createdUser *models.Usuario
	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Usuario")).Return(func(ctx context.Context, u *models.Usuario) *models.Usuario {
		assert.Equal(t, req.Email, u.Email)
		assert.Equal(t, models.ProviderLocal, u.Provider)
		assert.Equal(t, models.StatusOnboardingPending, u.Status)
		assert.NotEmpty(t, *u.PasswordHash)

		// Verifica se a senha salva é um hash bcrypt válido
		err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(req.Password))
		assert.NoError(t, err)

		u.ID = uuid.New()
		createdUser = u
		return u
	}, nil)

	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.RefreshToken")).Return(func(ctx context.Context, rt *models.RefreshToken) *models.RefreshToken {
		assert.Equal(t, createdUser.ID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.Signup(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, models.StatusOnboardingPending, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_Signup_DuplicateEmail(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	service := NewAuthService(userRepo, nil, "client-id")

	req := &models.SignupRequest{
		Email:    "existing@example.com",
		Password: "SecurePassword123!",
	}

	existingUser := &models.Usuario{
		ID:    uuid.New(),
		Email: req.Email,
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(existingUser, nil)

	_, _, err := service.Signup(context.Background(), req)
	assert.ErrorIs(t, err, ErrEmailAlreadyExists)

	userRepo.AssertExpectations(t)
}

func TestAuthService_Login_Success(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	tokenRepo := new(repository.MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	password := "SecurePassword123!"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	passwordHash := string(hashedBytes)

	user := &models.Usuario{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: &passwordHash,
		Provider:     models.ProviderLocal,
		Status:       models.StatusActive,
	}

	req := &models.LoginRequest{
		Email:    user.Email,
		Password: password,
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(user, nil)
	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.RefreshToken")).Return(func(ctx context.Context, rt *models.RefreshToken) *models.RefreshToken {
		assert.Equal(t, user.ID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.Login(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, models.StatusActive, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	service := NewAuthService(userRepo, nil, "client-id")

	req := &models.LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "WrongPassword123!",
	}

	// Caso 1: Usuário não existe
	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(nil, repository.ErrNotFound).Once()
	_, _, err := service.Login(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	// Caso 2: Senha incorreta
	password := "CorrectPassword123!"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	passwordHash := string(hashedBytes)
	user := &models.Usuario{
		Email:        "existing@example.com",
		PasswordHash: &passwordHash,
		Provider:     models.ProviderLocal,
	}
	req.Email = user.Email

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(user, nil).Once()
	_, _, err = service.Login(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	userRepo.AssertExpectations(t)
}

func TestAuthService_GoogleAuth_ExistingUser(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	tokenRepo := new(repository.MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &models.GoogleAuthRequest{
		IDToken: "valid-google-id-token-long-enough",
	}

	userID := uuid.New()
	existingUser := &models.Usuario{
		ID:       userID,
		Email:    "google-test-user@gmail.com",
		Provider: models.ProviderGoogle,
		Status:   models.StatusActive,
	}

	// Primeiro busca pelo provider + sub (nosso stub busca pelo provider e email do stub)
	userRepo.On("FindByProviderID", mock.Anything, models.ProviderGoogle, "google-sub-12345").Return(existingUser, nil)
	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.RefreshToken")).Return(func(ctx context.Context, rt *models.RefreshToken) *models.RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.GoogleAuth(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, models.StatusActive, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_GoogleAuth_NewUser(t *testing.T) {
	userRepo := new(repository.MockUsuarioRepository)
	tokenRepo := new(repository.MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &models.GoogleAuthRequest{
		IDToken: "valid-google-id-token-long-enough",
	}

	userID := uuid.New()

	// 1. Busca por provider/sub retorna NotFound
	userRepo.On("FindByProviderID", mock.Anything, models.ProviderGoogle, "google-sub-12345").Return(nil, repository.ErrNotFound)
	
	// 2. Busca por email no banco retorna NotFound
	userRepo.On("FindByEmail", mock.Anything, "google-test-user@gmail.com").Return(nil, repository.ErrNotFound)

	// 3. Cadastra o novo usuário
	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Usuario")).Return(func(ctx context.Context, u *models.Usuario) *models.Usuario {
		assert.Equal(t, "google-test-user@gmail.com", u.Email)
		assert.Equal(t, models.ProviderGoogle, u.Provider)
		assert.Equal(t, models.StatusOnboardingPending, u.Status)
		assert.Nil(t, u.PasswordHash) // Novo login social não tem password_hash
		u.ID = userID
		return u
	}, nil)

	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.RefreshToken")).Return(func(ctx context.Context, rt *models.RefreshToken) *models.RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.GoogleAuth(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, models.StatusOnboardingPending, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}
