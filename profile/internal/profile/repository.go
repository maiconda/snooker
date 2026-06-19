package profile

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("registro nao encontrado")
	ErrDuplicateNickname = errors.New("nickname ja existe")
	ErrInvalidUpload     = errors.New("upload invalido")
)

type Repository interface {
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error)
	Upsert(ctx context.Context, p *Profile) (*Profile, error)
	Update(ctx context.Context, p *Profile) (*Profile, error)

	CreateUploadSession(ctx context.Context, session *PhotoUploadSession) (*PhotoUploadSession, error)
	FindUploadSession(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*PhotoUploadSession, error)
	MarkUploadConsumed(ctx context.Context, id uuid.UUID) error
}

type Storage interface {
	PublicURL(objectKey string) string
	PresignPutObject(objectKey string, contentType string, expiresInSeconds int64) (string, error)
	HeadObject(ctx context.Context, objectKey string) (*ObjectInfo, error)
}

type ObjectInfo struct {
	ContentType   string
	ContentLength int64
}

type AuthActivator interface {
	ActivateUser(ctx context.Context, userID uuid.UUID) (accessToken string, status string, err error)
}
