package storage

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Storage define a interface para operações de armazenamento de objetos.
type Storage interface {
	EnsureBucket(ctx context.Context) error
	GeneratePresignedUploadURL(ctx context.Context) (uploadURL string, objectKey string, err error)
	ConsolidateAvatar(ctx context.Context, tempKey string, userID uuid.UUID) (string, error)
	GetPublicURL(objectKey string) string
}

// StorageService gerencia operações de armazenamento de objetos (MinIO/S3).
// Spec: 04-storage-integration.md
type StorageService struct {
	client     *minio.Client
	bucketName string
}

// NewStorageService cria uma nova instância do StorageService.
func NewStorageService(endpoint, accessKey, secretKey string, useSSL bool, bucketName string) (*StorageService, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("falha ao criar cliente MinIO: %w", err)
	}

	return &StorageService{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// EnsureBucket garante que o bucket existe, criando se necessário.
func (s *StorageService) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucketName)
	if err != nil {
		return fmt.Errorf("falha ao verificar bucket: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucketName, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("falha ao criar bucket: %w", err)
		}
	}
	return nil
}

// GeneratePresignedUploadURL gera uma URL pré-assinada para upload de avatar.
// Spec: Gera URL com PUT para prefix temp/<uuid>.png, válida por 5 minutos.
func (s *StorageService) GeneratePresignedUploadURL(ctx context.Context) (uploadURL string, objectKey string, err error) {
	fileID := uuid.New().String()
	objectKey = fmt.Sprintf("temp/%s.png", fileID)

	reqParams := make(url.Values)
	reqParams.Set("Content-Type", "image/png")

	presignedURL, err := s.client.PresignedPutObject(ctx, s.bucketName, objectKey, 5*time.Minute)
	if err != nil {
		return "", "", fmt.Errorf("falha ao gerar presigned URL: %w", err)
	}

	return presignedURL.String(), objectKey, nil
}

// ConsolidateAvatar copia o arquivo de temp/ para active/<userID>.png e remove o temporário.
// Spec: 04-storage-integration.md - CopyObject de temp/ para active/
func (s *StorageService) ConsolidateAvatar(ctx context.Context, tempKey string, userID uuid.UUID) (string, error) {
	activeKey := fmt.Sprintf("active/%s.png", userID.String())

	// CopyObject de temp para active
	src := minio.CopySrcOptions{
		Bucket: s.bucketName,
		Object: tempKey,
	}
	dst := minio.CopyDestOptions{
		Bucket: s.bucketName,
		Object: activeKey,
	}

	_, err := s.client.CopyObject(ctx, dst, src)
	if err != nil {
		return "", fmt.Errorf("falha ao copiar avatar de temp para active: %w", err)
	}

	// Remove arquivo temporário
	if err := s.client.RemoveObject(ctx, s.bucketName, tempKey, minio.RemoveObjectOptions{}); err != nil {
		// Log o erro mas não falha a operação - lifecycle rule limpará
		fmt.Printf("WARN: falha ao remover temp file %s: %v\n", tempKey, err)
	}

	return activeKey, nil
}

// GetPublicURL retorna a URL pública de um objeto.
func (s *StorageService) GetPublicURL(objectKey string) string {
	scheme := "http"
	endpoint := s.client.EndpointURL().Host
	if s.client.EndpointURL().Scheme == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, endpoint, s.bucketName, objectKey)
}
