package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"snooker/auth/internal/httpx"
)

type Handler struct {
	authService  *AuthService
	tokenService *TokenService
	cookieSecure bool
}

type HandlerOption func(*Handler)

func WithSecureRefreshCookie(secure bool) HandlerOption {
	return func(h *Handler) {
		h.cookieSecure = secure
	}
}

func NewHandler(authService *AuthService, tokenService *TokenService, opts ...HandlerOption) *Handler {
	handler := &Handler{
		authService:  authService,
		tokenService: tokenService,
		cookieSecure: true,
	}

	for _, opt := range opts {
		opt(handler)
	}

	return handler
}

func (h *Handler) Signup(c *gin.Context) {
	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeValidationFailed,
				Message: "Invalid request body",
				Details: httpx.ValidationDetails(err),
			},
		})
		return
	}

	resp, rawRefreshToken, err := h.authService.Signup(c.Request.Context(), &req)
	if err != nil {
		handleAuthError(c, err)
		return
	}

	h.setRefreshTokenCookie(c, rawRefreshToken)
	c.JSON(http.StatusCreated, resp)
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeValidationFailed,
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

	h.setRefreshTokenCookie(c, rawRefreshToken)
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) GoogleAuth(c *gin.Context) {
	var req GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeValidationFailed,
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

	h.setRefreshTokenCookie(c, rawRefreshToken)

	statusCode := http.StatusOK
	if resp.Status == StatusOnboardingPending {
		statusCode = http.StatusCreated
	}
	c.JSON(statusCode, resp)
}

func (h *Handler) Refresh(c *gin.Context) {
	refreshCookie, err := c.Cookie("refresh_token")
	if err != nil || refreshCookie == "" {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeUnauthorized,
				Message: "Refresh token cookie is missing",
			},
		})
		return
	}

	newRawToken, userID, err := h.tokenService.RotateRefreshToken(c.Request.Context(), refreshCookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeUnauthorized,
				Message: "Invalid or expired refresh token",
			},
		})
		return
	}

	accessToken, err := h.authService.IssueAccessTokenForUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeInternal,
				Message: "Failed to generate access token",
			},
		})
		return
	}

	h.setRefreshTokenCookie(c, newRawToken)
	c.JSON(http.StatusOK, RefreshResponse{
		AccessToken: accessToken,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeUnauthorized,
				Message: "User not authenticated",
			},
		})
		return
	}

	if err := h.tokenService.RevokeUserTokens(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeInternal,
				Message: "Failed to revoke tokens",
			},
		})
		return
	}

	h.clearRefreshTokenCookie(c)
	c.JSON(http.StatusOK, httpx.MessageResponse{
		Message: "Sessao encerrada com sucesso",
	})
}

func (h *Handler) setRefreshTokenCookie(c *gin.Context, rawToken string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		rawToken,
		604800,
		"/api/v1/auth",
		"",
		h.cookieSecure,
		true,
	)
}

func (h *Handler) clearRefreshTokenCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(
		"refresh_token",
		"",
		0,
		"/api/v1/auth",
		"",
		h.cookieSecure,
		true,
	)
}

func handleAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeInvalidCredentials,
				Message: "Invalid email or password",
			},
		})
	case errors.Is(err, ErrEmailAlreadyExists):
		c.JSON(http.StatusConflict, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeEmailAlreadyExists,
				Message: "An account with this email already exists",
			},
		})
	default:
		c.JSON(http.StatusInternalServerError, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeInternal,
				Message: "An unexpected error occurred",
			},
		})
	}
}

func userIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get(httpx.ContextKeyUserID)
	if !exists {
		return uuid.Nil, false
	}
	userIDStr, ok := value.(string)
	if !ok {
		return uuid.Nil, false
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, false
	}
	return userID, true
}
