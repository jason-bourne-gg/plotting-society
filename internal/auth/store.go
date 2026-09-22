package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jason-bourne-gg/plotting-society/internal/database"
	"github.com/jason-bourne-gg/plotting-society/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

const userColumns = `
	id, builder_id, COALESCE(email,''), COALESCE(phone,''), name, role,
	COALESCE(avatar_url,''), COALESCE(current_address,''), COALESCE(city,''),
	directory_opt_in, is_active, created_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.BuilderID, &u.Email, &u.Phone, &u.Name, &u.Role,
		&u.AvatarURL, &u.CurrentAddress, &u.City, &u.DirectoryOptIn, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (domain.User, string, error) {
	row := s.db.QueryRow(ctx,
		`SELECT `+userColumns+`, COALESCE(password_hash,'') FROM users WHERE lower(email) = lower($1)`, email)

	var u domain.User
	var hash string
	err := row.Scan(&u.ID, &u.BuilderID, &u.Email, &u.Phone, &u.Name, &u.Role,
		&u.AvatarURL, &u.CurrentAddress, &u.City, &u.DirectoryOptIn, &u.IsActive, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (s *Store) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateProfile(ctx context.Context, id uuid.UUID, name, phone, address, city string, optIn bool) (domain.User, error) {
	return scanUser(s.db.QueryRow(ctx, `
		UPDATE users
		   SET name = $2, phone = NULLIF($3,''), current_address = NULLIF($4,''),
		       city = NULLIF($5,''), directory_opt_in = $6, updated_at = now()
		 WHERE id = $1
		RETURNING `+userColumns, id, name, phone, address, city, optIn))
}

func (s *Store) SetPassword(ctx context.Context, id uuid.UUID, hash string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	return err
}

// ------------------------------------------------------------------ invites

type Invite struct {
	ID        uuid.UUID
	Email     string
	Name      string
	Role      domain.Role
	BuilderID *uuid.UUID
	PlotID    *uuid.UUID
	ExpiresAt time.Time
}

func (s *Store) CreateInvite(ctx context.Context, in Invite, tokenHash string, createdBy uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `
		INSERT INTO invites (email, name, role, builder_id, plot_id, token_hash, expires_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		in.Email, in.Name, in.Role, in.BuilderID, in.PlotID, tokenHash, in.ExpiresAt, createdBy,
	).Scan(&id)
	return id, err
}

func (s *Store) InviteByTokenHash(ctx context.Context, hash string) (Invite, error) {
	var in Invite
	err := s.db.QueryRow(ctx, `
		SELECT id, email, name, role, builder_id, plot_id, expires_at
		  FROM invites
		 WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > now()`, hash,
	).Scan(&in.ID, &in.Email, &in.Name, &in.Role, &in.BuilderID, &in.PlotID, &in.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return in, err
}

// AcceptInvite creates the user, links them to their plot and burns the invite
// in one transaction: a half-accepted invite would strand the owner.
func (s *Store) AcceptInvite(ctx context.Context, in Invite, passwordHash string) (domain.User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (builder_id, email, name, role, password_hash)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING `+userColumns,
		in.BuilderID, in.Email, in.Name, in.Role, passwordHash))
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}

	if in.PlotID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE plots SET owner_id = $1, status = 'sold', updated_at = now() WHERE id = $2`,
			user.ID, *in.PlotID); err != nil {
			return domain.User{}, fmt.Errorf("link plot: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE invites SET accepted_at = now() WHERE id = $1`, in.ID); err != nil {
		return domain.User{}, fmt.Errorf("burn invite: %w", err)
	}

	return user, tx.Commit(ctx)
}

// ----------------------------------------------------------- refresh tokens

func (s *Store) StoreRefreshToken(ctx context.Context, userID uuid.UUID, hash string, expires time.Time, ua string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent)
		VALUES ($1,$2,$3,NULLIF($4,''))`, userID, hash, expires, ua)
	return err
}

func (s *Store) ConsumeRefreshToken(ctx context.Context, hash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.db.QueryRow(ctx, `
		UPDATE refresh_tokens
		   SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id`, hash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return userID, err
}

func (s *Store) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
