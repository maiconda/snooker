package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"snooker/internal/auth"
	"snooker/internal/middleware"
	"snooker/internal/models"
	"snooker/internal/profile"
)

// AuthHandler gerencia os endpoints de autenticação.
type AuthHandler struct {
	authService  *auth.AuthService
	tokenService *auth.TokenService
}

// NewAuthHandler cria uma nova instância de AuthHandler.
func NewAuthHandler(authService *auth.AuthService, tokenService *auth.TokenService) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		tokenService: tokenService,
	}
}

// Signup handler - POST /api/v1/auth/signup
// Spec: 02-api-endpoints.md - Creates local account with email/password
func (h *AuthHandler) Signup(c *gin.Context) {
	var req models.SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeValidationFailed,
				Message: "Invalid request body",
				Details: parseValidationErrors(err),
			},
		})
		return
	}

	resp, rawRefreshToken, err := h.authService.Signup(c.Request.Context(), &req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	setRefreshTokenCookie(c, rawRefreshToken)
	c.JSON(http.StatusCreated, resp)
}

// Login handler - POST /api/v1/auth/login
// Spec: 02-api-endpoints.md - Authenticates with email/password
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeValidationFailed,
				Message: "Invalid request body",
			},
		})
		return
	}

	resp, rawRefreshToken, err := h.authService.Login(c.Request.Context(), &req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	setRefreshTokenCookie(c, rawRefreshToken)
	c.JSON(http.StatusOK, resp)
}

// GoogleAuth handler - POST /api/v1/auth/google
// Spec: 02-api-endpoints.md - Authenticates with Google id_token
func (h *AuthHandler) GoogleAuth(c *gin.Context) {
	var req models.GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeValidationFailed,
				Message: "Invalid request body",
			},
		})
		return
	}

	resp, rawRefreshToken, err := h.authService.GoogleAuth(c.Request.Context(), &req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	setRefreshTokenCookie(c, rawRefreshToken)

	// Determina status code: 201 se novo usuário, 200 se existente
	statusCode := http.StatusOK
	if resp.Status == models.StatusOnboardingPending {
		statusCode = http.StatusCreated
	}
	c.JSON(statusCode, resp)
}

// Refresh handler - POST /api/v1/auth/refresh
// Spec: 03-token-and-session.md - RTR com grace period
func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshCookie, err := c.Cookie("refresh_token")
	if err != nil || refreshCookie == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeUnauthorized,
				Message: "Refresh token cookie is missing",
			},
		})
		return
	}

	newRawToken, userID, err := h.tokenService.RotateRefreshToken(c.Request.Context(), refreshCookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeUnauthorized,
				Message: "Invalid or expired refresh token",
			},
		})
		return
	}

	// Busca o usuário para gerar novo access token com claims atuais
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to retrieve user data",
			},
		})
		return
	}

	accessToken, err := h.tokenService.GenerateAccessToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to generate access token",
			},
		})
		return
	}

	setRefreshTokenCookie(c, newRawToken)
	c.JSON(http.StatusOK, models.RefreshResponse{
		AccessToken: accessToken,
	})
}

// Logout handler - POST /api/v1/auth/logout
// Spec: 02-api-endpoints.md - Revokes session
func (h *AuthHandler) Logout(c *gin.Context) {
	userIDStr, exists := c.Get(middleware.ContextKeyUserID)
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeUnauthorized,
				Message: "User not authenticated",
			},
		})
		return
	}

	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Invalid user ID",
			},
		})
		return
	}

	// Revoga todos os tokens do usuário
	if err := h.tokenService.RevokeUserTokens(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to revoke tokens",
			},
		})
		return
	}

	// Limpa o cookie de refresh token
	// Spec: Set-Cookie com Max-Age=0 para expirar
	clearRefreshTokenCookie(c)
	c.JSON(http.StatusOK, models.MessageResponse{
		Message: "Sessão encerrada com sucesso",
	})
}

// ==================== Profile Handler ====================

// ProfileHandler gerencia os endpoints de perfil.
type ProfileHandler struct {
	profileService *profile.ProfileService
	tokenService   *auth.TokenService
	authService    *auth.AuthService
}

// NewProfileHandler cria uma nova instância de ProfileHandler.
func NewProfileHandler(
	profileService *profile.ProfileService,
	tokenService *auth.TokenService,
	authService *auth.AuthService,
) *ProfileHandler {
	return &ProfileHandler{
		profileService: profileService,
		tokenService:   tokenService,
		authService:    authService,
	}
}

// GetUploadURL handler - GET /api/v1/profile/upload-url
// Spec: 02-api-endpoints.md - Generates presigned upload URL
func (h *ProfileHandler) GetUploadURL(c *gin.Context) {
	resp, err := h.profileService.GetUploadURL(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to generate upload URL",
			},
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// CompleteProfile handler - POST /api/v1/profile/complete
// Spec: 02-api-endpoints.md - Completes onboarding, transitions to active
func (h *ProfileHandler) CompleteProfile(c *gin.Context) {
	var req models.CompleteProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeValidationFailed,
				Message: "Invalid request body",
				Details: parseValidationErrors(err),
			},
		})
		return
	}

	userIDStr, _ := c.Get(middleware.ContextKeyUserID)
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Invalid user ID",
			},
		})
		return
	}

	if err := h.profileService.CompleteProfile(c.Request.Context(), userID, &req); err != nil {
		if err == profile.ErrProfileAlreadyExists {
			c.JSON(http.StatusConflict, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "PROFILE_EXISTS",
					Message: "Profile already exists for this user",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to complete profile",
			},
		})
		return
	}

	// Gera novo token com status active
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to retrieve updated user",
			},
		})
		return
	}

	newToken, err := h.tokenService.GenerateAccessToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "Failed to generate new token",
			},
		})
		return
	}

	c.JSON(http.StatusOK, models.CompleteProfileResponse{
		Message: "Perfil configurado com sucesso. Conta ativa.",
		Token:   newToken,
	})
}

// ==================== Helpers ====================

// setRefreshTokenCookie configura o cookie de refresh token.
// Spec: 03-token-and-session.md - HttpOnly, Secure, SameSite=Strict, Path=/api/v1/auth, Max-Age=604800
func setRefreshTokenCookie(c *gin.Context, rawToken string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",       // name
		rawToken,              // value
		604800,                // maxAge (7 days in seconds)
		"/api/v1/auth",        // path
		"",                    // domain (empty = request host)
		true,                  // secure
		true,                  // httpOnly
	)
}

// clearRefreshTokenCookie expira o cookie de refresh token.
// Spec: Set-Cookie com Max-Age=0
func clearRefreshTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		"",
		0,                // Max-Age=0 expira imediatamente
		"/api/v1/auth",
		"",
		true,
		true,
	)
}

// handleAuthError converte erros de domínio em respostas HTTP.
func handleAuthError(c *gin.Context, err error) {
	switch err {
	case auth.ErrInvalidCredentials:
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeInvalidCredentials,
				Message: "Invalid email or password",
			},
		})
	case auth.ErrEmailAlreadyExists:
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeEmailAlreadyExists,
				Message: "An account with this email already exists",
			},
		})
	default:
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    "INTERNAL_ERROR",
				Message: "An unexpected error occurred",
			},
		})
	}
}

// parseValidationErrors tenta extrair erros de validação de campos.
func parseValidationErrors(err error) []models.FieldError {
	// Gin validation errors não são triviais de parsear genericamente
	// Retornamos uma mensagem genérica por enquanto
	return []models.FieldError{
		{
			Field: "body",
			Issue: err.Error(),
		},
	}
}
