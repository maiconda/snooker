package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrExpiredToken  = errors.New("expired token")
	ErrTokenRevoked  = errors.New("token revoked")
	ErrRTRFraud      = errors.New("refresh token reuse detected (fraud)")
	ErrNoActiveToken = errors.New("no active token in family")
)

type JWTClaims struct {
	Email  string `json:"email"`
	Status string `json:"status"`
	jwt.RegisteredClaims
}

type TokenService struct {
	jwtSecret        string
	refreshTokenRepo RefreshTokenRepository
}

func NewTokenService(jwtSecret string, rtrRepo RefreshTokenRepository) *TokenService {
	return &TokenService{
		jwtSecret:        jwtSecret,
		refreshTokenRepo: rtrRepo,
	}
}

func (s *TokenService) GenerateAccessToken(user *Usuario) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Email:  user.Email,
		Status: string(user.Status),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("falha ao assinar JWT: %w", err)
	}

	return tokenString, nil
}

func (s *TokenService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metodo de assinatura inesperado: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *TokenService) GenerateRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	rawToken, err := generateRandomHex(32)
	if err != nil {
		return "", fmt.Errorf("falha ao gerar bytes aleatorios para token: %w", err)
	}

	tokenHash := s.HashToken(rawToken)
	familyID := uuid.New()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	rt := &RefreshToken{
		UserID:    userID,
		TokenHash: tokenHash,
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
		Revoked:   false,
	}

	_, err = s.refreshTokenRepo.Create(ctx, rt)
	if err != nil {
		return "", fmt.Errorf("falha ao salvar refresh token no banco: %w", err)
	}

	return rawToken, nil
}

func (s *TokenService) RotateRefreshToken(ctx context.Context, oldRawToken string) (newRawToken string, userID uuid.UUID, err error) {
	tokenHash := s.HashToken(oldRawToken)
	rt, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", uuid.Nil, ErrInvalidToken
		}
		return "", uuid.Nil, fmt.Errorf("erro ao buscar refresh token: %w", err)
	}

	if rt.IsExpired() {
		return "", uuid.Nil, ErrExpiredToken
	}

	if rt.IsActive() {
		if err := s.refreshTokenRepo.RevokeByID(ctx, rt.ID); err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao revogar token antigo: %w", err)
		}

		newRaw, err := generateRandomHex(32)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao gerar novo token: %w", err)
		}

		newRt := &RefreshToken{
			UserID:    rt.UserID,
			TokenHash: s.HashToken(newRaw),
			FamilyID:  rt.FamilyID,
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
			Revoked:   false,
		}

		_, err = s.refreshTokenRepo.Create(ctx, newRt)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao salvar novo refresh token: %w", err)
		}

		return newRaw, rt.UserID, nil
	}

	if rt.IsWithinGracePeriod() {
		activeRt, err := s.refreshTokenRepo.FindActiveByFamilyID(ctx, rt.FamilyID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return "", uuid.Nil, ErrNoActiveToken
			}
			return "", uuid.Nil, fmt.Errorf("erro ao buscar token ativo na familia: %w", err)
		}

		if err := s.refreshTokenRepo.RevokeByID(ctx, activeRt.ID); err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao rotacionar token ativo concorrente: %w", err)
		}

		newRaw, err := generateRandomHex(32)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao gerar novo token concorrente: %w", err)
		}

		newRt := &RefreshToken{
			UserID:    rt.UserID,
			TokenHash: s.HashToken(newRaw),
			FamilyID:  rt.FamilyID,
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
			Revoked:   false,
		}

		_, err = s.refreshTokenRepo.Create(ctx, newRt)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao salvar novo token concorrente: %w", err)
		}

		return newRaw, rt.UserID, nil
	}

	if err := s.refreshTokenRepo.RevokeAllByFamilyID(ctx, rt.FamilyID); err != nil {
		fmt.Printf("ERROR: falha ao revogar familia de tokens apos reuso: %v\n", err)
	}

	return "", uuid.Nil, ErrRTRFraud
}

func (s *TokenService) RevokeUserTokens(ctx context.Context, userID uuid.UUID) error {
	return s.refreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

func (s *TokenService) HashToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}

func generateRandomHex(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
