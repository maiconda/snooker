package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
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
	userRepo := new(MockUsuarioRepository)
	tokenRepo := new(MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &SignupRequest{
		Email:    "newuser@example.com",
		Password: "SecurePassword123!",
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(nil, ErrNotFound)

	var createdUser *Usuario
	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.Usuario")).Return(func(ctx context.Context, u *Usuario) *Usuario {
		assert.Equal(t, req.Email, u.Email)
		assert.Equal(t, ProviderLocal, u.Provider)
		assert.Equal(t, StatusOnboardingPending, u.Status)
		assert.NotEmpty(t, *u.PasswordHash)

		err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(req.Password))
		assert.NoError(t, err)

		u.ID = uuid.New()
		createdUser = u
		return u
	}, nil)

	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, createdUser.ID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.Signup(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, StatusOnboardingPending, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_Signup_DuplicateEmail(t *testing.T) {
	userRepo := new(MockUsuarioRepository)
	service := NewAuthService(userRepo, nil, "client-id")

	req := &SignupRequest{
		Email:    "existing@example.com",
		Password: "SecurePassword123!",
	}

	existingUser := &Usuario{
		ID:    uuid.New(),
		Email: req.Email,
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(existingUser, nil)

	_, _, err := service.Signup(context.Background(), req)
	assert.ErrorIs(t, err, ErrEmailAlreadyExists)

	userRepo.AssertExpectations(t)
}

func TestAuthService_Login_Success(t *testing.T) {
	userRepo := new(MockUsuarioRepository)
	tokenRepo := new(MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	password := "SecurePassword123!"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	passwordHash := string(hashedBytes)

	user := &Usuario{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: &passwordHash,
		Provider:     ProviderLocal,
		Status:       StatusActive,
	}

	req := &LoginRequest{
		Email:    user.Email,
		Password: password,
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(user, nil)
	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, user.ID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.Login(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, StatusActive, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	userRepo := new(MockUsuarioRepository)
	service := NewAuthService(userRepo, nil, "client-id")

	req := &LoginRequest{
		Email:    "nonexistent@example.com",
		Password: "WrongPassword123!",
	}

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(nil, ErrNotFound).Once()
	_, _, err := service.Login(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	password := "CorrectPassword123!"
	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(password), 12)
	passwordHash := string(hashedBytes)
	user := &Usuario{
		Email:        "existing@example.com",
		PasswordHash: &passwordHash,
		Provider:     ProviderLocal,
	}
	req.Email = user.Email

	userRepo.On("FindByEmail", mock.Anything, req.Email).Return(user, nil).Once()
	_, _, err = service.Login(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	userRepo.AssertExpectations(t)
}

func TestAuthService_GoogleAuth_ExistingUser(t *testing.T) {
	userRepo := new(MockUsuarioRepository)
	tokenRepo := new(MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &GoogleAuthRequest{
		IDToken: "valid-google-id-token-long-enough",
	}

	userID := uuid.New()
	providerID := "google-sub-12345"
	existingUser := &Usuario{
		ID:         userID,
		Email:      "google-test-user@gmail.com",
		Provider:   ProviderGoogle,
		ProviderID: &providerID,
		Status:     StatusActive,
	}

	userRepo.On("FindByProviderID", mock.Anything, ProviderGoogle, providerID).Return(existingUser, nil)
	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.GoogleAuth(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, StatusActive, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestAuthService_GoogleAuth_NewUser(t *testing.T) {
	userRepo := new(MockUsuarioRepository)
	tokenRepo := new(MockRefreshTokenRepository)
	tokenService := NewTokenService("my-jwt-test-secret-key-that-must-be-very-long", tokenRepo)
	service := NewAuthService(userRepo, tokenService, "client-id")

	req := &GoogleAuthRequest{
		IDToken: "valid-google-id-token-long-enough",
	}

	userID := uuid.New()
	providerID := "google-sub-12345"

	userRepo.On("FindByProviderID", mock.Anything, ProviderGoogle, providerID).Return(nil, ErrNotFound)
	userRepo.On("FindByEmail", mock.Anything, "google-test-user@gmail.com").Return(nil, ErrNotFound)
	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.Usuario")).Return(func(ctx context.Context, u *Usuario) *Usuario {
		assert.Equal(t, "google-test-user@gmail.com", u.Email)
		assert.Equal(t, ProviderGoogle, u.Provider)
		assert.Equal(t, providerID, *u.ProviderID)
		assert.Equal(t, StatusOnboardingPending, u.Status)
		assert.Nil(t, u.PasswordHash)
		u.ID = userID
		return u
	}, nil)

	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		return rt
	}, nil)

	resp, rawRefresh, err := service.GoogleAuth(context.Background(), req)
	assert.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, StatusOnboardingPending, resp.Status)
	assert.NotEmpty(t, rawRefresh)

	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}
