package auth

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

type pgUsuarioRepo struct {
	pool *pgxpool.Pool
}

func NewUsuarioRepository(pool *pgxpool.Pool) UsuarioRepository {
	return &pgUsuarioRepo{pool: pool}
}

func (r *pgUsuarioRepo) Create(ctx context.Context, u *Usuario) (*Usuario, error) {
	query := `
		INSERT INTO usuarios (email, password_hash, provider, provider_id, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, email, password_hash, provider, provider_id, status, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query,
		u.Email, u.PasswordHash, u.Provider, u.ProviderID, u.Status,
	)

	result := &Usuario{}
	err := row.Scan(
		&result.ID, &result.Email, &result.PasswordHash, &result.Provider,
		&result.ProviderID, &result.Status, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrDuplicateEmail
		}
		return nil, fmt.Errorf("falha ao criar usuario: %w", err)
	}

	return result, nil
}

func (r *pgUsuarioRepo) FindByID(ctx context.Context, id uuid.UUID) (*Usuario, error) {
	query := `
		SELECT id, email, password_hash, provider, provider_id, status, created_at, updated_at
		FROM usuarios WHERE id = $1`

	return r.scanUsuario(r.pool.QueryRow(ctx, query, id))
}

func (r *pgUsuarioRepo) FindByEmail(ctx context.Context, email string) (*Usuario, error) {
	query := `
		SELECT id, email, password_hash, provider, provider_id, status, created_at, updated_at
		FROM usuarios WHERE email = $1`

	return r.scanUsuario(r.pool.QueryRow(ctx, query, email))
}

func (r *pgUsuarioRepo) FindByProviderID(ctx context.Context, provider AuthProvider, providerID string) (*Usuario, error) {
	query := `
		SELECT id, email, password_hash, provider, provider_id, status, created_at, updated_at
		FROM usuarios WHERE provider = $1 AND provider_id = $2`

	return r.scanUsuario(r.pool.QueryRow(ctx, query, provider, providerID))
}

func (r *pgUsuarioRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status UserStatus) error {
	query := `UPDATE usuarios SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	tag, err := r.pool.Exec(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("falha ao atualizar status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgUsuarioRepo) scanUsuario(row pgx.Row) (*Usuario, error) {
	u := &Usuario{}
	err := row.Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Provider,
		&u.ProviderID, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("falha ao ler usuario: %w", err)
	}
	return u, nil
}

type pgRefreshTokenRepo struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) RefreshTokenRepository {
	return &pgRefreshTokenRepo{pool: pool}
}

func (r *pgRefreshTokenRepo) Create(ctx context.Context, t *RefreshToken) (*RefreshToken, error) {
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at, revoked)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, token_hash, family_id, expires_at, revoked, revoked_at, created_at`

	row := r.pool.QueryRow(ctx, query,
		t.UserID, t.TokenHash, t.FamilyID, t.ExpiresAt, t.Revoked,
	)

	result := &RefreshToken{}
	err := row.Scan(
		&result.ID, &result.UserID, &result.TokenHash, &result.FamilyID,
		&result.ExpiresAt, &result.Revoked, &result.RevokedAt, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar refresh token: %w", err)
	}

	return result, nil
}

func (r *pgRefreshTokenRepo) FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, family_id, expires_at, revoked, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`

	return r.scanRefreshToken(r.pool.QueryRow(ctx, query, tokenHash))
}

func (r *pgRefreshTokenRepo) FindActiveByFamilyID(ctx context.Context, familyID uuid.UUID) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, family_id, expires_at, revoked, revoked_at, created_at
		FROM refresh_tokens
		WHERE family_id = $1 AND revoked = FALSE AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1`

	return r.scanRefreshToken(r.pool.QueryRow(ctx, query, familyID))
}

func (r *pgRefreshTokenRepo) RevokeByID(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	query := `UPDATE refresh_tokens SET revoked = TRUE, revoked_at = $1 WHERE id = $2`

	tag, err := r.pool.Exec(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("falha ao revogar refresh token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *pgRefreshTokenRepo) RevokeAllByFamilyID(ctx context.Context, familyID uuid.UUID) error {
	now := time.Now()
	query := `UPDATE refresh_tokens SET revoked = TRUE, revoked_at = $1 WHERE family_id = $2 AND revoked = FALSE`
	_, err := r.pool.Exec(ctx, query, now, familyID)
	if err != nil {
		return fmt.Errorf("falha ao revogar familia de tokens: %w", err)
	}
	return nil
}

func (r *pgRefreshTokenRepo) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	query := `UPDATE refresh_tokens SET revoked = TRUE, revoked_at = $1 WHERE user_id = $2 AND revoked = FALSE`
	_, err := r.pool.Exec(ctx, query, now, userID)
	if err != nil {
		return fmt.Errorf("falha ao revogar todos os tokens do usuario: %w", err)
	}
	return nil
}

func (r *pgRefreshTokenRepo) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM refresh_tokens WHERE expires_at < NOW()`
	tag, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("falha ao deletar tokens expirados: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *pgRefreshTokenRepo) scanRefreshToken(row pgx.Row) (*RefreshToken, error) {
	t := &RefreshToken{}
	err := row.Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID,
		&t.ExpiresAt, &t.Revoked, &t.RevokedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("falha ao ler refresh token: %w", err)
	}
	return t, nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "23505") || strings.Contains(errStr, "duplicate key")
}
