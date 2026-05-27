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
	relayMeta, _ := json.Marshal(i.RelayMetadata)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO social_integrations (platform, account_name, account_id, access_token, refresh_token,
		 token_expiry, agent_id, active, created_at, relay_provider, relay_provider_key_id, relay_account_id, relay_metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, COALESCE(NULLIF($10,''),'direct'), NULLIF($11,'')::uuid, $12, $13)
		 ON CONFLICT (agent_id, platform, account_id) DO UPDATE SET
		   access_token = $4, refresh_token = $5, token_expiry = $6, active = $8,
		   relay_provider = COALESCE(NULLIF($10,''),'direct'), relay_provider_key_id = NULLIF($11,'')::uuid,
		   relay_account_id = $12, relay_metadata = $13
		 RETURNING id`,
		i.Platform, i.AccountName, i.AccountID, i.AccessToken, i.RefreshToken,
		i.TokenExpiry, i.AgentID, i.Active, time.Now(),
		i.RelayProvider, i.RelayProviderKeyID, i.RelayAccountID, relayMeta).Scan(&id)
	return id, err
}

func (s *Store) ListIntegrations(ctx context.Context, agentID string) ([]Integration, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, platform, account_name, account_id, token_expiry, agent_id, active, created_at,
		        COALESCE(nickname,''), COALESCE(avatar_url,''), COALESCE(post_hours,'[]'::jsonb),
		        COALESCE(post_days,'[0,1,2,3,4,5,6]'::jsonb), COALESCE(group_name,''), COALESCE(paused,false),
		        COALESCE(relay_provider,'direct'), COALESCE(relay_provider_key_id::text,''), COALESCE(relay_account_id,''), COALESCE(relay_metadata,'{}'::jsonb)
		 FROM social_integrations WHERE agent_id = $1 ORDER BY platform`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	integrations := []Integration{}
	for rows.Next() {
		var i Integration
		var hoursJSON, daysJSON, relayMetaJSON []byte
		rows.Scan(&i.ID, &i.Platform, &i.AccountName, &i.AccountID, &i.TokenExpiry, &i.AgentID, &i.Active, &i.CreatedAt,
			&i.Nickname, &i.AvatarURL, &hoursJSON, &daysJSON, &i.GroupName, &i.Paused,
			&i.RelayProvider, &i.RelayProviderKeyID, &i.RelayAccountID, &relayMetaJSON)
		jsonUnmarshalInts(hoursJSON, &i.PostHours)
		jsonUnmarshalInts(daysJSON, &i.PostDays)
		if len(relayMetaJSON) > 0 {
			json.Unmarshal(relayMetaJSON, &i.RelayMetadata) //nolint:errcheck
		}
		integrations = append(integrations, i)
	}
	return integrations, nil
}

// GetIntegrationByID fetches a single integration by its ID (including relay fields).
func (s *Store) GetIntegrationByID(ctx context.Context, integrationID string) (*Integration, error) {
	var i Integration
	var hoursJSON, daysJSON, relayMetaJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, platform, account_name, account_id, access_token, COALESCE(refresh_token,''),
		 token_expiry, agent_id, active, created_at,
		 COALESCE(nickname,''), COALESCE(avatar_url,''), COALESCE(post_hours,'[]'::jsonb),
		 COALESCE(post_days,'[0,1,2,3,4,5,6]'::jsonb), COALESCE(group_name,''), COALESCE(paused,false),
		 COALESCE(relay_provider,'direct'), COALESCE(relay_provider_key_id::text,''), COALESCE(relay_account_id,''), COALESCE(relay_metadata,'{}'::jsonb)
		 FROM social_integrations WHERE id = $1`, integrationID).Scan(
		&i.ID, &i.Platform, &i.AccountName, &i.AccountID, &i.AccessToken, &i.RefreshToken,
		&i.TokenExpiry, &i.AgentID, &i.Active, &i.CreatedAt,
		&i.Nickname, &i.AvatarURL, &hoursJSON, &daysJSON, &i.GroupName, &i.Paused,
		&i.RelayProvider, &i.RelayProviderKeyID, &i.RelayAccountID, &relayMetaJSON)
	if err != nil {
		return nil, err
	}
	jsonUnmarshalInts(hoursJSON, &i.PostHours)
	jsonUnmarshalInts(daysJSON, &i.PostDays)
	if len(relayMetaJSON) > 0 {
		json.Unmarshal(relayMetaJSON, &i.RelayMetadata) //nolint:errcheck
	}
	return &i, nil
}

// UpdateIntegrationSettings patches per-channel settings for an integration.
func (s *Store) UpdateIntegrationSettings(ctx context.Context, id, nickname, avatarURL, groupName string, postHours, postDays []int, paused bool) error {
	hoursJSON, _ := json.Marshal(postHours)
	daysJSON, _ := json.Marshal(postDays)
	_, err := s.pool.Exec(ctx,
		`UPDATE social_integrations SET
		   nickname = $1, avatar_url = $2, post_hours = $3, post_days = $4, group_name = $5, paused = $6
		 WHERE id = $7`,
		nickname, avatarURL, hoursJSON, daysJSON, groupName, paused, id)
	return err
}

func jsonUnmarshalInts(data []byte, out *[]int) {
	if len(data) == 0 { return }
	json.Unmarshal(data, out) //nolint:errcheck
}

// FilterAllowedPlatforms returns only the platforms that pass their integration's
// posting-hour and posting-day restrictions at the given time.
// skipAll is true when every platform is gated — the caller should defer the post.
func (s *Store) FilterAllowedPlatforms(ctx context.Context, agentID string, platforms []Platform, at time.Time) (allowed []Platform, skipAll bool) {
	// Load settings for every integration belonging to this agent
	rows, err := s.pool.Query(ctx,
		`SELECT platform, COALESCE(post_hours,'[]'::jsonb), COALESCE(post_days,'[0,1,2,3,4,5,6]'::jsonb), COALESCE(paused,false)
		 FROM social_integrations WHERE agent_id = $1 AND active = true`, agentID)
	if err != nil {
		// If we can't read settings, allow all platforms
		return platforms, false
	}
	defer rows.Close()

	type settings struct {
		hours  []int
		days   []int
		paused bool
	}
	byPlatform := map[Platform]*settings{}
	for rows.Next() {
		var platform Platform
		var hoursJSON, daysJSON []byte
		var paused bool
		rows.Scan(&platform, &hoursJSON, &daysJSON, &paused)
		s := &settings{paused: paused}
		jsonUnmarshalInts(hoursJSON, &s.hours)
		jsonUnmarshalInts(daysJSON, &s.days)
		byPlatform[platform] = s
	}

	hour := at.Hour()             // 0-23
	day := int(at.Weekday())      // 0=Sun … 6=Sat

	for _, p := range platforms {
		cfg, ok := byPlatform[p]
		if !ok {
			// No integration record found — allow (token-based or no restrictions)
			allowed = append(allowed, p)
			continue
		}
		if cfg.paused {
			continue
		}
		// Check posting hours (empty = any hour)
		if len(cfg.hours) > 0 {
			hourOK := false
			for _, h := range cfg.hours {
				if h == hour {
					hourOK = true
					break
				}
			}
			if !hourOK {
				continue
			}
		}
		// Check posting days (empty = any day)
		if len(cfg.days) > 0 {
			dayOK := false
			for _, d := range cfg.days {
				if d == day {
					dayOK = true
					break
				}
			}
			if !dayOK {
				continue
			}
		}
		allowed = append(allowed, p)
	}

	return allowed, len(allowed) == 0
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

// ListExpiringSoon returns active integrations whose token expires within the given window.
// Used by the token refresh background worker.
func (s *Store) ListExpiringSoon(ctx context.Context, within time.Duration) ([]Integration, error) {
	deadline := time.Now().Add(within)
	rows, err := s.pool.Query(ctx,
		`SELECT id, platform, account_name, account_id, access_token, COALESCE(refresh_token,''),
		 token_expiry, agent_id, active
		 FROM social_integrations
		 WHERE active = true AND token_expiry IS NOT NULL AND token_expiry <= $1
		 ORDER BY token_expiry`, deadline)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Integration{}
	for rows.Next() {
		var i Integration
		rows.Scan(&i.ID, &i.Platform, &i.AccountName, &i.AccountID,
			&i.AccessToken, &i.RefreshToken, &i.TokenExpiry, &i.AgentID, &i.Active)
		out = append(out, i)
	}
	return out, nil
}

// UpdateTokens replaces access/refresh tokens and expiry for an integration.
func (s *Store) UpdateTokens(ctx context.Context, integrationID, accessToken, refreshToken string, expiry *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE social_integrations
		 SET access_token = $1, refresh_token = $2, token_expiry = $3, active = true
		 WHERE id = $4`,
		accessToken, refreshToken, expiry, integrationID)
	return err
}

// MarkNeedsReconnect sets active=false when token refresh fails.
func (s *Store) MarkNeedsReconnect(ctx context.Context, integrationID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE social_integrations SET active = false WHERE id = $1`, integrationID)
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

// --- Account Rules ---

func (s *Store) GetAccountRules(ctx context.Context, integrationID string) (*AccountRules, error) {
	var r AccountRules
	var hashtagsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, integration_id, voice_style, content_rules,
		 knowledge_context, hashtag_sets, posting_guidelines, created_at, updated_at
		 FROM social_account_rules WHERE integration_id = $1`, integrationID).Scan(
		&r.ID, &r.TenantID, &r.AgentID, &r.IntegrationID, &r.VoiceStyle, &r.ContentRules,
		&r.KnowledgeContext, &hashtagsJSON, &r.PostingGuidelines, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(hashtagsJSON) > 0 {
		json.Unmarshal(hashtagsJSON, &r.HashtagSets)
	}
	if r.HashtagSets == nil {
		r.HashtagSets = map[string][]string{}
	}
	return &r, nil
}

func (s *Store) UpsertAccountRules(ctx context.Context, rules *AccountRules) error {
	hashtagsJSON, _ := json.Marshal(rules.HashtagSets)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO social_account_rules (tenant_id, agent_id, integration_id, voice_style, content_rules,
		 knowledge_context, hashtag_sets, posting_guidelines, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, now())
		 ON CONFLICT (agent_id, integration_id) DO UPDATE SET
		   voice_style = $4, content_rules = $5, knowledge_context = $6,
		   hashtag_sets = $7, posting_guidelines = $8, updated_at = now()`,
		rules.TenantID, rules.AgentID, rules.IntegrationID, rules.VoiceStyle, rules.ContentRules,
		rules.KnowledgeContext, hashtagsJSON, rules.PostingGuidelines)
	return err
}

func (s *Store) ListAccountRulesByAgent(ctx context.Context, agentID string) ([]AccountRules, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_id, integration_id, voice_style, content_rules,
		 knowledge_context, hashtag_sets, posting_guidelines, created_at, updated_at
		 FROM social_account_rules WHERE agent_id = $1::uuid ORDER BY created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccountRules{}
	for rows.Next() {
		var r AccountRules
		var hashtagsJSON []byte
		rows.Scan(&r.ID, &r.TenantID, &r.AgentID, &r.IntegrationID, &r.VoiceStyle, &r.ContentRules,
			&r.KnowledgeContext, &hashtagsJSON, &r.PostingGuidelines, &r.CreatedAt, &r.UpdatedAt)
		if len(hashtagsJSON) > 0 {
			json.Unmarshal(hashtagsJSON, &r.HashtagSets)
		}
		if r.HashtagSets == nil {
			r.HashtagSets = map[string][]string{}
		}
		out = append(out, r)
	}
	return out, nil
}
