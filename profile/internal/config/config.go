package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	AllowedOrigins string
	JWTSecret      string
	InternalAPIKey string

	AuthInternalURL string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	StorageEndpoint      string
	StoragePublicBaseURL string
	StorageAccessKey     string
	StorageSecretKey     string
	StorageBucketName    string
	StorageRegion        string
	StorageUseSSL        bool
	MaxPhotoBytes        int64
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	dbPort, err := strconv.Atoi(getEnv("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("DB_PORT invalido: %w", err)
	}

	storageUseSSL, err := strconv.ParseBool(getEnv("STORAGE_USE_SSL", "false"))
	if err != nil {
		return nil, fmt.Errorf("STORAGE_USE_SSL invalido: %w", err)
	}

	maxPhotoBytes, err := strconv.ParseInt(getEnv("PROFILE_MAX_PHOTO_BYTES", "2097152"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("PROFILE_MAX_PHOTO_BYTES invalido: %w", err)
	}

	jwtSecret, err := requireEnv("JWT_SECRET")
	if err != nil {
		return nil, err
	}

	internalAPIKey, err := requireEnv("INTERNAL_API_KEY")
	if err != nil {
		return nil, err
	}

	storageEndpoint, err := requireEnv("STORAGE_ENDPOINT")
	if err != nil {
		return nil, err
	}

	storageAccessKey, err := requireEnv("STORAGE_ACCESS_KEY")
	if err != nil {
		return nil, err
	}

	storageSecretKey, err := requireEnv("STORAGE_SECRET_KEY")
	if err != nil {
		return nil, err
	}

	bucketName, err := requireEnv("STORAGE_BUCKET_NAME")
	if err != nil {
		return nil, err
	}

	publicBaseURL := strings.TrimRight(getEnv("STORAGE_PUBLIC_BASE_URL", ""), "/")
	if publicBaseURL == "" {
		scheme := "http"
		if storageUseSSL {
			scheme = "https"
		}
		publicBaseURL = fmt.Sprintf("%s://%s/%s", scheme, storageEndpoint, bucketName)
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),
		JWTSecret:      jwtSecret,
		InternalAPIKey: internalAPIKey,

		AuthInternalURL: strings.TrimRight(getEnv("AUTH_INTERNAL_URL", "http://localhost:8081"), "/"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     dbPort,
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "snooker_profile"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		StorageEndpoint:      storageEndpoint,
		StoragePublicBaseURL: publicBaseURL,
		StorageAccessKey:     storageAccessKey,
		StorageSecretKey:     storageSecretKey,
		StorageBucketName:    bucketName,
		StorageRegion:        getEnv("STORAGE_REGION", "us-east-1"),
		StorageUseSSL:        storageUseSSL,
		MaxPhotoBytes:        maxPhotoBytes,
	}, nil
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}
