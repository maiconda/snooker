package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"snooker/auth/internal/httpx"
)

func AuthMiddleware(tokenService *TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    httpx.ErrCodeUnauthorized,
					Message: "Authorization header is required",
				},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    httpx.ErrCodeUnauthorized,
					Message: "Authorization header must be in format: Bearer <token>",
				},
			})
			return
		}

		claims, err := tokenService.ValidateAccessToken(parts[1])
		if err != nil {
			code := httpx.ErrCodeUnauthorized
			message := "Invalid or expired token"
			if strings.Contains(err.Error(), "expired") {
				code = httpx.ErrCodeTokenExpired
				message = "Access token has expired"
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    code,
					Message: message,
				},
			})
			return
		}

		c.Set(httpx.ContextKeyUserID, claims.Subject)
		c.Set(httpx.ContextKeyEmail, claims.Email)
		c.Set(httpx.ContextKeyStatus, claims.Status)
		c.Next()
	}
}

func RequireStatus(allowedStatuses ...UserStatus) gin.HandlerFunc {
	return func(c *gin.Context) {
		statusStr, exists := c.Get(httpx.ContextKeyStatus)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{
					Code:    httpx.ErrCodeUnauthorized,
					Message: "User status not found in context",
				},
			})
			return
		}

		currentStatus := UserStatus(statusStr.(string))
		for _, allowed := range allowedStatuses {
			if currentStatus == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, httpx.ErrorResponse{
			Error: httpx.ErrorDetail{
				Code:    httpx.ErrCodeOnboardingPending,
				Message: "Your account status does not allow access to this resource",
			},
		})
	}
}

func RequireOnboarding() gin.HandlerFunc {
	return RequireStatus(StatusOnboardingPending)
}

func RequireActive() gin.HandlerFunc {
	return RequireStatus(StatusActive)
}

func RequireActiveOrOnboarding() gin.HandlerFunc {
	return RequireStatus(StatusActive, StatusOnboardingPending)
}
