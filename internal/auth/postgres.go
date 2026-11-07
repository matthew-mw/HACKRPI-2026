package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PostgresStore struct{ DB *sql.DB }

func (s *PostgresStore) Create(ctx context.Context, hash [32]byte, userID string, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, hash[:], userID, expires)
	return err
}

func (s *PostgresStore) Lookup(ctx context.Context, hash [32]byte) (Session, error) {
	var session Session
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token_hash = $1 AND expires_at > now()`, hash[:]).Scan(&session.UserID, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrUnauthenticated
	}
	return session, err
}

func (s *PostgresStore) Delete(ctx context.Context, hash [32]byte) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash[:])
	return err
}

// DeleteUserSessions supports password resets and signing out every device.
func (s *PostgresStore) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *PostgresStore) DeleteExpired(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return err
}
