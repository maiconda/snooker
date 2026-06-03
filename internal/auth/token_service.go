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
	"snooker/internal/models"
	"snooker/internal/repository"
)

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrExpiredToken   = errors.New("expired token")
	ErrTokenRevoked   = errors.New("token revoked")
	ErrRTRFraud       = errors.New("refresh token reuse detected (fraud)")
	ErrNoActiveToken  = errors.New("no active token in family")
)

// JWTClaims define a estrutura das claims do Access Token.
// Spec: 03-token-and-session.md
type JWTClaims struct {
	Email  string `json:"email"`
	Status string `json:"status"`
	jwt.RegisteredClaims
}

// TokenService gerencia o ciclo de vida de JWTs e Refresh Tokens.
// Spec: 03-token-and-session.md
type TokenService struct {
	jwtSecret        string
	refreshTokenRepo repository.RefreshTokenRepository
}

// NewTokenService cria uma nova instância de TokenService.
func NewTokenService(jwtSecret string, rtrRepo repository.RefreshTokenRepository) *TokenService {
	return &TokenService{
		jwtSecret:        jwtSecret,
		refreshTokenRepo: rtrRepo,
	}
}

// GenerateAccessToken gera um JWT de 15 minutos assinado com HS256.
// Spec: 03-token-and-session.md - claims: sub, email, status, iat, exp
func (s *TokenService) GenerateAccessToken(user *models.Usuario) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Email:  user.Email,
		Status: string(user.Status),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)), // 15 minutos estritamente
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", fmt.Errorf("falha ao assinar JWT: %w", err)
	}

	return tokenString, nil
}

// ValidateAccessToken valida e parseia o JWT claims.
func (s *TokenService) ValidateAccessToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Garante que o método de assinatura é HS256
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de assinatura inesperado: %v", token.Header["alg"])
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

// GenerateRefreshToken cria um novo refresh token opaco e salva seu hash no banco.
// Spec: 03-token-and-session.md, 07-token-policy.md - 64 bytes opaco
func (s *TokenService) GenerateRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	rawToken, err := generateRandomHex(32) // 32 bytes = 64 chars em hex
	if err != nil {
		return "", fmt.Errorf("falha ao gerar bytes aleatórios para token: %w", err)
	}

	tokenHash := s.HashToken(rawToken)
	familyID := uuid.New()
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 dias

	rt := &models.RefreshToken{
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

// RotateRefreshToken rotaciona o refresh token seguindo a política de RTR com Grace Period.
// Spec: 03-token-and-session.md (RTR Grace Period & Fraud Detection)
func (s *TokenService) RotateRefreshToken(ctx context.Context, oldRawToken string) (newRawToken string, userID uuid.UUID, err error) {
	tokenHash := s.HashToken(oldRawToken)
	rt, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", uuid.Nil, ErrInvalidToken
		}
		return "", uuid.Nil, fmt.Errorf("erro ao buscar refresh token: %w", err)
	}

	// 1. Verifica se expirou
	if rt.IsExpired() {
		return "", uuid.Nil, ErrExpiredToken
	}

	// 2. Se o token estiver ativo, realiza fluxo normal de rotação
	if rt.IsActive() {
		// Revoga o token antigo
		if err := s.refreshTokenRepo.RevokeByID(ctx, rt.ID); err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao revogar token antigo: %w", err)
		}

		// Cria novo token na mesma família
		newRaw, err := generateRandomHex(32)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao gerar novo token: %w", err)
		}

		newHash := s.HashToken(newRaw)
		newRt := &models.RefreshToken{
			UserID:    rt.UserID,
			TokenHash: newHash,
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

	// 3. Se estiver revogado, verifica se está dentro do Grace Period (15s) para concorrência
	if rt.IsWithinGracePeriod() {
		// Concorrência detectada. Busca o token ativo atual da família.
		activeRt, err := s.refreshTokenRepo.FindActiveByFamilyID(ctx, rt.FamilyID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return "", uuid.Nil, ErrNoActiveToken
			}
			return "", uuid.Nil, fmt.Errorf("erro ao buscar token ativo na família: %w", err)
		}

		// Para a requisição concorrente receber um token válido e utilizável sem precisar logar de novo,
		// nós geramos um novo token ativo na mesma família e revogamos o token ativo anterior da família
		// (para que o cliente tenha o cookie mais recente atualizado).
		if err := s.refreshTokenRepo.RevokeByID(ctx, activeRt.ID); err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao rotacionar token ativo concorrente: %w", err)
		}

		newRaw, err := generateRandomHex(32)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("falha ao gerar novo token concorrente: %w", err)
		}

		newHash := s.HashToken(newRaw)
		newRt := &models.RefreshToken{
			UserID:    rt.UserID,
			TokenHash: newHash,
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

	// 4. Se estiver revogado e fora do Grace Period: ALERTA DE FRAUDE / REUSO!
	// Revoga TODOS os tokens da família para bloquear a sessão imediatamente.
	if err := s.refreshTokenRepo.RevokeAllByFamilyID(ctx, rt.FamilyID); err != nil {
		// Loga mas prossegue retornando o erro de fraude
		fmt.Printf("ERROR: falha ao revogar família de tokens após reuso: %v\n", err)
	}

	return "", uuid.Nil, ErrRTRFraud
}

// RevokeUserTokens revoga todos os tokens ativos de um usuário (logout global).
func (s *TokenService) RevokeUserTokens(ctx context.Context, userID uuid.UUID) error {
	return s.refreshTokenRepo.RevokeAllByUserID(ctx, userID)
}

// HashToken calcula o SHA-256 do token em string hexadecimal.
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
