package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Configura apenas os required
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing")
	os.Setenv("GOOGLE_CLIENT_ID", "test-google-client-id")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("GOOGLE_CLIENT_ID")
	}()

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "localhost", cfg.DBHost)
	assert.Equal(t, 5432, cfg.DBPort)
	assert.Equal(t, "postgres", cfg.DBUser)
	assert.Equal(t, "snooker_db", cfg.DBName)
	assert.Equal(t, "disable", cfg.DBSSLMode)
	assert.Equal(t, "localhost:9000", cfg.StorageEndpoint)
	assert.Equal(t, "snooker-profiles", cfg.StorageBucket)
	assert.False(t, cfg.StorageUseSSL)
}

func TestLoad_CustomValues(t *testing.T) {
	envs := map[string]string{
		"JWT_SECRET":        "my-secret",
		"GOOGLE_CLIENT_ID":  "my-google-id",
		"PORT":              "3000",
		"DB_HOST":           "db.example.com",
		"DB_PORT":           "5433",
		"DB_USER":           "myuser",
		"DB_PASSWORD":       "mypass",
		"DB_NAME":           "mydb",
		"DB_SSLMODE":        "require",
		"STORAGE_ENDPOINT":  "s3.example.com",
		"STORAGE_ACCESS_KEY": "access123",
		"STORAGE_SECRET_KEY": "secret456",
		"STORAGE_USE_SSL":   "true",
		"STORAGE_BUCKET_NAME": "my-bucket",
	}
	for k, v := range envs {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range envs {
			os.Unsetenv(k)
		}
	}()

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "3000", cfg.Port)
	assert.Equal(t, "my-secret", cfg.JWTSecret)
	assert.Equal(t, "my-google-id", cfg.GoogleClientID)
	assert.Equal(t, "db.example.com", cfg.DBHost)
	assert.Equal(t, 5433, cfg.DBPort)
	assert.Equal(t, "myuser", cfg.DBUser)
	assert.Equal(t, "mypass", cfg.DBPassword)
	assert.Equal(t, "mydb", cfg.DBName)
	assert.Equal(t, "require", cfg.DBSSLMode)
	assert.Equal(t, "s3.example.com", cfg.StorageEndpoint)
	assert.Equal(t, "access123", cfg.StorageAccessKey)
	assert.Equal(t, "secret456", cfg.StorageSecretKey)
	assert.True(t, cfg.StorageUseSSL)
	assert.Equal(t, "my-bucket", cfg.StorageBucket)
}

func TestLoad_InvalidDBPort(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	os.Setenv("GOOGLE_CLIENT_ID", "test-id")
	os.Setenv("DB_PORT", "not-a-number")
	defer func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("GOOGLE_CLIENT_ID")
		os.Unsetenv("DB_PORT")
	}()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PORT inválido")
}

func TestDatabaseURL(t *testing.T) {
	cfg := &Config{
		DBUser:     "myuser",
		DBPassword: "mypass",
		DBHost:     "localhost",
		DBPort:     5432,
		DBName:     "testdb",
		DBSSLMode:  "disable",
	}

	expected := "postgres://myuser:mypass@localhost:5432/testdb?sslmode=disable"
	assert.Equal(t, expected, cfg.DatabaseURL())
}

func TestGetEnvRequired_Missing(t *testing.T) {
	os.Unsetenv("SOME_MISSING_VAR")
	result := getEnvRequired("SOME_MISSING_VAR")
	assert.Equal(t, "MISSING_SOME_MISSING_VAR", result)
}
