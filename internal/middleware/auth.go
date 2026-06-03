package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"snooker/internal/auth"
	"snooker/internal/models"
)

const (
	// ContextKeyUserID é a chave do contexto para o ID do usuário autenticado.
	ContextKeyUserID = "user_id"
	// ContextKeyEmail é a chave do contexto para o email do usuário autenticado.
	ContextKeyEmail = "user_email"
	// ContextKeyStatus é a chave do contexto para o status do usuário autenticado.
	ContextKeyStatus = "user_status"
)

// AuthMiddleware cria um middleware Gin que valida o JWT do header Authorization.
// Spec: 02-api-endpoints.md - Authorization: Bearer <Access Token>
func AuthMiddleware(tokenService *auth.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    models.ErrCodeUnauthorized,
					Message: "Authorization header is required",
				},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    models.ErrCodeUnauthorized,
					Message: "Authorization header must be in format: Bearer <token>",
				},
			})
			return
		}

		claims, err := tokenService.ValidateAccessToken(parts[1])
		if err != nil {
			code := models.ErrCodeUnauthorized
			message := "Invalid or expired token"

			if strings.Contains(err.Error(), "expired") {
				code = models.ErrCodeTokenExpired
				message = "Access token has expired"
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    code,
					Message: message,
				},
			})
			return
		}

		// Injeta dados do usuário no contexto do Gin
		c.Set(ContextKeyUserID, claims.Subject)
		c.Set(ContextKeyEmail, claims.Email)
		c.Set(ContextKeyStatus, claims.Status)

		c.Next()
	}
}

// RequireStatus cria um middleware que exige um status específico do usuário.
// Spec: onboarding_pending só pode acessar /upload-url e /profile/complete.
func RequireStatus(allowedStatuses ...models.UserStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		statusStr, exists := c.Get(ContextKeyStatus)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    models.ErrCodeUnauthorized,
					Message: "User status not found in context",
				},
			})
			return
		}

		currentStatus := models.UserStatus(statusStr.(string))
		for _, allowed := range allowedStatuses {
			if currentStatus == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, models.ErrorResponse{
			Error: models.ErrorDetail{
				Code:    models.ErrCodeOnboardingPending,
				Message: "Your account status does not allow access to this resource",
			},
		})
	}
}

// RequireOnboarding exige que o usuário esteja em status onboarding_pending.
func RequireOnboarding() gin.HandlerFunc {
	return RequireStatus(models.StatusOnboardingPending)
}

// RequireActive exige que o usuário esteja com status active.
func RequireActive() gin.HandlerFunc {
	return RequireStatus(models.StatusActive)
}

// RequireActiveOrOnboarding permite acesso a usuários active ou onboarding_pending.
func RequireActiveOrOnboarding() gin.HandlerFunc {
	return RequireStatus(models.StatusActive, models.StatusOnboardingPending)
}
