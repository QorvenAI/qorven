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
	ChatID     string    `json:"chat_id"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) SetOnline(ctx context.Context, userID, tenantID, channel string) error {
	return s.SetOnlineWithChatID(ctx, userID, tenantID, channel, "")
}

func (s *Store) SetOnlineWithChatID(ctx context.Context, userID, tenantID, channel, chatID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_presence (user_id, tenant_id, last_seen_at, is_online, channel, chat_id)
         VALUES ($1, $2, now(), true, $3, $4)
         ON CONFLICT (user_id) DO UPDATE
           SET is_online = true, last_seen_at = now(), channel = $3,
               chat_id = CASE WHEN $4 = '' THEN user_presence.chat_id ELSE $4 END`,
		userID, tenantID, channel, chatID,
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
		`SELECT user_id, tenant_id, last_seen_at, is_online, channel, COALESCE(chat_id,'') FROM user_presence WHERE user_id = $1`, userID,
	).Scan(&p.UserID, &p.TenantID, &p.LastSeenAt, &p.IsOnline, &p.Channel, &p.ChatID)
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
	ch, _, err := s.LastChannelAndChatID(ctx, userID)
	return ch, err
}

// LastChannelAndChatID returns the last channel name and the chat_id (e.g. Telegram chat ID)
// the user was seen on. Both are empty-string when there is no presence record.
func (s *Store) LastChannelAndChatID(ctx context.Context, userID string) (channel, chatID string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT channel, COALESCE(chat_id,'') FROM user_presence WHERE user_id = $1`, userID,
	).Scan(&channel, &chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "web", "", nil
	}
	return channel, chatID, err
}
