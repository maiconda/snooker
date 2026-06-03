package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect cria e retorna um pool de conexões com o PostgreSQL.
// As configurações do pool são otimizadas para microserviços em containers.
// Spec: security-and-k8s-audit.md - MaxConns = 10-15 per Go pod.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear database URL: %w", err)
	}

	// Configurações de pool otimizadas para containers/K8s
	// Fórmula: MaxConns_per_pod × Max_pods_HPA < PostgreSQL_max_connections
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar pool de conexões: %w", err)
	}

	// Verifica a conectividade
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("falha ao conectar ao banco de dados: %w", err)
	}

	return pool, nil
}

// RunMigrations executa as migrations SQL na ordem correta.
// Spec: 01-database-and-models.md - DDL exato.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations := []string{
		migrationCreateExtensions,
		migrationCreateTypes,
		migrationCreateUsuarios,
		migrationCreatePerfis,
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

// ==================== MIGRATIONS SQL ====================
// DDL conforme spec 01-database-and-models.md

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
    status        user_status NOT NULL DEFAULT 'onboarding_pending',
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCreatePerfis = `
CREATE TABLE IF NOT EXISTS perfis (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID UNIQUE NOT NULL,
    display_name VARCHAR(50) NOT NULL,
    bio          VARCHAR(200) NOT NULL DEFAULT '',
    photo_url    VARCHAR(512) NOT NULL DEFAULT '',
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES usuarios(id) ON DELETE CASCADE
);
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
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
`
