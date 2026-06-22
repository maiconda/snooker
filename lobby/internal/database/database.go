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
		migrationCreateRooms,
		migrationAddRoomPresenceColumns,
		migrationCreateRoomMatchStates,
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
    CREATE TYPE room_status AS ENUM ('waiting', 'playing', 'finished', 'expired');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;
`

const migrationCreateRooms = `
CREATE TABLE IF NOT EXISTS rooms (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code          VARCHAR(6) UNIQUE NULL,
    creator_id    UUID NOT NULL,
    opponent_id   UUID NULL,
    status        room_status NOT NULL DEFAULT 'waiting',
    is_private    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    TIMESTAMP WITH TIME ZONE NOT NULL,
    creator_disconnected_at TIMESTAMP WITH TIME ZONE NULL,
    opponent_disconnected_at TIMESTAMP WITH TIME ZONE NULL,
    creator_connection_id TEXT NULL,
    opponent_connection_id TEXT NULL
);
`

const migrationAddRoomPresenceColumns = `
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS creator_disconnected_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS opponent_disconnected_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS creator_connection_id TEXT NULL;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS opponent_connection_id TEXT NULL;
`

const migrationCreateRoomMatchStates = `
CREATE TABLE IF NOT EXISTS room_match_states (
    room_id    UUID PRIMARY KEY REFERENCES rooms(id) ON DELETE CASCADE,
    snapshot   JSONB NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCreateIndexes = `
CREATE INDEX IF NOT EXISTS idx_rooms_code ON rooms(code) WHERE code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_rooms_status ON rooms(status);
CREATE INDEX IF NOT EXISTS idx_rooms_creator ON rooms(creator_id);
CREATE INDEX IF NOT EXISTS idx_rooms_opponent ON rooms(opponent_id) WHERE opponent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_rooms_owner_disconnect ON rooms(creator_disconnected_at) WHERE creator_disconnected_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_rooms_opponent_disconnect ON rooms(opponent_disconnected_at) WHERE opponent_disconnected_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_room_match_states_updated ON room_match_states(updated_at);
`
