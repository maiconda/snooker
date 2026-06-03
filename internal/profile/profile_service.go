package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"snooker/internal/models"
	"snooker/internal/repository"
	"snooker/internal/storage"
)

var (
	ErrProfileAlreadyExists = errors.New("profile already exists")
	ErrProfileNotFound      = errors.New("profile not found")
	ErrInvalidPhotoKey      = errors.New("invalid photo key format")
)

// ProfileService gerencia operações de perfil e onboarding.
// Spec: 02-api-endpoints.md, 04-storage-integration.md
type ProfileService struct {
	perfilRepo  repository.PerfilRepository
	usuarioRepo repository.UsuarioRepository
	storage     storage.Storage
}

// NewProfileService cria uma nova instância de ProfileService.
func NewProfileService(
	perfilRepo repository.PerfilRepository,
	usuarioRepo repository.UsuarioRepository,
	storage storage.Storage,
) *ProfileService {
	return &ProfileService{
		perfilRepo:  perfilRepo,
		usuarioRepo: usuarioRepo,
		storage:     storage,
	}
}

// GetUploadURL gera uma URL pré-assinada para upload de avatar.
// Spec: GET /api/v1/profile/upload-url - retorna upload_url e object_key.
func (s *ProfileService) GetUploadURL(ctx context.Context) (*models.UploadURLResponse, error) {
	uploadURL, objectKey, err := s.storage.GeneratePresignedUploadURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao gerar URL de upload: %w", err)
	}

	return &models.UploadURLResponse{
		UploadURL: uploadURL,
		ObjectKey: objectKey,
	}, nil
}

// CompleteProfile finaliza o onboarding do usuário: cria perfil, consolida avatar, muda status para active.
// Spec: POST /api/v1/profile/complete
func (s *ProfileService) CompleteProfile(ctx context.Context, userID uuid.UUID, req *models.CompleteProfileRequest) error {
	// Verifica se perfil já existe
	_, err := s.perfilRepo.FindByUserID(ctx, userID)
	if err == nil {
		return ErrProfileAlreadyExists
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("falha ao verificar perfil existente: %w", err)
	}

	// Consolida avatar se photo_key foi fornecido
	photoURL := ""
	if req.PhotoKey != "" {
		activeKey, err := s.storage.ConsolidateAvatar(ctx, req.PhotoKey, userID)
		if err != nil {
			return fmt.Errorf("falha ao consolidar avatar: %w", err)
		}
		photoURL = s.storage.GetPublicURL(activeKey)
	}

	// Cria perfil
	perfil := &models.Perfil{
		UserID:      userID,
		DisplayName: req.DisplayName,
		Bio:         req.Bio,
		PhotoURL:    photoURL,
	}

	_, err = s.perfilRepo.Create(ctx, perfil)
	if err != nil {
		return fmt.Errorf("falha ao criar perfil: %w", err)
	}

	// Atualiza status do usuário para active
	if err := s.usuarioRepo.UpdateStatus(ctx, userID, models.StatusActive); err != nil {
		return fmt.Errorf("falha ao ativar usuário: %w", err)
	}

	return nil
}

// GetProfile retorna o perfil de um usuário.
func (s *ProfileService) GetProfile(ctx context.Context, userID uuid.UUID) (*models.ProfileResponse, error) {
	perfil, err := s.perfilRepo.FindByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("falha ao buscar perfil: %w", err)
	}

	return &models.ProfileResponse{
		DisplayName: perfil.DisplayName,
		PhotoURL:    perfil.PhotoURL,
		Bio:         perfil.Bio,
	}, nil
}
