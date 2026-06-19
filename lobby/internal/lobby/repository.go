package lobby

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRoomNotFound      = errors.New("sala nao encontrada")
	ErrRoomFull          = errors.New("sala ja esta cheia")
	ErrRoomExpired       = errors.New("sala expirada")
	ErrRoomAlreadyJoined = errors.New("voce ja esta nesta sala")
)

type Repository interface {
	CreateRoom(ctx context.Context, creatorID string, isPrivate bool) (*Room, error)
	GetRoomByID(ctx context.Context, id string) (*Room, error)
	GetRoomByCode(ctx context.Context, code string) (*Room, error)
	ListPublicRooms(ctx context.Context) ([]*Room, error)
	JoinRoom(ctx context.Context, roomID string, opponentID string) (*Room, error)
	ExpireRooms(ctx context.Context) (int64, error)
	CloseRoom(ctx context.Context, roomID string) error
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) CreateRoom(ctx context.Context, creatorID string, isPrivate bool) (*Room, error) {
	code, err := r.generateUniqueCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao gerar codigo unico: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)

	query := `
		INSERT INTO rooms (code, creator_id, opponent_id, status, is_private, created_at, expires_at)
		VALUES ($1, $2, NULL, 'waiting', $3, $4, $5)
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
	`

	var room Room
	err = r.pool.QueryRow(ctx, query, code, creatorID, isPrivate, now, expiresAt).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao inserir sala: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) GetRoomByID(ctx context.Context, id string) (*Room, error) {
	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
		FROM rooms
		WHERE id = $1
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala por ID: %w", err)
	}

	// Se estiver expirada no tempo mas ainda nao marcada no banco, marcar como expirada
	if room.Status == StatusWaiting && time.Now().After(room.ExpiresAt) {
		_ = r.updateStatus(ctx, room.ID, StatusExpired)
		room.Status = StatusExpired
	}

	return &room, nil
}

func (r *postgresRepository) GetRoomByCode(ctx context.Context, code string) (*Room, error) {
	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
		FROM rooms
		WHERE code = $1
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, code).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala por codigo: %w", err)
	}

	// Verificar expiracao
	if room.Status == StatusWaiting && time.Now().After(room.ExpiresAt) {
		_ = r.updateStatus(ctx, room.ID, StatusExpired)
		room.Status = StatusExpired
	}

	return &room, nil
}

func (r *postgresRepository) ListPublicRooms(ctx context.Context) ([]*Room, error) {
	// Atualizar expiradas antes de listar
	_, _ = r.ExpireRooms(ctx)

	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
		FROM rooms
		WHERE is_private = FALSE AND status = 'waiting'
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("falha ao listar salas publicas: %w", err)
	}
	defer rows.Close()

	rooms := []*Room{}
	for rows.Next() {
		var room Room
		err := rows.Scan(
			&room.ID,
			&room.Code,
			&room.CreatorID,
			&room.OpponentID,
			&room.Status,
			&room.IsPrivate,
			&room.CreatedAt,
			&room.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("falha ao escanear sala: %w", err)
		}
		rooms = append(rooms, &room)
	}

	return rooms, nil
}

func (r *postgresRepository) JoinRoom(ctx context.Context, roomID string, opponentID string) (*Room, error) {
	// Iniciar transacao para garantir atomicidade do join
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar transacao: %w", err)
	}
	defer tx.Rollback(ctx)

	querySelect := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
		FROM rooms
		WHERE id = $1
		FOR UPDATE
	`

	var room Room
	err = tx.QueryRow(ctx, querySelect, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala para juncao: %w", err)
	}

	// Validacoes
	if room.Status == StatusExpired || time.Now().After(room.ExpiresAt) {
		_, _ = tx.Exec(ctx, "UPDATE rooms SET status = 'expired' WHERE id = $1", roomID)
		return nil, ErrRoomExpired
	}

	if room.Status != StatusWaiting {
		return nil, fmt.Errorf("sala nao esta aguardando jogadores (status: %s)", room.Status)
	}

	if room.CreatorID == opponentID {
		return nil, ErrRoomAlreadyJoined
	}

	if room.OpponentID != nil {
		if *room.OpponentID == opponentID {
			return &room, nil // Ja esta na sala
		}
		return nil, ErrRoomFull
	}

	queryUpdate := `
		UPDATE rooms
		SET opponent_id = $1
		WHERE id = $2
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at
	`

	err = tx.QueryRow(ctx, queryUpdate, opponentID, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao atualizar oponente na sala: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao commitar transacao: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) ExpireRooms(ctx context.Context) (int64, error) {
	query := `
		UPDATE rooms
		SET status = 'expired'
		WHERE status = 'waiting' AND expires_at < $1
	`
	cmd, err := r.pool.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("falha ao expirar salas: %w", err)
	}
	return cmd.RowsAffected(), nil
}

func (r *postgresRepository) updateStatus(ctx context.Context, roomID string, status RoomStatus) error {
	query := "UPDATE rooms SET status = $1 WHERE id = $2"
	_, err := r.pool.Exec(ctx, query, status, roomID)
	return err
}

func (r *postgresRepository) CloseRoom(ctx context.Context, roomID string) error {
	query := "UPDATE rooms SET status = 'expired' WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, roomID)
	return err
}

func (r *postgresRepository) generateUniqueCode(ctx context.Context) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for attempt := 0; attempt < 10; attempt++ {
		code := make([]byte, 6)
		for i := range code {
			num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", err
			}
			code[i] = charset[num.Int64()]
		}

		codeStr := string(code)

		// Verificar se ja existe
		var exists bool
		err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM rooms WHERE code = $1)", codeStr).Scan(&exists)
		if err != nil {
			return "", err
		}

		if !exists {
			return codeStr, nil
		}
	}
	return "", errors.New("nao foi possivel gerar um codigo de sala unico apos varias tentativas")
}
