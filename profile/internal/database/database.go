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
		migrationCreateProfiles,
		migrationCreatePhotoUploadSessions,
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

const migrationCreateProfiles = `
CREATE TABLE IF NOT EXISTS profiles (
    user_id             UUID PRIMARY KEY,
    nickname            VARCHAR(24) NOT NULL,
    nickname_normalized VARCHAR(24) NOT NULL,
    bio                 VARCHAR(200) NOT NULL DEFAULT '',
    photo_object_key    TEXT NOT NULL,
    photo_url           TEXT NOT NULL,
    xp                  INTEGER NOT NULL DEFAULT 0 CHECK (xp >= 0),
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCreatePhotoUploadSessions = `
CREATE TABLE IF NOT EXISTS photo_upload_sessions (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID NOT NULL,
    object_key     TEXT NOT NULL,
    content_type   VARCHAR(64) NOT NULL,
    max_size_bytes INTEGER NOT NULL,
    expires_at     TIMESTAMP WITH TIME ZONE NOT NULL,
    consumed_at    TIMESTAMP WITH TIME ZONE NULL,
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCreateIndexes = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_nickname_normalized ON profiles(nickname_normalized);
CREATE INDEX IF NOT EXISTS idx_profiles_xp ON profiles(xp DESC);
CREATE INDEX IF NOT EXISTS idx_photo_upload_sessions_user ON photo_upload_sessions(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_photo_upload_sessions_expiry ON photo_upload_sessions(expires_at);
`
