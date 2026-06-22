package lobby

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRoomNotFound      = errors.New("sala nao encontrada")
	ErrRoomFull          = errors.New("sala ja esta cheia")
	ErrRoomExpired       = errors.New("sala expirada")
	ErrRoomAlreadyJoined = errors.New("voce ja esta nesta sala")
	ErrUserInActiveRoom  = errors.New("usuario ja esta em uma partida ativa")
)

type LeaveRole string

const (
	LeaveRoleSpectator LeaveRole = "spectator"
	LeaveRoleCreator   LeaveRole = "creator"
	LeaveRoleOpponent  LeaveRole = "opponent"
)

type ReleasedOpponent struct {
	Room       *Room
	OpponentID string
}

type Repository interface {
	CreateRoom(ctx context.Context, creatorID string, isPrivate bool) (*Room, error)
	UserHasActiveRoom(ctx context.Context, userID string, excludeRoomID string) (bool, error)
	GetRoomByID(ctx context.Context, id string) (*Room, error)
	GetRoomByCode(ctx context.Context, code string) (*Room, error)
	ListPublicRooms(ctx context.Context) ([]*Room, error)
	JoinRoom(ctx context.Context, roomID string, opponentID string) (*Room, error)
	StartRoom(ctx context.Context, roomID string) (*Room, error)
	FinishRoom(ctx context.Context, roomID string) (*Room, error)
	ResetRoom(ctx context.Context, roomID string) (*Room, error)
	StartRematchRoom(ctx context.Context, roomID string) (*Room, error)
	ExpireRooms(ctx context.Context) ([]string, error)
	CloseRoom(ctx context.Context, roomID string) error
	LeaveRoom(ctx context.Context, roomID string, userID string) (*Room, LeaveRole, error)
	MarkCreatorConnected(ctx context.Context, roomID string, creatorID string, connectionID string) (*Room, error)
	MarkCreatorDisconnected(ctx context.Context, roomID string, creatorID string, connectionID string, disconnectedAt time.Time) (bool, error)
	MarkOpponentConnected(ctx context.Context, roomID string, opponentID string, connectionID string) (*Room, error)
	MarkOpponentDisconnected(ctx context.Context, roomID string, opponentID string, connectionID string, disconnectedAt time.Time) (bool, error)
	CloseRoomIfCreatorDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (bool, error)
	CloseRoomsWithExpiredCreatorDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]string, error)
	ReleaseOpponentIfDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (*Room, bool, error)
	ReleaseRoomsWithExpiredOpponentDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]ReleasedOpponent, error)
	GetMatchSnapshot(ctx context.Context, roomID string) (json.RawMessage, error)
	UpsertMatchSnapshot(ctx context.Context, roomID string, snapshot json.RawMessage) error
	DeleteMatchSnapshot(ctx context.Context, roomID string) error
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) userHasActiveRoomTx(ctx context.Context, tx pgx.Tx, userID string, excludeRoomID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM rooms
			WHERE status IN ('waiting', 'playing')
				AND (creator_id = $1 OR opponent_id = $1)
				AND ($2 = '' OR id::text <> $2)
		)
	`
	var exists bool
	if err := tx.QueryRow(ctx, query, userID, excludeRoomID).Scan(&exists); err != nil {
		return false, fmt.Errorf("falha ao verificar partida ativa do usuario: %w", err)
	}
	return exists, nil
}

func (r *postgresRepository) UserHasActiveRoom(ctx context.Context, userID string, excludeRoomID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM rooms
			WHERE status IN ('waiting', 'playing')
				AND (creator_id = $1 OR opponent_id = $1)
				AND ($2 = '' OR id::text <> $2)
		)
	`
	var exists bool
	if err := r.pool.QueryRow(ctx, query, userID, excludeRoomID).Scan(&exists); err != nil {
		return false, fmt.Errorf("falha ao verificar partida ativa do usuario: %w", err)
	}
	return exists, nil
}

func (r *postgresRepository) lockActiveRoomUsersTx(ctx context.Context, tx pgx.Tx, userIDs ...string) error {
	uniqueUserIDs := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		uniqueUserIDs[userID] = struct{}{}
	}

	orderedUserIDs := make([]string, 0, len(uniqueUserIDs))
	for userID := range uniqueUserIDs {
		orderedUserIDs = append(orderedUserIDs, userID)
	}
	sort.Strings(orderedUserIDs)

	for _, userID := range orderedUserIDs {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(753281, hashtext($1))", userID); err != nil {
			return fmt.Errorf("falha ao bloquear partida ativa do usuario: %w", err)
		}
	}

	return nil
}

func (r *postgresRepository) CreateRoom(ctx context.Context, creatorID string, isPrivate bool) (*Room, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar transacao de criacao de sala: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := r.lockActiveRoomUsersTx(ctx, tx, creatorID); err != nil {
		return nil, err
	}

	hasActiveRoom, err := r.userHasActiveRoomTx(ctx, tx, creatorID, "")
	if err != nil {
		return nil, err
	}
	if hasActiveRoom {
		return nil, ErrUserInActiveRoom
	}

	code, err := r.generateUniqueCodeTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("falha ao gerar codigo unico: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)

	query := `
		INSERT INTO rooms (code, creator_id, opponent_id, status, is_private, created_at, expires_at)
		VALUES ($1, $2, NULL, 'waiting', $3, $4, $5)
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err = tx.QueryRow(ctx, query, code, creatorID, isPrivate, now, expiresAt).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao inserir sala: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("falha ao commitar criacao de sala: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) GetRoomByID(ctx context.Context, id string) (*Room, error) {
	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala por ID: %w", err)
	}

	// Se estiver expirada no tempo mas ainda nao marcada no banco, marcar como expirada
	if room.Status == StatusWaiting && room.OpponentID == nil && time.Now().After(room.ExpiresAt) {
		_ = r.updateStatus(ctx, room.ID, StatusExpired)
		room.Status = StatusExpired
	}

	return &room, nil
}

func (r *postgresRepository) GetRoomByCode(ctx context.Context, code string) (*Room, error) {
	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala por codigo: %w", err)
	}

	// Verificar expiracao
	if room.Status == StatusWaiting && room.OpponentID == nil && time.Now().After(room.ExpiresAt) {
		_ = r.updateStatus(ctx, room.ID, StatusExpired)
		room.Status = StatusExpired
	}

	return &room, nil
}

func (r *postgresRepository) ListPublicRooms(ctx context.Context) ([]*Room, error) {
	query := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
		FROM rooms
		WHERE is_private = FALSE AND status IN ('waiting', 'playing', 'finished')
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
			&room.CreatorDisconnectedAt,
			&room.OpponentDisconnectedAt,
			&room.CreatorConnectionID,
			&room.OpponentConnectionID,
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
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala para juncao: %w", err)
	}

	// Validacoes
	if room.Status == StatusExpired || (room.OpponentID == nil && time.Now().After(room.ExpiresAt)) {
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
			return nil, ErrRoomAlreadyJoined
		}
		return nil, ErrRoomFull
	}

	if err := r.lockActiveRoomUsersTx(ctx, tx, opponentID); err != nil {
		return nil, err
	}

	hasActiveRoom, err := r.userHasActiveRoomTx(ctx, tx, opponentID, roomID)
	if err != nil {
		return nil, err
	}
	if hasActiveRoom {
		return nil, ErrUserInActiveRoom
	}

	queryUpdate := `
		UPDATE rooms
		SET opponent_id = $1
		WHERE id = $2
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
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

func (r *postgresRepository) StartRoom(ctx context.Context, roomID string) (*Room, error) {
	query := `
		UPDATE rooms
		SET status = 'playing'
		WHERE id = $1
			AND status = 'waiting'
			AND opponent_id IS NOT NULL
			AND creator_disconnected_at IS NULL
			AND opponent_disconnected_at IS NULL
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao iniciar sala: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) FinishRoom(ctx context.Context, roomID string) (*Room, error) {
	query := `
		UPDATE rooms
		SET status = 'finished'
		WHERE id = $1
			AND status = 'playing'
			AND opponent_id IS NOT NULL
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao finalizar sala: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) ResetRoom(ctx context.Context, roomID string) (*Room, error) {
	query := `
		UPDATE rooms
		SET status = 'waiting',
			expires_at = NOW() + INTERVAL '5 minutes',
			creator_disconnected_at = NULL,
			opponent_disconnected_at = NULL
		WHERE id = $1
			AND status = 'finished'
			AND opponent_id IS NOT NULL
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao resetar sala para revanche: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) StartRematchRoom(ctx context.Context, roomID string) (*Room, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar transacao de revanche: %w", err)
	}
	defer tx.Rollback(ctx)

	querySelect := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar sala para revanche: %w", err)
	}
	if room.Status != StatusFinished || room.OpponentID == nil {
		return nil, ErrRoomNotFound
	}

	if err := r.lockActiveRoomUsersTx(ctx, tx, room.CreatorID, *room.OpponentID); err != nil {
		return nil, err
	}

	creatorInOtherRoom, err := r.userHasActiveRoomTx(ctx, tx, room.CreatorID, room.ID)
	if err != nil {
		return nil, err
	}
	opponentInOtherRoom, err := r.userHasActiveRoomTx(ctx, tx, *room.OpponentID, room.ID)
	if err != nil {
		return nil, err
	}
	if creatorInOtherRoom || opponentInOtherRoom {
		return nil, ErrUserInActiveRoom
	}

	queryUpdate := `
		UPDATE rooms
		SET status = 'playing',
			expires_at = NOW() + INTERVAL '10 minutes',
			creator_disconnected_at = NULL,
			opponent_disconnected_at = NULL
		WHERE id = $1
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`
	err = tx.QueryRow(ctx, queryUpdate, roomID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao atualizar sala para revanche: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("falha ao commitar revanche: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) ExpireRooms(ctx context.Context) ([]string, error) {
	query := `
		UPDATE rooms
		SET status = 'expired'
		WHERE status = 'waiting' AND opponent_id IS NULL AND expires_at < $1
		RETURNING id
	`
	rows, err := r.pool.Query(ctx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("falha ao expirar salas: %w", err)
	}
	defer rows.Close()

	expiredIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("falha ao ler sala expirada: %w", err)
		}
		expiredIDs = append(expiredIDs, id)
	}

	return expiredIDs, rows.Err()
}

func (r *postgresRepository) updateStatus(ctx context.Context, roomID string, status RoomStatus) error {
	query := "UPDATE rooms SET status = $1 WHERE id = $2"
	_, err := r.pool.Exec(ctx, query, status, roomID)
	return err
}

func (r *postgresRepository) CloseRoom(ctx context.Context, roomID string) error {
	query := `
		UPDATE rooms
		SET status = 'expired',
			creator_disconnected_at = NULL,
			opponent_disconnected_at = NULL,
			creator_connection_id = NULL,
			opponent_connection_id = NULL
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, roomID)
	return err
}

func (r *postgresRepository) LeaveRoom(ctx context.Context, roomID string, userID string) (*Room, LeaveRole, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("falha ao iniciar transacao de saida da sala: %w", err)
	}
	defer tx.Rollback(ctx)

	querySelect := `
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
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
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrRoomNotFound
		}
		return nil, "", fmt.Errorf("falha ao buscar sala para saida: %w", err)
	}

	role := LeaveRoleSpectator
	if room.CreatorID == userID {
		role = LeaveRoleCreator
		queryUpdate := `
			UPDATE rooms
			SET status = 'expired',
				creator_disconnected_at = NULL,
				opponent_disconnected_at = NULL,
				creator_connection_id = NULL,
				opponent_connection_id = NULL
			WHERE id = $1
			RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
		`
		err = tx.QueryRow(ctx, queryUpdate, roomID).Scan(
			&room.ID,
			&room.Code,
			&room.CreatorID,
			&room.OpponentID,
			&room.Status,
			&room.IsPrivate,
			&room.CreatedAt,
			&room.ExpiresAt,
			&room.CreatorDisconnectedAt,
			&room.OpponentDisconnectedAt,
			&room.CreatorConnectionID,
			&room.OpponentConnectionID,
		)
		if err != nil {
			return nil, "", fmt.Errorf("falha ao fechar sala na saida do criador: %w", err)
		}
	} else if room.OpponentID != nil && *room.OpponentID == userID {
		role = LeaveRoleOpponent
		queryUpdate := `
			UPDATE rooms
			SET opponent_id = NULL,
				status = 'waiting',
				expires_at = NOW() + INTERVAL '5 minutes',
				opponent_disconnected_at = NULL,
				opponent_connection_id = NULL
			WHERE id = $1
			RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
		`
		err = tx.QueryRow(ctx, queryUpdate, roomID).Scan(
			&room.ID,
			&room.Code,
			&room.CreatorID,
			&room.OpponentID,
			&room.Status,
			&room.IsPrivate,
			&room.CreatedAt,
			&room.ExpiresAt,
			&room.CreatorDisconnectedAt,
			&room.OpponentDisconnectedAt,
			&room.CreatorConnectionID,
			&room.OpponentConnectionID,
		)
		if err != nil {
			return nil, "", fmt.Errorf("falha ao liberar vaga do oponente: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", fmt.Errorf("falha ao commitar saida da sala: %w", err)
	}

	return &room, role, nil
}

func (r *postgresRepository) MarkCreatorConnected(ctx context.Context, roomID string, creatorID string, connectionID string) (*Room, error) {
	query := `
		UPDATE rooms
		SET creator_disconnected_at = NULL, creator_connection_id = $1
		WHERE id = $2 AND creator_id = $3 AND status IN ('waiting', 'playing', 'finished')
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, connectionID, roomID, creatorID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao marcar criador conectado: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) MarkCreatorDisconnected(ctx context.Context, roomID string, creatorID string, connectionID string, disconnectedAt time.Time) (bool, error) {
	query := `
		UPDATE rooms
		SET creator_disconnected_at = $1
		WHERE id = $2
			AND creator_id = $3
			AND creator_connection_id = $4
			AND status IN ('waiting', 'playing', 'finished')
	`

	cmd, err := r.pool.Exec(ctx, query, disconnectedAt, roomID, creatorID, connectionID)
	if err != nil {
		return false, fmt.Errorf("falha ao marcar criador desconectado: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}

func (r *postgresRepository) MarkOpponentConnected(ctx context.Context, roomID string, opponentID string, connectionID string) (*Room, error) {
	query := `
		UPDATE rooms
		SET opponent_disconnected_at = NULL, opponent_connection_id = $1
		WHERE id = $2 AND opponent_id = $3 AND status IN ('waiting', 'playing', 'finished')
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, connectionID, roomID, opponentID).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao marcar oponente conectado: %w", err)
	}

	return &room, nil
}

func (r *postgresRepository) MarkOpponentDisconnected(ctx context.Context, roomID string, opponentID string, connectionID string, disconnectedAt time.Time) (bool, error) {
	query := `
		UPDATE rooms
		SET opponent_disconnected_at = $1
		WHERE id = $2
			AND opponent_id = $3
			AND opponent_connection_id = $4
			AND status IN ('waiting', 'playing', 'finished')
	`

	cmd, err := r.pool.Exec(ctx, query, disconnectedAt, roomID, opponentID, connectionID)
	if err != nil {
		return false, fmt.Errorf("falha ao marcar oponente desconectado: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}

func (r *postgresRepository) CloseRoomIfCreatorDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (bool, error) {
	query := `
		UPDATE rooms
		SET status = 'expired',
			creator_disconnected_at = NULL,
			opponent_disconnected_at = NULL,
			creator_connection_id = NULL,
			opponent_connection_id = NULL
		WHERE id = $1
			AND status IN ('waiting', 'playing', 'finished')
			AND creator_disconnected_at IS NOT NULL
			AND creator_disconnected_at <= $2
	`

	cmd, err := r.pool.Exec(ctx, query, roomID, disconnectedBefore)
	if err != nil {
		return false, fmt.Errorf("falha ao fechar sala por desconexao do criador: %w", err)
	}

	return cmd.RowsAffected() > 0, nil
}

func (r *postgresRepository) CloseRoomsWithExpiredCreatorDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]string, error) {
	query := `
		UPDATE rooms
		SET status = 'expired',
			creator_disconnected_at = NULL,
			opponent_disconnected_at = NULL,
			creator_connection_id = NULL,
			opponent_connection_id = NULL
		WHERE status IN ('waiting', 'playing', 'finished')
			AND creator_disconnected_at IS NOT NULL
			AND creator_disconnected_at <= $1
		RETURNING id
	`

	rows, err := r.pool.Query(ctx, query, disconnectedBefore)
	if err != nil {
		return nil, fmt.Errorf("falha ao fechar salas com criador desconectado: %w", err)
	}
	defer rows.Close()

	roomIDs := []string{}
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, fmt.Errorf("falha ao ler sala fechada por timeout do criador: %w", err)
		}
		roomIDs = append(roomIDs, roomID)
	}

	return roomIDs, rows.Err()
}

func (r *postgresRepository) ReleaseOpponentIfDisconnected(ctx context.Context, roomID string, disconnectedBefore time.Time) (*Room, bool, error) {
	query := `
		UPDATE rooms
		SET opponent_id = NULL,
			status = 'waiting',
			expires_at = NOW() + INTERVAL '5 minutes',
			opponent_disconnected_at = NULL,
			opponent_connection_id = NULL
		WHERE id = $1
			AND status IN ('waiting', 'playing', 'finished')
			AND opponent_id IS NOT NULL
			AND opponent_disconnected_at IS NOT NULL
			AND opponent_disconnected_at <= $2
		RETURNING id, code, creator_id, opponent_id, status, is_private, created_at, expires_at, creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id
	`

	var room Room
	err := r.pool.QueryRow(ctx, query, roomID, disconnectedBefore).Scan(
		&room.ID,
		&room.Code,
		&room.CreatorID,
		&room.OpponentID,
		&room.Status,
		&room.IsPrivate,
		&room.CreatedAt,
		&room.ExpiresAt,
		&room.CreatorDisconnectedAt,
		&room.OpponentDisconnectedAt,
		&room.CreatorConnectionID,
		&room.OpponentConnectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("falha ao liberar oponente desconectado: %w", err)
	}

	return &room, true, nil
}

func (r *postgresRepository) ReleaseRoomsWithExpiredOpponentDisconnect(ctx context.Context, disconnectedBefore time.Time) ([]ReleasedOpponent, error) {
	query := `
		WITH candidates AS (
			SELECT id, opponent_id AS released_opponent_id
			FROM rooms
			WHERE status IN ('waiting', 'playing', 'finished')
				AND opponent_id IS NOT NULL
				AND opponent_disconnected_at IS NOT NULL
				AND opponent_disconnected_at <= $1
		),
		updated AS (
			UPDATE rooms r
			SET opponent_id = NULL,
				status = 'waiting',
				expires_at = NOW() + INTERVAL '5 minutes',
				opponent_disconnected_at = NULL,
				opponent_connection_id = NULL
			FROM candidates c
			WHERE r.id = c.id
			RETURNING r.id, r.code, r.creator_id, r.opponent_id, r.status, r.is_private, r.created_at, r.expires_at,
				r.creator_disconnected_at, r.opponent_disconnected_at, r.creator_connection_id, r.opponent_connection_id,
				c.released_opponent_id
		)
		SELECT id, code, creator_id, opponent_id, status, is_private, created_at, expires_at,
			creator_disconnected_at, opponent_disconnected_at, creator_connection_id, opponent_connection_id,
			released_opponent_id
		FROM updated
	`

	rows, err := r.pool.Query(ctx, query, disconnectedBefore)
	if err != nil {
		return nil, fmt.Errorf("falha ao liberar oponentes desconectados: %w", err)
	}
	defer rows.Close()

	released := []ReleasedOpponent{}
	for rows.Next() {
		var room Room
		var opponentID string
		if err := rows.Scan(
			&room.ID,
			&room.Code,
			&room.CreatorID,
			&room.OpponentID,
			&room.Status,
			&room.IsPrivate,
			&room.CreatedAt,
			&room.ExpiresAt,
			&room.CreatorDisconnectedAt,
			&room.OpponentDisconnectedAt,
			&room.CreatorConnectionID,
			&room.OpponentConnectionID,
			&opponentID,
		); err != nil {
			return nil, fmt.Errorf("falha ao ler oponente liberado: %w", err)
		}
		released = append(released, ReleasedOpponent{Room: &room, OpponentID: opponentID})
	}

	return released, rows.Err()
}

func (r *postgresRepository) GetMatchSnapshot(ctx context.Context, roomID string) (json.RawMessage, error) {
	var snapshot json.RawMessage
	err := r.pool.QueryRow(ctx, "SELECT snapshot FROM room_match_states WHERE room_id = $1", roomID).Scan(&snapshot)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("falha ao buscar snapshot da partida: %w", err)
	}
	return append(json.RawMessage(nil), snapshot...), nil
}

func (r *postgresRepository) UpsertMatchSnapshot(ctx context.Context, roomID string, snapshot json.RawMessage) error {
	query := `
		INSERT INTO room_match_states (room_id, snapshot, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (room_id)
		DO UPDATE SET snapshot = EXCLUDED.snapshot, updated_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, roomID, snapshot)
	if err != nil {
		return fmt.Errorf("falha ao salvar snapshot da partida: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteMatchSnapshot(ctx context.Context, roomID string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM room_match_states WHERE room_id = $1", roomID)
	if err != nil {
		return fmt.Errorf("falha ao remover snapshot da partida: %w", err)
	}
	return nil
}

func (r *postgresRepository) generateUniqueCodeTx(ctx context.Context, tx pgx.Tx) (string, error) {
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
		err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM rooms WHERE code = $1)", codeStr).Scan(&exists)
		if err != nil {
			return "", err
		}

		if !exists {
			return codeStr, nil
		}
	}
	return "", errors.New("nao foi possivel gerar um codigo de sala unico apos varias tentativas")
}
