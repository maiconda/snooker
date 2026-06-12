package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTokenService_GenerateAndValidateAccessToken(t *testing.T) {
	secret := "my-jwt-test-secret-key-that-must-be-very-long"
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService(secret, repo)

	userID := uuid.New()
	user := &Usuario{
		ID:     userID,
		Email:  "test@example.com",
		Status: StatusActive,
	}

	tokenStr, err := service.GenerateAccessToken(user)
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := service.ValidateAccessToken(tokenStr)
	assert.NoError(t, err)
	assert.Equal(t, user.Email, claims.Email)
	assert.Equal(t, string(user.Status), claims.Status)
	assert.Equal(t, userID.String(), claims.Subject)
}

func TestTokenService_ValidateAccessToken_Expired(t *testing.T) {
	secret := "my-jwt-test-secret-key-that-must-be-very-long"
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService(secret, repo)

	userID := uuid.New()
	user := &Usuario{
		ID:     userID,
		Email:  "test@example.com",
		Status: StatusActive,
	}

	tokenStr, err := service.GenerateAccessToken(user)
	assert.NoError(t, err)

	_, err = service.ValidateAccessToken(tokenStr)
	assert.NoError(t, err)

	invalidService := NewTokenService("wrong-secret-key", repo)
	_, err = invalidService.ValidateAccessToken(tokenStr)
	assert.Error(t, err)
}

func TestTokenService_GenerateRefreshToken_Success(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()

	repo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		assert.False(t, rt.Revoked)
		assert.NotEmpty(t, rt.TokenHash)
		return rt
	}, nil)

	rawToken, err := service.GenerateRefreshToken(context.Background(), userID)
	assert.NoError(t, err)
	assert.Len(t, rawToken, 64)
	repo.AssertExpectations(t)
}

func TestTokenService_GenerateRefreshToken_RepositoryError(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()

	repo.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("database connection down"))

	_, err := service.GenerateRefreshToken(context.Background(), userID)
	assert.ErrorContains(t, err, "falha ao salvar refresh token no banco")
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_NormalFlow(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()
	familyID := uuid.New()
	rawToken := "existing-raw-refresh-token-12345"
	hashedToken := service.HashToken(rawToken)

	existingToken := &RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashedToken,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   false,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)
	repo.On("RevokeByID", mock.Anything, existingToken.ID).Return(nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		assert.Equal(t, familyID, rt.FamilyID)
		assert.False(t, rt.Revoked)
		return rt
	}, nil)

	newRaw, gotUserID, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.NoError(t, err)
	assert.NotEmpty(t, newRaw)
	assert.Equal(t, userID, gotUserID)
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_Expired(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	rawToken := "expired-token"
	hashedToken := service.HashToken(rawToken)

	existingToken := &RefreshToken{
		ID:        uuid.New(),
		TokenHash: hashedToken,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		Revoked:   false,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)

	_, _, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.ErrorIs(t, err, ErrExpiredToken)
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_GracePeriod(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()
	familyID := uuid.New()
	rawToken := "revoked-within-grace-period"
	hashedToken := service.HashToken(rawToken)

	revokedAt := time.Now().Add(-5 * time.Second)
	existingToken := &RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashedToken,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   true,
		RevokedAt: &revokedAt,
	}

	activeToken := &RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: "some-other-hash",
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   false,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)
	repo.On("FindActiveByFamilyID", mock.Anything, familyID).Return(activeToken, nil)
	repo.On("RevokeByID", mock.Anything, activeToken.ID).Return(nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *RefreshToken) *RefreshToken {
		assert.Equal(t, userID, rt.UserID)
		assert.Equal(t, familyID, rt.FamilyID)
		assert.False(t, rt.Revoked)
		return rt
	}, nil)

	newRaw, gotUserID, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.NoError(t, err)
	assert.NotEmpty(t, newRaw)
	assert.Equal(t, userID, gotUserID)
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_FraudDetection(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()
	familyID := uuid.New()
	rawToken := "revoked-outside-grace-period"
	hashedToken := service.HashToken(rawToken)

	revokedAt := time.Now().Add(-30 * time.Second)
	existingToken := &RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hashedToken,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   true,
		RevokedAt: &revokedAt,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)
	repo.On("RevokeAllByFamilyID", mock.Anything, familyID).Return(nil)

	_, _, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.ErrorIs(t, err, ErrRTRFraud)
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_RepositoryErrorOnFind(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	rawToken := "error-token"
	hashedToken := service.HashToken(rawToken)

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(nil, errors.New("read failure"))

	_, _, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.ErrorContains(t, err, "erro ao buscar refresh token")
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_RepositoryErrorOnRevoke(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	rawToken := "revoke-fail-token"
	hashedToken := service.HashToken(rawToken)

	existingToken := &RefreshToken{
		ID:        uuid.New(),
		TokenHash: hashedToken,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   false,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)
	repo.On("RevokeByID", mock.Anything, existingToken.ID).Return(errors.New("db lock failure"))

	_, _, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.ErrorContains(t, err, "falha ao revogar token antigo")
	repo.AssertExpectations(t)
}

func TestTokenService_RotateRefreshToken_RepositoryErrorOnCreate(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	rawToken := "create-fail-token"
	hashedToken := service.HashToken(rawToken)

	existingToken := &RefreshToken{
		ID:        uuid.New(),
		TokenHash: hashedToken,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Revoked:   false,
	}

	repo.On("FindByTokenHash", mock.Anything, hashedToken).Return(existingToken, nil)
	repo.On("RevokeByID", mock.Anything, existingToken.ID).Return(nil)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("db insert failure"))

	_, _, err := service.RotateRefreshToken(context.Background(), rawToken)
	assert.ErrorContains(t, err, "falha ao salvar novo refresh token")
	repo.AssertExpectations(t)
}

func TestTokenService_RevokeUserTokens(t *testing.T) {
	repo := new(MockRefreshTokenRepository)
	service := NewTokenService("secret", repo)
	userID := uuid.New()

	repo.On("RevokeAllByUserID", mock.Anything, userID).Return(nil)

	err := service.RevokeUserTokens(context.Background(), userID)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}
