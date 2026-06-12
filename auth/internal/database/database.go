package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear database URL: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar pool de conexoes: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("falha ao conectar ao banco de dados: %w", err)
	}

	return pool, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations := []string{
		migrationCreateExtensions,
		migrationCreateTypes,
		migrationCreateUsuarios,
		migrationUpdateUsuariosProviderID,
		migrationCreateRefreshTokens,
		migrationCreateIndexes,
	}

	for i, migration := range migrations {
		if _, err := pool.Exec(ctx, migration); err != nil {
			return fmt.Errorf("migration %d falhou: %w", i+1, err)
		}
	}

	return nil
}

const migrationCreateExtensions = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
`

const migrationCreateTypes = `
DO $$ BEGIN
    CREATE TYPE user_status AS ENUM ('onboarding_pending', 'active', 'blocked');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE auth_provider AS ENUM ('local', 'google');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;
`

const migrationCreateUsuarios = `
CREATE TABLE IF NOT EXISTS usuarios (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NULL,
    provider      auth_provider NOT NULL DEFAULT 'local',
    provider_id   VARCHAR(255) NULL,
    status        user_status NOT NULL DEFAULT 'onboarding_pending',
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationUpdateUsuariosProviderID = `
ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS provider_id VARCHAR(255) NULL;
`

const migrationCreateRefreshTokens = `
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    family_id  UUID NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMP WITH TIME ZONE NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_refresh_user FOREIGN KEY (user_id) REFERENCES usuarios(id) ON DELETE CASCADE
);
`

const migrationCreateIndexes = `
CREATE INDEX IF NOT EXISTS idx_usuarios_email ON usuarios(email);
CREATE INDEX IF NOT EXISTS idx_usuarios_status ON usuarios(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_usuarios_provider_id ON usuarios(provider, provider_id) WHERE provider_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
`
