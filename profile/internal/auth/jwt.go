package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"snooker/profile/internal/httpx"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type JWTClaims struct {
	Email  string `json:"email"`
	Status string `json:"status"`
	jwt.RegisteredClaims
}

type TokenValidator struct {
	jwtSecret string
}

func NewTokenValidator(jwtSecret string) *TokenValidator {
	return &TokenValidator{jwtSecret: jwtSecret}
}

func (v *TokenValidator) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metodo de assinatura inesperado: %v", token.Header["alg"])
		}
		return []byte(v.jwtSecret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid || claims.Subject == "" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func Middleware(validator *TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Authorization header is required"},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: httpx.ErrCodeUnauthorized, Message: "Authorization header must be in format: Bearer <token>"},
			})
			return
		}

		claims, err := validator.ValidateAccessToken(parts[1])
		if err != nil {
			code := httpx.ErrCodeUnauthorized
			message := "Invalid or expired token"
			if errors.Is(err, ErrExpiredToken) || strings.Contains(err.Error(), "expired") {
				code = httpx.ErrCodeTokenExpired
				message = "Access token has expired"
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.ErrorResponse{
				Error: httpx.ErrorDetail{Code: code, Message: message},
			})
			return
		}

		c.Set(httpx.ContextKeyUserID, claims.Subject)
		c.Set(httpx.ContextKeyEmail, claims.Email)
		c.Set(httpx.ContextKeyStatus, claims.Status)
		c.Next()
	}
}
