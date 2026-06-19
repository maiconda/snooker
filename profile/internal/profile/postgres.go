package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	query := `
		SELECT user_id, nickname, nickname_normalized, bio, photo_object_key, photo_url, xp, created_at, updated_at
		FROM profiles
		WHERE user_id = $1`
	return r.scanProfile(r.pool.QueryRow(ctx, query, userID))
}

func (r *pgRepository) Upsert(ctx context.Context, p *Profile) (*Profile, error) {
	query := `
		INSERT INTO profiles (user_id, nickname, nickname_normalized, bio, photo_object_key, photo_url, xp)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, 0))
		ON CONFLICT (user_id) DO UPDATE SET
			nickname = EXCLUDED.nickname,
			nickname_normalized = EXCLUDED.nickname_normalized,
			bio = EXCLUDED.bio,
			photo_object_key = EXCLUDED.photo_object_key,
			photo_url = EXCLUDED.photo_url,
			updated_at = CURRENT_TIMESTAMP
		RETURNING user_id, nickname, nickname_normalized, bio, photo_object_key, photo_url, xp, created_at, updated_at`

	result, err := r.scanProfile(r.pool.QueryRow(ctx, query,
		p.UserID, p.Nickname, p.NicknameNormalized, p.Bio, p.PhotoObjectKey, p.PhotoURL, p.XP,
	))
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrDuplicateNickname
		}
		return nil, err
	}
	return result, nil
}

func (r *pgRepository) Update(ctx context.Context, p *Profile) (*Profile, error) {
	query := `
		UPDATE profiles SET
			nickname = $2,
			nickname_normalized = $3,
			bio = $4,
			photo_object_key = $5,
			photo_url = $6,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
		RETURNING user_id, nickname, nickname_normalized, bio, photo_object_key, photo_url, xp, created_at, updated_at`

	result, err := r.scanProfile(r.pool.QueryRow(ctx, query,
		p.UserID, p.Nickname, p.NicknameNormalized, p.Bio, p.PhotoObjectKey, p.PhotoURL,
	))
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrDuplicateNickname
		}
		return nil, err
	}
	return result, nil
}

func (r *pgRepository) CreateUploadSession(ctx context.Context, session *PhotoUploadSession) (*PhotoUploadSession, error) {
	query := `
		INSERT INTO photo_upload_sessions (id, user_id, object_key, content_type, max_size_bytes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, object_key, content_type, max_size_bytes, expires_at, consumed_at, created_at`
	return r.scanUploadSession(r.pool.QueryRow(ctx, query,
		session.ID, session.UserID, session.ObjectKey, session.ContentType, session.MaxSizeBytes, session.ExpiresAt,
	))
}

func (r *pgRepository) FindUploadSession(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*PhotoUploadSession, error) {
	query := `
		SELECT id, user_id, object_key, content_type, max_size_bytes, expires_at, consumed_at, created_at
		FROM photo_upload_sessions
		WHERE id = $1 AND user_id = $2`
	return r.scanUploadSession(r.pool.QueryRow(ctx, query, id, userID))
}

func (r *pgRepository) MarkUploadConsumed(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE photo_upload_sessions SET consumed_at = $1 WHERE id = $2 AND consumed_at IS NULL`
	tag, err := r.pool.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("falha ao consumir upload: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidUpload
	}
	return nil
}

func (r *pgRepository) scanProfile(row pgx.Row) (*Profile, error) {
	p := &Profile{}
	err := row.Scan(
		&p.UserID, &p.Nickname, &p.NicknameNormalized, &p.Bio,
		&p.PhotoObjectKey, &p.PhotoURL, &p.XP, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("falha ao ler profile: %w", err)
	}
	return p, nil
}

func (r *pgRepository) scanUploadSession(row pgx.Row) (*PhotoUploadSession, error) {
	session := &PhotoUploadSession{}
	err := row.Scan(
		&session.ID, &session.UserID, &session.ObjectKey, &session.ContentType,
		&session.MaxSizeBytes, &session.ExpiresAt, &session.ConsumedAt, &session.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("falha ao ler sessao de upload: %w", err)
	}
	return session, nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "23505") || strings.Contains(errStr, "duplicate key")
}
