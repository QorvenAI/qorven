package presence

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Presence struct {
	UserID     string    `json:"user_id"`
	TenantID   string    `json:"tenant_id"`
	LastSeenAt time.Time `json:"last_seen_at"`
	IsOnline   bool      `json:"is_online"`
	Channel    string    `json:"channel"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) SetOnline(ctx context.Context, userID, tenantID, channel string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_presence (user_id, tenant_id, last_seen_at, is_online, channel)
         VALUES ($1, $2, now(), true, $3)
         ON CONFLICT (user_id) DO UPDATE
           SET is_online = true, last_seen_at = now(), channel = $3`,
		userID, tenantID, channel,
	)
	return err
}

func (s *Store) SetOffline(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE user_presence SET is_online = false, last_seen_at = now() WHERE user_id = $1`, userID)
	return err
}

func (s *Store) Get(ctx context.Context, userID string) (*Presence, error) {
	p := &Presence{}
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, tenant_id, last_seen_at, is_online, channel FROM user_presence WHERE user_id = $1`, userID,
	).Scan(&p.UserID, &p.TenantID, &p.LastSeenAt, &p.IsOnline, &p.Channel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) IsOnline(ctx context.Context, tenantID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM user_presence WHERE tenant_id = $1 AND is_online = true`, tenantID,
	).Scan(&count)
	return count > 0, err
}

func (s *Store) LastChannel(ctx context.Context, userID string) (string, error) {
	var channel string
	err := s.pool.QueryRow(ctx,
		`SELECT channel FROM user_presence WHERE user_id = $1`, userID,
	).Scan(&channel)
	if errors.Is(err, pgx.ErrNoRows) {
		return "web", nil
	}
	if err != nil {
		return "", err
	}
	return channel, nil
}
