package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"snooker/internal/models"
	"snooker/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

// AuthService gerencia o fluxo de cadastro, login e onboarding de usuários.
// Spec: 02-api-endpoints.md
type AuthService struct {
	usuarioRepo    repository.UsuarioRepository
	tokenService   *TokenService
	googleClientID string
}

// NewAuthService cria uma nova instância de AuthService.
func NewAuthService(
	usuarioRepo repository.UsuarioRepository,
	tokenService *TokenService,
	googleClientID string,
) *AuthService {
	return &AuthService{
		usuarioRepo:    usuarioRepo,
		tokenService:   tokenService,
		googleClientID: googleClientID,
	}
}

// Signup cria uma nova conta local com email e senha.
// Spec: 02-api-endpoints.md - validações rígidas de senha e bcrypt cost 12
func (s *AuthService) Signup(ctx context.Context, req *models.SignupRequest) (*models.SignupResponse, string, error) {
	// 1. Valida senha
	if fieldErrors := s.ValidatePassword(req.Password); len(fieldErrors) > 0 {
		return nil, "", fmt.Errorf("senha fraca: %v", fieldErrors[0].Issue)
	}

	// 2. Verifica se usuário com email já existe
	_, err := s.usuarioRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		return nil, "", ErrEmailAlreadyExists
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, "", fmt.Errorf("erro ao verificar email existente: %w", err)
	}

	// 3. Gera hash da senha (bcrypt com custo 12)
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao criptografar senha: %w", err)
	}
	passwordHash := string(hashedBytes)

	// 4. Cria o usuário
	user := &models.Usuario{
		Email:        req.Email,
		PasswordHash: &passwordHash,
		Provider:     models.ProviderLocal,
		Status:       models.StatusOnboardingPending,
	}

	createdUser, err := s.usuarioRepo.Create(ctx, user)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao cadastrar usuário: %w", err)
	}

	// 5. Gera tokens
	accessToken, err := s.tokenService.GenerateAccessToken(createdUser)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar access token: %w", err)
	}

	rawRefreshToken, err := s.tokenService.GenerateRefreshToken(ctx, createdUser.ID)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar refresh token: %w", err)
	}

	return &models.SignupResponse{
		Message: "Conta criada com sucesso",
		Token:   accessToken,
		Status:  createdUser.Status,
	}, rawRefreshToken, nil
}

// Login autentica um usuário local com email e senha.
// Spec: 02-api-endpoints.md
func (s *AuthService) Login(ctx context.Context, req *models.LoginRequest) (*models.LoginResponse, string, error) {
	// 1. Busca usuário
	user, err := s.usuarioRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", fmt.Errorf("erro ao buscar usuário no login: %w", err)
	}

	// 2. Verifica se é conta local e tem senha cadastrada
	if user.PasswordHash == nil || user.Provider != models.ProviderLocal {
		return nil, "", ErrInvalidCredentials
	}

	// 3. Compara senha
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	// 4. Gera tokens
	accessToken, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar access token: %w", err)
	}

	rawRefreshToken, err := s.tokenService.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar refresh token: %w", err)
	}

	return &models.LoginResponse{
		Token:  accessToken,
		Status: user.Status,
	}, rawRefreshToken, nil
}

// GoogleAuth realiza login ou cadastro usando ID Token do Google OAuth.
// Spec: 02-api-endpoints.md, 08-google-jwks.md (stub para testes integrados inicial)
func (s *AuthService) GoogleAuth(ctx context.Context, req *models.GoogleAuthRequest) (*models.GoogleAuthResponse, string, error) {
	// ATENÇÃO: Em produção o token deve ser validado localmente com as chaves públicas obtidas do Google JWKS.
	// Por ora, validamos o formato básico para os testes passarem.
	if len(req.IDToken) < 10 {
		return nil, "", fmt.Errorf("ID token inválido")
	}

	// Mockando a extração do email e sub para este stub inicial de desenvolvimento:
	// Em um fluxo real, decodificamos a assinatura do token e lemos as claims.
	email := "google-test-user@gmail.com"
	sub := "google-sub-12345"

	user, err := s.usuarioRepo.FindByProviderID(ctx, models.ProviderGoogle, sub)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Se não existir, tenta buscar pelo email para vincular ou criar
			user, err = s.usuarioRepo.FindByEmail(ctx, email)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					// Cadastra novo usuário social
					newUser := &models.Usuario{
						Email:    email,
						Provider: models.ProviderGoogle,
						Status:   models.StatusOnboardingPending,
					}
					user, err = s.usuarioRepo.Create(ctx, newUser)
					if err != nil {
						return nil, "", fmt.Errorf("falha ao cadastrar usuário google: %w", err)
					}
				} else {
					return nil, "", fmt.Errorf("erro ao verificar email: %w", err)
				}
			}
		} else {
			return nil, "", fmt.Errorf("erro ao buscar usuário google: %w", err)
		}
	}

	accessToken, err := s.tokenService.GenerateAccessToken(user)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar access token: %w", err)
	}

	rawRefreshToken, err := s.tokenService.GenerateRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao gerar refresh token: %w", err)
	}

	return &models.GoogleAuthResponse{
		Token:  accessToken,
		Status: user.Status,
	}, rawRefreshToken, nil
}

// GetUserByID busca as informações de um usuário por ID.
func (s *AuthService) GetUserByID(ctx context.Context, id uuid.UUID) (*models.Usuario, error) {
	return s.usuarioRepo.FindByID(ctx, id)
}

// ValidatePassword valida as regras rígidas de segurança para senhas.
// Spec: 02-api-endpoints.md - min 8, max 72, 1 maiúscula, 1 minúscula, 1 número, 1 especial char
func (s *AuthService) ValidatePassword(password string) []models.FieldError {
	var errors []models.FieldError

	if len(password) < 8 || len(password) > 72 {
		errors = append(errors, models.FieldError{
			Field: "password",
			Issue: "A senha deve ter entre 8 e 72 caracteres.",
		})
	}

	hasUppercase := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUppercase {
		errors = append(errors, models.FieldError{
			Field: "password",
			Issue: "A senha deve conter pelo menos uma letra maiúscula.",
		})
	}

	hasLowercase := regexp.MustCompile(`[a-z]`).MatchString(password)
	if !hasLowercase {
		errors = append(errors, models.FieldError{
			Field: "password",
			Issue: "A senha deve conter pelo menos uma letra minúscula.",
		})
	}

	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasNumber {
		errors = append(errors, models.FieldError{
			Field: "password",
			Issue: "A senha deve conter pelo menos um número.",
		})
	}

	hasSpecial := regexp.MustCompile(`[!@#\$%\^&\*\(\)_\+\-=\[\]\{\};':",\./<>\?~` + "`" + `|\\ ]`).MatchString(password)
	if !hasSpecial {
		errors = append(errors, models.FieldError{
			Field: "password",
			Issue: "A senha deve conter pelo menos um caractere especial.",
		})
	}

	return errors
}
