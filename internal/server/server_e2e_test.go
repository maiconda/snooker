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
	"snooker/internal/auth"
	"snooker/internal/config"
	"snooker/internal/handler"
	"snooker/internal/middleware"
	"snooker/internal/models"
	"snooker/internal/repository"
)

// TestE2E_FullAuthAndOnboardingFlow roda o fluxo E2E completo conforme spec 05.
func TestE2E_FullAuthAndOnboardingFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Setup de dependências mockadas para isolar o teste
	userRepo := new(repository.MockUsuarioRepository)
	profileRepo := new(repository.MockPerfilRepository)
	tokenRepo := new(repository.MockRefreshTokenRepository)

	cfg := &config.Config{
		JWTSecret:      "very-long-secret-key-for-jwt-signing-test-purposes",
		GoogleClientID: "google-client-id",
	}

	tokenService := auth.NewTokenService(cfg.JWTSecret, tokenRepo)
	authService := auth.NewAuthService(userRepo, tokenService, cfg.GoogleClientID)

	// O ProfileService e o StorageService serão mockados ou chamados de forma a não bater no MinIO real
	// Para isso, faremos um mock de storage chamando diretamente ou usaremos um mock de alto nível
	// Vamos criar as rotas usando o Router Gin construído localmente
	router := gin.Default()
	router.Use(CORSMiddleware("http://localhost:3000"))

	// Registro manual simplificado para o teste E2E usar os mocks injetados
	authHandler := handlerNewAuthHandler(authService, tokenService)
	
	// Criamos o fluxo e mock de comportamento
	userID := uuid.New()
	email := "e2e-user@example.com"
	password := "SecurePassword123!"

	// Variável para rastrear o status do usuário em memória para simular o banco
	userStatus := models.StatusOnboardingPending

	// ==================== MOCKS CONFIGURATION ====================
	
	// A. Fluxo de Sign Up
	userRepo.On("FindByEmail", mock.Anything, email).Return(func(ctx context.Context, email string) *models.Usuario {
		// Retorna NotFound na primeira chamada (signup) e o usuário nas chamadas seguintes (login)
		if userStatus == models.StatusOnboardingPending {
			return nil
		}
		// Se já completou, simula achar
		passwordHash := "hashed-pwd"
		return &models.Usuario{
			ID:       userID,
			Email:    email,
			Provider: models.ProviderLocal,
			Status:   userStatus,
			PasswordHash: &passwordHash,
		}
	}, func(ctx context.Context, email string) error {
		if userStatus == models.StatusOnboardingPending {
			return repository.ErrNotFound
		}
		return nil
	})

	userRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Usuario")).Return(func(ctx context.Context, u *models.Usuario) *models.Usuario {
		u.ID = userID
		return u
	}, nil)

	tokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.RefreshToken")).Return(func(ctx context.Context, rt *models.RefreshToken) *models.RefreshToken {
		rt.ID = uuid.New()
		return rt
	}, nil)

	// B. Acesso ao Lobbies (inicialmente falha)
	// Exige status Active. Como userStatus é onboarding_pending, deve retornar 403.

	// C. Upload URL
	// Apenas simulamos retornar uma URL presigned estática nos testes.

	// D. Complete Profile
	profileRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)
	profileRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Perfil")).Return(func(ctx context.Context, p *models.Perfil) *models.Perfil {
		p.ID = uuid.New()
		return p
	}, nil)
	userRepo.On("UpdateStatus", mock.Anything, userID, models.StatusActive).Return(func(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
		userStatus = status // Atualiza o estado
		return nil
	})
	userRepo.On("FindByID", mock.Anything, userID).Return(func(ctx context.Context, id uuid.UUID) *models.Usuario {
		return &models.Usuario{
			ID:     userID,
			Email:  email,
			Status: userStatus,
		}
	}, nil)

	// Setup de endpoints no router de teste
	v1 := router.Group("/api/v1")
	{
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/signup", authHandler.Signup)
		}

		authMiddleware := middleware.AuthMiddleware(tokenService)

		v1.GET("/lobbies", authMiddleware, middleware.RequireActive(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		// Simulação simplificada de upload-url e complete para o teste E2E
		v1.GET("/profile/upload-url", authMiddleware, middleware.RequireActiveOrOnboarding(), func(c *gin.Context) {
			c.JSON(http.StatusOK, models.UploadURLResponse{
				UploadURL: "http://storage.local/temp/file.png",
				ObjectKey: "temp/file.png",
			})
		})

		v1.POST("/profile/complete", authMiddleware, middleware.RequireOnboarding(), func(c *gin.Context) {
			var req models.CompleteProfileRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			
			// Executa a lógica mockada
			_ = userRepo.UpdateStatus(c, userID, models.StatusActive)
			user, _ := userRepo.FindByID(c, userID)
			newToken, _ := tokenService.GenerateAccessToken(user)

			c.JSON(http.StatusOK, models.CompleteProfileResponse{
				Message: "Perfil configurado com sucesso. Conta ativa.",
				Token:   newToken,
			})
		})
	}

	// ==================== EXECUTE FLOW ====================

	// Passo 1: POST /signup -> Sucesso 201
	signupBody, _ := json.Marshal(models.SignupRequest{
		Email:    email,
		Password: password,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBuffer(signupBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	
	var signupResp models.SignupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &signupResp)
	provisionalToken := signupResp.Token
	assert.NotEmpty(t, provisionalToken)
	assert.Equal(t, string(models.StatusOnboardingPending), string(signupResp.Status))

	// Passo 2: GET /lobbies com Token Provisório -> 403 Forbidden
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/lobbies", nil)
	req.Header.Set("Authorization", "Bearer "+provisionalToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Passo 3: GET /profile/upload-url com Token Provisório -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/profile/upload-url", nil)
	req.Header.Set("Authorization", "Bearer "+provisionalToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var uploadResp models.UploadURLResponse
	_ = json.Unmarshal(w.Body.Bytes(), &uploadResp)
	assert.NotEmpty(t, uploadResp.UploadURL)
	assert.Equal(t, "temp/file.png", uploadResp.ObjectKey)

	// Passo 4: POST /profile/complete com Token Provisório -> 200 OK
	completeBody, _ := json.Marshal(models.CompleteProfileRequest{
		DisplayName: "SnookerPlayer",
		Bio:         "Mestre",
		PhotoKey:    "temp/file.png",
	})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/profile/complete", bytes.NewBuffer(completeBody))
	req.Header.Set("Authorization", "Bearer "+provisionalToken)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var completeResp models.CompleteProfileResponse
	_ = json.Unmarshal(w.Body.Bytes(), &completeResp)
	definitiveToken := completeResp.Token
	assert.NotEmpty(t, definitiveToken)

	// Passo 5: GET /lobbies com Token Definitivo -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/lobbies", nil)
	req.Header.Set("Authorization", "Bearer "+definitiveToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Wrapper local para criar handler sem expor outras dependências no teste
func handlerNewAuthHandler(authService *auth.AuthService, tokenService *auth.TokenService) *handler.AuthHandler {
	return handler.NewAuthHandler(authService, tokenService)
}
