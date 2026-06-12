package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"snooker/auth/internal/auth"
)

func TestE2E_AuthSignupAndLogoutFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userRepo := new(auth.MockUsuarioRepository)
	tokenRepo := new(auth.MockRefreshTokenRepository)
	tokenService := auth.NewTokenService("very-long-secret-key-for-jwt-signing-test-purposes", tokenRepo)
	authService := auth.NewAuthService(userRepo, tokenService, "google-client-id")
	authHandler := auth.NewHandler(authService, tokenService)

	router := gin.Default()
	router.Use(CORSMiddleware("http://localhost:3000"))

	v1 := router.Group("/api/v1")
	authGroup := v1.Group("/auth")
	authGroup.POST("/signup", authHandler.Signup)
	authGroup.POST("/logout", auth.AuthMiddleware(tokenService), authHandler.Logout)

	userID := uuid.New()
	email := "e2e-user@example.com"
	password := "SecurePassword123!"

	userRepo.On("FindByEmail", mock.Anything, email).Return(nil, auth.ErrNotFound)
	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.Usuario")).Return(func(ctx context.Context, u *auth.Usuario) *auth.Usuario {
		u.ID = userID
		return u
	}, nil)
	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*auth.RefreshToken")).Return(func(ctx context.Context, rt *auth.RefreshToken) *auth.RefreshToken {
		rt.ID = uuid.New()
		return rt
	}, nil)
	tokenRepo.On("RevokeAllByUserID", mock.Anything, userID).Return(nil)

	signupBody, _ := json.Marshal(auth.SignupRequest{
		Email:    email,
		Password: password,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBuffer(signupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var signupResp auth.SignupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &signupResp)
	assert.NotEmpty(t, signupResp.Token)
	assert.Equal(t, auth.StatusOnboardingPending, signupResp.Status)

	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+signupResp.Token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	userRepo.AssertExpectations(t)
	tokenRepo.AssertExpectations(t)
}

func TestCORSMiddleware_RejectsUnknownOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORSMiddleware("http://localhost:3000"))
	router.OPTIONS("/api/v1/auth/login", func(c *gin.Context) {})

	req, _ := http.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://evil.local")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}
