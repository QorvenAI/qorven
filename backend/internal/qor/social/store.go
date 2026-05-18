// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// store.go — Social media post and integration storage.

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// --- Posts ---

func (s *Store) CreatePost(ctx context.Context, p *Post) (string, error) {
	if p.Status == "" { p.Status = PostDraft }
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	platforms, _ := json.Marshal(p.Platforms)
	tags, _ := json.Marshal(p.Tags)
	media, _ := json.Marshal(p.MediaURLs)
	meta, _ := json.Marshal(p.Metadata)

	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO social_posts (content, media_urls, platforms, tags, status, scheduled_at, agent_id, team_id, metadata, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		p.Content, media, platforms, tags, p.Status, p.ScheduledAt, p.AgentID, p.TeamID, meta, now, now).Scan(&id)
	return id, err
}

func (s *Store) GetPost(ctx context.Context, postID string) (*Post, error) {
	var p Post
	var platforms, tags, media, meta []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, content, media_urls, platforms, tags, status, scheduled_at, published_at,
		 agent_id, COALESCE(team_id,''), metadata, created_at, updated_at
		 FROM social_posts WHERE id = $1`, postID).Scan(
		&p.ID, &p.Content, &media, &platforms, &tags, &p.Status, &p.ScheduledAt, &p.PublishedAt,
		&p.AgentID, &p.TeamID, &meta, &p.CreatedAt, &p.UpdatedAt)
	if err != nil { return nil, err }
	json.Unmarshal(platforms, &p.Platforms)
	json.Unmarshal(tags, &p.Tags)
	json.Unmarshal(media, &p.MediaURLs)
	json.Unmarshal(meta, &p.Metadata)
	return &p, nil
}

func (s *Store) ListPosts(ctx context.Context, agentID string, status PostStatus, limit, offset int) ([]Post, error) {
	if limit <= 0 { limit = 50 }
	query := `SELECT id, content, platforms, status, scheduled_at, published_at, agent_id, created_at
		FROM social_posts WHERE agent_id = $1`
	args := []any{agentID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil { return nil, err }
	defer rows.Close()

	posts := []Post{}
	for rows.Next() {
		var p Post
		var platforms []byte
		rows.Scan(&p.ID, &p.Content, &platforms, &p.Status, &p.ScheduledAt, &p.PublishedAt, &p.AgentID, &p.CreatedAt)
		json.Unmarshal(platforms, &p.Platforms)
		posts = append(posts, p)
	}
	return posts, nil
}

func (s *Store) UpdatePostStatus(ctx context.Context, postID string, status PostStatus) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE social_posts SET status = $1, updated_at = $2 WHERE id = $3`,
		status, time.Now(), postID)
	return err
}

func (s *Store) MarkPublished(ctx context.Context, postID string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE social_posts SET status = 'published', published_at = $1, updated_at = $1 WHERE id = $2`, now, postID)
	return err
}

func (s *Store) DeletePost(ctx context.Context, postID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM social_posts WHERE id = $1`, postID)
	return err
}

func (s *Store) ListScheduledDue(ctx context.Context) ([]Post, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, content, platforms, agent_id, scheduled_at FROM social_posts
		 WHERE status = 'scheduled' AND scheduled_at <= NOW() ORDER BY scheduled_at`)
	if err != nil { return nil, err }
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		var p Post
		var platforms []byte
		rows.Scan(&p.ID, &p.Content, &platforms, &p.AgentID, &p.ScheduledAt)
		json.Unmarshal(platforms, &p.Platforms)
		posts = append(posts, p)
	}
	return posts, nil
}

// --- Integrations ---

func (s *Store) SaveIntegration(ctx context.Context, i Integration) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO social_integrations (platform, account_name, account_id, access_token, refresh_token,
		 token_expiry, agent_id, active, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (agent_id, platform, account_id) DO UPDATE SET
		   access_token = $4, refresh_token = $5, token_expiry = $6, active = $8
		 RETURNING id`,
		i.Platform, i.AccountName, i.AccountID, i.AccessToken, i.RefreshToken,
		i.TokenExpiry, i.AgentID, i.Active, time.Now()).Scan(&id)
	return id, err
}

func (s *Store) ListIntegrations(ctx context.Context, agentID string) ([]Integration, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, platform, account_name, account_id, token_expiry, agent_id, active, created_at
		 FROM social_integrations WHERE agent_id = $1 ORDER BY platform`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	integrations := []Integration{}
	for rows.Next() {
		var i Integration
		rows.Scan(&i.ID, &i.Platform, &i.AccountName, &i.AccountID, &i.TokenExpiry, &i.AgentID, &i.Active, &i.CreatedAt)
		integrations = append(integrations, i)
	}
	return integrations, nil
}

func (s *Store) GetIntegrationToken(ctx context.Context, agentID string, platform Platform) (string, string, error) {
	var access, refresh string
	err := s.pool.QueryRow(ctx,
		`SELECT access_token, COALESCE(refresh_token,'') FROM social_integrations
		 WHERE agent_id = $1 AND platform = $2 AND active = true`, agentID, platform).Scan(&access, &refresh)
	return access, refresh, err
}

func (s *Store) DeleteIntegration(ctx context.Context, integrationID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM social_integrations WHERE id = $1`, integrationID)
	return err
}

// --- AutoPosts ---

func (s *Store) CreateAutoPost(ctx context.Context, a AutoPost) (string, error) {
	var id string
	platforms, _ := json.Marshal(a.Platforms)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO social_autoposts (name, source, source_url, platforms, schedule, template, active, agent_id, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		a.Name, a.Source, a.SourceURL, platforms, a.Schedule, a.Template, a.Active, a.AgentID, time.Now()).Scan(&id)
	return id, err
}

func (s *Store) ListAutoPosts(ctx context.Context, agentID string) ([]AutoPost, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, source, COALESCE(source_url,''), platforms, schedule, active, agent_id, created_at
		 FROM social_autoposts WHERE agent_id = $1 ORDER BY name`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	autoposts := []AutoPost{}
	for rows.Next() {
		var a AutoPost
		var platforms []byte
		rows.Scan(&a.ID, &a.Name, &a.Source, &a.SourceURL, &platforms, &a.Schedule, &a.Active, &a.AgentID, &a.CreatedAt)
		json.Unmarshal(platforms, &a.Platforms)
		autoposts = append(autoposts, a)
	}
	return autoposts, nil
}

func (s *Store) ToggleAutoPost(ctx context.Context, autopostID string, active bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE social_autoposts SET active = $1 WHERE id = $2`, active, autopostID)
	return err
}

func (s *Store) DeleteAutoPost(ctx context.Context, autopostID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM social_autoposts WHERE id = $1`, autopostID)
	return err
}
