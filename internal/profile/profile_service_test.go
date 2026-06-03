package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"snooker/internal/models"
	"snooker/internal/repository"
	"snooker/internal/storage"
)

func TestProfileService_GetProfile_Success(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	service := NewProfileService(perfilRepo, userRepo, nil)

	userID := uuid.New()
	expectedProfile := &models.Perfil{
		UserID:      userID,
		DisplayName: "Test Player",
		PhotoURL:    "http://storage.snooker/avatar.png",
		Bio:         "Gamer bio",
	}

	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(expectedProfile, nil)

	resp, err := service.GetProfile(context.Background(), userID)
	assert.NoError(t, err)
	assert.Equal(t, expectedProfile.DisplayName, resp.DisplayName)
	assert.Equal(t, expectedProfile.PhotoURL, resp.PhotoURL)
	assert.Equal(t, expectedProfile.Bio, resp.Bio)

	perfilRepo.AssertExpectations(t)
}

func TestProfileService_GetProfile_NotFound(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	service := NewProfileService(perfilRepo, userRepo, nil)

	userID := uuid.New()
	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)

	_, err := service.GetProfile(context.Background(), userID)
	assert.ErrorIs(t, err, ErrProfileNotFound)

	perfilRepo.AssertExpectations(t)
}

func TestProfileService_GetProfile_RepositoryError(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	service := NewProfileService(perfilRepo, userRepo, nil)

	userID := uuid.New()
	expectedErr := errors.New("database connection lost")
	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, expectedErr)

	_, err := service.GetProfile(context.Background(), userID)
	assert.ErrorContains(t, err, "falha ao buscar perfil")

	perfilRepo.AssertExpectations(t)
}

func TestProfileService_CompleteProfile_AlreadyExists(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	service := NewProfileService(perfilRepo, userRepo, nil)

	userID := uuid.New()
	existingProfile := &models.Perfil{UserID: userID}

	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(existingProfile, nil)

	req := &models.CompleteProfileRequest{
		DisplayName: "New Player",
	}

	err := service.CompleteProfile(context.Background(), userID, req)
	assert.ErrorIs(t, err, ErrProfileAlreadyExists)

	perfilRepo.AssertExpectations(t)
}

func TestProfileService_GetUploadURL_Success(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	expectedURL := "http://storage.snooker/temp/avatar-123.png"
	expectedKey := "temp/avatar-123.png"

	mockStorage.On("GeneratePresignedUploadURL", mock.Anything).Return(expectedURL, expectedKey, nil)

	resp, err := service.GetUploadURL(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedURL, resp.UploadURL)
	assert.Equal(t, expectedKey, resp.ObjectKey)

	mockStorage.AssertExpectations(t)
}

func TestProfileService_GetUploadURL_Error(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	expectedErr := errors.New("failed to connect to minio")
	mockStorage.On("GeneratePresignedUploadURL", mock.Anything).Return("", "", expectedErr)

	_, err := service.GetUploadURL(context.Background())
	assert.ErrorContains(t, err, "falha ao gerar URL de upload")

	mockStorage.AssertExpectations(t)
}

func TestProfileService_CompleteProfile_Success(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	userID := uuid.New()
	req := &models.CompleteProfileRequest{
		DisplayName: "NewPlayerName",
		Bio:         "Short bio description",
		PhotoKey:    "temp/avatar-123.png",
	}

	// 1. Perfil não deve existir
	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)

	// 2. Consolidação de imagem
	activeKey := "active/user-123.png"
	publicURL := "http://storage.snooker/active/user-123.png"
	mockStorage.On("ConsolidateAvatar", mock.Anything, req.PhotoKey, userID).Return(activeKey, nil)
	mockStorage.On("GetPublicURL", activeKey).Return(publicURL)

	// 3. Criação do perfil no DB
	perfilRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Perfil")).Return(func(ctx context.Context, p *models.Perfil) *models.Perfil {
		assert.Equal(t, userID, p.UserID)
		assert.Equal(t, req.DisplayName, p.DisplayName)
		assert.Equal(t, req.Bio, p.Bio)
		assert.Equal(t, publicURL, p.PhotoURL)
		return p
	}, nil)

	// 4. Update de status do usuário para Active
	userRepo.On("UpdateStatus", mock.Anything, userID, models.StatusActive).Return(nil)

	err := service.CompleteProfile(context.Background(), userID, req)
	assert.NoError(t, err)

	perfilRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

func TestProfileService_CompleteProfile_ConsolidateAvatarError(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	userID := uuid.New()
	req := &models.CompleteProfileRequest{
		DisplayName: "PlayerName",
		PhotoKey:    "temp/avatar-123.png",
	}

	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)
	mockStorage.On("ConsolidateAvatar", mock.Anything, req.PhotoKey, userID).Return("", errors.New("s3 connection timeout"))

	err := service.CompleteProfile(context.Background(), userID, req)
	assert.ErrorContains(t, err, "falha ao consolidar avatar")

	perfilRepo.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

func TestProfileService_CompleteProfile_CreateProfileError(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	userID := uuid.New()
	req := &models.CompleteProfileRequest{
		DisplayName: "PlayerName",
		PhotoKey:    "temp/avatar-123.png",
	}

	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)
	
	activeKey := "active/user-123.png"
	publicURL := "http://storage.snooker/active/user-123.png"
	mockStorage.On("ConsolidateAvatar", mock.Anything, req.PhotoKey, userID).Return(activeKey, nil)
	mockStorage.On("GetPublicURL", activeKey).Return(publicURL)

	perfilRepo.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	err := service.CompleteProfile(context.Background(), userID, req)
	assert.ErrorContains(t, err, "falha ao criar perfil")

	perfilRepo.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

func TestProfileService_CompleteProfile_UpdateStatusError(t *testing.T) {
	perfilRepo := new(repository.MockPerfilRepository)
	userRepo := new(repository.MockUsuarioRepository)
	mockStorage := new(storage.MockStorage)
	service := NewProfileService(perfilRepo, userRepo, mockStorage)

	userID := uuid.New()
	req := &models.CompleteProfileRequest{
		DisplayName: "PlayerName",
	}

	perfilRepo.On("FindByUserID", mock.Anything, userID).Return(nil, repository.ErrNotFound)
	perfilRepo.On("Create", mock.Anything, mock.Anything).Return(&models.Perfil{}, nil)
	userRepo.On("UpdateStatus", mock.Anything, userID, models.StatusActive).Return(errors.New("failed to update row"))

	err := service.CompleteProfile(context.Background(), userID, req)
	assert.ErrorContains(t, err, "falha ao ativar usuário")

	perfilRepo.AssertExpectations(t)
	userRepo.AssertExpectations(t)
}
