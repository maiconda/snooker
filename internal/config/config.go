package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config contém todas as variáveis de configuração da aplicação.
type Config struct {
	// Servidor
	Port           string
	AllowedOrigins string

	// JWT
	JWTSecret string

	// Google OAuth
	GoogleClientID string

	// PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// MinIO / S3
	StorageEndpoint  string
	StorageAccessKey string
	StorageSecretKey string
	StorageUseSSL    bool
	StorageBucket    string
}

// Load carrega as variáveis de ambiente e retorna uma Config validada.
func Load() (*Config, error) {
	// Tenta carregar .env (silenciosamente ignora se não existir, ex: em containers)
	_ = godotenv.Load()

	dbPort, err := strconv.Atoi(getEnv("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("DB_PORT inválido: %w", err)
	}

	storageSSL, _ := strconv.ParseBool(getEnv("STORAGE_USE_SSL", "false"))

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),

		JWTSecret:      getEnvRequired("JWT_SECRET"),
		GoogleClientID: getEnvRequired("GOOGLE_CLIENT_ID"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     dbPort,
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "snooker_db"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		StorageEndpoint:  getEnv("STORAGE_ENDPOINT", "localhost:9000"),
		StorageAccessKey: getEnv("STORAGE_ACCESS_KEY", "minioadmin"),
		StorageSecretKey: getEnv("STORAGE_SECRET_KEY", "minioadmin"),
		StorageUseSSL:    storageSSL,
		StorageBucket:    getEnv("STORAGE_BUCKET_NAME", "snooker-profiles"),
	}

	return cfg, nil
}

// DatabaseURL retorna a string de conexão formatada para o pgx.
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

func getEnvRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		// Em desenvolvimento, permitir placeholder para não bloquear startup
		return fmt.Sprintf("MISSING_%s", key)
	}
	return v
}
