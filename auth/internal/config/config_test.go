package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
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
	assert.Equal(t, "snooker_auth", cfg.DBName)
	assert.Equal(t, "disable", cfg.DBSSLMode)
	assert.False(t, cfg.CookieSecure)
}

func TestLoad_CustomValues(t *testing.T) {
	envs := map[string]string{
		"JWT_SECRET":       "my-secret",
		"GOOGLE_CLIENT_ID": "my-google-id",
		"PORT":             "3000",
		"DB_HOST":          "db.example.com",
		"DB_PORT":          "5433",
		"DB_USER":          "myuser",
		"DB_PASSWORD":      "mypass",
		"DB_NAME":          "mydb",
		"DB_SSLMODE":       "require",
		"COOKIE_SECURE":    "true",
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
	assert.True(t, cfg.CookieSecure)
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
	assert.Contains(t, err.Error(), "DB_PORT invalido")
}

func TestLoad_MissingRequiredSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	os.Setenv("GOOGLE_CLIENT_ID", "test-id")
	defer os.Unsetenv("GOOGLE_CLIENT_ID")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET is required")
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
