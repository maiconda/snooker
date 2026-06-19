package profile

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	uploadURLTTLSeconds = 300
	maxBioLength        = 200
)

var nicknamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,24}$`)

type Service struct {
	repo          Repository
	storage       Storage
	authActivator AuthActivator
	maxPhotoBytes int64
}

func NewService(repo Repository, storage Storage, authActivator AuthActivator, maxPhotoBytes int64) *Service {
	return &Service{
		repo:          repo,
		storage:       storage,
		authActivator: authActivator,
		maxPhotoBytes: maxPhotoBytes,
	}
}

func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	return s.repo.FindByUserID(ctx, userID)
}

func (s *Service) CreatePhotoUploadURL(ctx context.Context, userID uuid.UUID, req *PhotoUploadURLRequest) (*PhotoUploadURLResponse, error) {
	if !isAllowedContentType(req.ContentType) {
		return nil, fmt.Errorf("%w: tipo de imagem nao suportado", ErrInvalidUpload)
	}
	if req.FileSize <= 0 || req.FileSize > s.maxPhotoBytes {
		return nil, fmt.Errorf("%w: arquivo acima do limite", ErrInvalidUpload)
	}

	uploadID := uuid.New()
	objectKey := fmt.Sprintf("profiles/%s/%s%s", userID.String(), uploadID.String(), extensionForContentType(req.ContentType))
	expiresAt := time.Now().Add(uploadURLTTLSeconds * time.Second)

	session, err := s.repo.CreateUploadSession(ctx, &PhotoUploadSession{
		ID:           uploadID,
		UserID:       userID,
		ObjectKey:    objectKey,
		ContentType:  req.ContentType,
		MaxSizeBytes: s.maxPhotoBytes,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		return nil, err
	}

	uploadURL, err := s.storage.PresignPutObject(objectKey, req.ContentType, uploadURLTTLSeconds)
	if err != nil {
		return nil, fmt.Errorf("falha ao assinar upload: %w", err)
	}

	return &PhotoUploadURLResponse{
		UploadID:  session.ID,
		UploadURL: uploadURL,
		ObjectKey: objectKey,
		ExpiresAt: expiresAt,
		PublicURL: s.storage.PublicURL(objectKey),
		MaxSize:   s.maxPhotoBytes,
	}, nil
}

func (s *Service) CompleteProfile(ctx context.Context, userID uuid.UUID, req *CompleteProfileRequest) (*CompleteProfileResponse, error) {
	nickname, normalized, bio, err := normalizeProfileFields(req.Nickname, req.Bio)
	if err != nil {
		return nil, err
	}
	if req.PhotoUploadID == nil {
		return nil, fmt.Errorf("%w: a foto e obrigatoria", ErrInvalidUpload)
	}

	objectKey, photoURL, err := s.consumePhotoUpload(ctx, userID, *req.PhotoUploadID)
	if err != nil {
		return nil, err
	}

	saved, err := s.repo.Upsert(ctx, &Profile{
		UserID:             userID,
		Nickname:           nickname,
		NicknameNormalized: normalized,
		Bio:                bio,
		PhotoObjectKey:     objectKey,
		PhotoURL:           photoURL,
	})
	if err != nil {
		return nil, err
	}

	accessToken, status, err := s.authActivator.ActivateUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("falha ao ativar conta: %w", err)
	}

	return &CompleteProfileResponse{
		Profile:     saved,
		AccessToken: accessToken,
		Status:      status,
	}, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateProfileRequest) (*Profile, error) {
	current, err := s.repo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	nickname := current.Nickname
	if req.Nickname != nil {
		nickname = *req.Nickname
	}

	bio := current.Bio
	if req.Bio != nil {
		bio = *req.Bio
	}

	normalizedNickname, normalized, normalizedBio, err := normalizeProfileFields(nickname, bio)
	if err != nil {
		return nil, err
	}

	objectKey := current.PhotoObjectKey
	photoURL := current.PhotoURL
	if req.PhotoUploadID != nil {
		objectKey, photoURL, err = s.consumePhotoUpload(ctx, userID, *req.PhotoUploadID)
		if err != nil {
			return nil, err
		}
	}

	current.Nickname = normalizedNickname
	current.NicknameNormalized = normalized
	current.Bio = normalizedBio
	current.PhotoObjectKey = objectKey
	current.PhotoURL = photoURL

	return s.repo.Update(ctx, current)
}

func (s *Service) consumePhotoUpload(ctx context.Context, userID uuid.UUID, uploadID uuid.UUID) (objectKey string, photoURL string, err error) {
	session, err := s.repo.FindUploadSession(ctx, uploadID, userID)
	if err != nil {
		return "", "", err
	}
	if session.ConsumedAt != nil || time.Now().After(session.ExpiresAt) {
		return "", "", ErrInvalidUpload
	}

	info, err := s.storage.HeadObject(ctx, session.ObjectKey)
	if err != nil {
		return "", "", fmt.Errorf("%w: objeto nao encontrado", ErrInvalidUpload)
	}
	if info.ContentLength <= 0 || info.ContentLength > session.MaxSizeBytes {
		return "", "", fmt.Errorf("%w: tamanho invalido", ErrInvalidUpload)
	}
	if info.ContentType != "" && !sameContentType(info.ContentType, session.ContentType) {
		return "", "", fmt.Errorf("%w: tipo invalido", ErrInvalidUpload)
	}

	if err := s.repo.MarkUploadConsumed(ctx, session.ID); err != nil {
		return "", "", err
	}

	return session.ObjectKey, s.storage.PublicURL(session.ObjectKey), nil
}

func normalizeProfileFields(nickname string, bio string) (string, string, string, error) {
	nickname = strings.TrimSpace(nickname)
	bio = strings.TrimSpace(bio)

	if !nicknamePattern.MatchString(nickname) {
		return "", "", "", fmt.Errorf("%w: nickname deve ter 3 a 24 caracteres e usar letras, numeros ou underline", ErrInvalidUpload)
	}
	if len([]rune(bio)) > maxBioLength {
		return "", "", "", fmt.Errorf("%w: bio deve ter no maximo %d caracteres", ErrInvalidUpload, maxBioLength)
	}

	return nickname, strings.ToLower(nickname), bio, nil
}

func isAllowedContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case ContentTypeJPEG, ContentTypePNG, ContentTypeWEBP:
		return true
	default:
		return false
	}
}

func sameContentType(actual string, expected string) bool {
	actual = strings.ToLower(strings.TrimSpace(strings.Split(actual, ";")[0]))
	expected = strings.ToLower(strings.TrimSpace(expected))
	if actual == "image/jpg" {
		actual = ContentTypeJPEG
	}
	return actual == expected
}

func extensionForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case ContentTypeJPEG:
		return ".jpg"
	case ContentTypePNG:
		return ".png"
	default:
		return ".webp"
	}
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrDuplicateNickname)
}
