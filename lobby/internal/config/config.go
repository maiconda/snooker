package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	AllowedOrigins string

	JWTSecret      string
	InternalAPIKey string

	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	NATSURL    string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	dbPort, err := strconv.Atoi(getEnv("DB_PORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("DB_PORT invalido: %w", err)
	}

	jwtSecret, err := requireEnv("JWT_SECRET")
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:           getEnv("PORT", "8080"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000"),

		JWTSecret:      jwtSecret,
		InternalAPIKey: getEnv("INTERNAL_API_KEY", ""),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     dbPort,
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "snooker_lobby"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		NATSURL:    getEnv("NATS_URL", "nats://localhost:4222"),
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
