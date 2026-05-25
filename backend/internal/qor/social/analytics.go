// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// PostMetrics holds engagement data for a single platform at a point in time.
type PostMetrics struct {
	ID             string    `json:"id"`
	PostID         string    `json:"post_id"`
	Platform       Platform  `json:"platform"`
	PlatformPostID string    `json:"platform_post_id"`
	Impressions    int64     `json:"impressions"`
	Likes          int64     `json:"likes"`
	Shares         int64     `json:"shares"`
	Comments       int64     `json:"comments"`
	Clicks         int64     `json:"clicks"`
	Reach          int64     `json:"reach"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// SaveMetrics stores a metrics snapshot for a post+platform.
func (s *Store) SaveMetrics(ctx context.Context, m PostMetrics) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO social_post_metrics
		 (post_id, platform, platform_post_id, impressions, likes, shares, comments, clicks, reach, fetched_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (post_id, platform, fetched_at) DO NOTHING`,
		m.PostID, m.Platform, m.PlatformPostID,
		m.Impressions, m.Likes, m.Shares, m.Comments, m.Clicks, m.Reach,
		m.FetchedAt)
	return err
}

// GetMetrics returns the latest metrics snapshot for a post across all platforms.
func (s *Store) GetMetrics(ctx context.Context, postID string) ([]PostMetrics, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT ON (platform) id, post_id, platform, platform_post_id,
		 impressions, likes, shares, comments, clicks, reach, fetched_at
		 FROM social_post_metrics WHERE post_id = $1
		 ORDER BY platform, fetched_at DESC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PostMetrics{}
	for rows.Next() {
		var m PostMetrics
		rows.Scan(&m.ID, &m.PostID, &m.Platform, &m.PlatformPostID,
			&m.Impressions, &m.Likes, &m.Shares, &m.Comments, &m.Clicks, &m.Reach, &m.FetchedAt)
		out = append(out, m)
	}
	return out, nil
}

// GetAggregateMetrics returns summed metrics across all posts for an agent,
// grouped by platform, for the given time range.
func (s *Store) GetAggregateMetrics(ctx context.Context, agentID string, since time.Time) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.platform,
		 SUM(m.impressions) AS impressions, SUM(m.likes) AS likes,
		 SUM(m.shares) AS shares, SUM(m.comments) AS comments,
		 COUNT(DISTINCT m.post_id) AS post_count
		 FROM social_post_metrics m
		 JOIN social_posts p ON p.id = m.post_id
		 WHERE p.agent_id = $1 AND m.fetched_at >= $2
		 GROUP BY m.platform ORDER BY impressions DESC`, agentID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var platform string
		var impressions, likes, shares, comments, postCount int64
		rows.Scan(&platform, &impressions, &likes, &shares, &comments, &postCount)
		out = append(out, map[string]any{
			"platform":    platform,
			"impressions": impressions,
			"likes":       likes,
			"shares":      shares,
			"comments":    comments,
			"post_count":  postCount,
		})
	}
	return out, nil
}

// GetTopPosts returns the top N posts by total engagement for an agent.
func (s *Store) GetTopPosts(ctx context.Context, agentID string, limit int, since time.Time) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx,
		`SELECT p.id, p.content, m.platform,
		 (m.likes + m.shares + m.comments) AS engagement,
		 m.impressions, m.likes, m.shares, m.comments
		 FROM social_post_metrics m
		 JOIN social_posts p ON p.id = m.post_id
		 WHERE p.agent_id = $1 AND m.fetched_at >= $2
		 ORDER BY engagement DESC LIMIT $3`, agentID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, content, platform string
		var engagement, impressions, likes, shares, comments int64
		rows.Scan(&id, &content, &platform, &engagement, &impressions, &likes, &shares, &comments)
		out = append(out, map[string]any{
			"post_id":     id,
			"content":     content,
			"platform":    platform,
			"engagement":  engagement,
			"impressions": impressions,
			"likes":       likes,
			"shares":      shares,
			"comments":    comments,
		})
	}
	return out, nil
}

// ListPublishedWithPlatformIDs returns posts that have platform_post_ids for metrics polling.
func (s *Store) ListPublishedWithPlatformIDs(ctx context.Context) ([]struct {
	ID              string
	PlatformPostIDs map[string]string
	AgentID         string
}, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, platform_post_ids, agent_id FROM social_posts
		 WHERE status = 'published' AND platform_post_ids != '{}'::jsonb
		 ORDER BY published_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []struct {
		ID              string
		PlatformPostIDs map[string]string
		AgentID         string
	}{}
	for rows.Next() {
		var id, agentID string
		var raw []byte
		rows.Scan(&id, &raw, &agentID)
		var ids map[string]string
		json.Unmarshal(raw, &ids)
		out = append(out, struct {
			ID              string
			PlatformPostIDs map[string]string
			AgentID         string
		}{id, ids, agentID})
	}
	return out, nil
}

// StorePlatformPostIDs saves the platform post IDs returned after publishing.
func (s *Store) StorePlatformPostIDs(ctx context.Context, postID string, ids map[string]string) error {
	raw, _ := json.Marshal(ids)
	_, err := s.pool.Exec(ctx,
		`UPDATE social_posts SET platform_post_ids = $1 WHERE id = $2`, raw, postID)
	return err
}

// ─── Analytics Worker ──────────────────────────────────────────────────────────

// AnalyticsWorker periodically fetches engagement metrics for published posts.
type AnalyticsWorker struct {
	store  *Store
	client *http.Client
	// tokenFn returns the access token for agent+platform
	tokenFn func(ctx context.Context, agentID string, platform Platform) (string, error)
}

func NewAnalyticsWorker(store *Store, tokenFn func(ctx context.Context, agentID string, platform Platform) (string, error)) *AnalyticsWorker {
	return &AnalyticsWorker{
		store:   store,
		client:  &http.Client{Timeout: 15 * time.Second},
		tokenFn: tokenFn,
	}
}

// Start runs the analytics polling loop until ctx is cancelled.
// Polls every hour.
func (w *AnalyticsWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *AnalyticsWorker) runOnce(ctx context.Context) {
	posts, err := w.store.ListPublishedWithPlatformIDs(ctx)
	if err != nil {
		slog.Error("social.analytics.list_failed", "err", err)
		return
	}
	if len(posts) == 0 {
		return
	}
	slog.Info("social.analytics.polling", "count", len(posts))
	for _, post := range posts {
		for platformStr, platformPostID := range post.PlatformPostIDs {
			platform := Platform(platformStr)
			token, err := w.tokenFn(ctx, post.AgentID, platform)
			if err != nil || token == "" {
				continue
			}
			metrics := w.fetchMetrics(ctx, platform, token, platformPostID)
			if metrics == nil {
				continue
			}
			metrics.PostID = post.ID
			metrics.PlatformPostID = platformPostID
			metrics.FetchedAt = time.Now()
			if err := w.store.SaveMetrics(ctx, *metrics); err != nil {
				slog.Warn("social.analytics.save_failed", "post_id", post.ID, "platform", platform, "err", err)
			}
		}
	}
}

func (w *AnalyticsWorker) fetchMetrics(ctx context.Context, platform Platform, token, postID string) *PostMetrics {
	m := &PostMetrics{Platform: platform}
	switch platform {
	case PlatformTwitter:
		return w.fetchTwitterMetrics(ctx, token, postID, m)
	case PlatformLinkedIn:
		return w.fetchLinkedInMetrics(ctx, token, postID, m)
	case PlatformFacebook:
		return w.fetchFacebookMetrics(ctx, token, postID, m)
	default:
		// Platform doesn't support metrics polling yet — return zeros
		return m
	}
}

func (w *AnalyticsWorker) fetchTwitterMetrics(ctx context.Context, token, tweetID string, m *PostMetrics) *PostMetrics {
	url := fmt.Sprintf("https://api.twitter.com/2/tweets/%s?tweet.fields=public_metrics", tweetID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := w.client.Do(req)
	if err != nil {
		return m
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		Data struct {
			PublicMetrics struct {
				RetweetCount int64 `json:"retweet_count"`
				LikeCount    int64 `json:"like_count"`
				ReplyCount   int64 `json:"reply_count"`
				ImpressionCount int64 `json:"impression_count"`
			} `json:"public_metrics"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return m
	}
	m.Likes = r.Data.PublicMetrics.LikeCount
	m.Shares = r.Data.PublicMetrics.RetweetCount
	m.Comments = r.Data.PublicMetrics.ReplyCount
	m.Impressions = r.Data.PublicMetrics.ImpressionCount
	return m
}

func (w *AnalyticsWorker) fetchLinkedInMetrics(ctx context.Context, token, postID string, m *PostMetrics) *PostMetrics {
	url := fmt.Sprintf("https://api.linkedin.com/v2/socialActions/%s", postID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := w.client.Do(req)
	if err != nil {
		return m
	}
	defer resp.Body.Close()
	var r struct {
		LikesSummary struct{ TotalLikes int64 `json:"totalLikes"` } `json:"likesSummary"`
		CommentsSummary struct{ TotalFirstLevelComments int64 `json:"totalFirstLevelComments"` } `json:"commentsSummary"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	m.Likes = r.LikesSummary.TotalLikes
	m.Comments = r.CommentsSummary.TotalFirstLevelComments
	return m
}

func (w *AnalyticsWorker) fetchFacebookMetrics(ctx context.Context, token, postID string, m *PostMetrics) *PostMetrics {
	url := fmt.Sprintf("https://graph.facebook.com/%s/insights/post_impressions,post_engaged_users?access_token=%s", postID, token)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := w.client.Do(req)
	if err != nil {
		return m
	}
	defer resp.Body.Close()
	var r struct {
		Data []struct {
			Name   string `json:"name"`
			Values []struct{ Value int64 `json:"value"` } `json:"values"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return m
	}
	for _, d := range r.Data {
		if len(d.Values) > 0 {
			switch d.Name {
			case "post_impressions":
				m.Impressions = d.Values[len(d.Values)-1].Value
			case "post_engaged_users":
				m.Clicks = d.Values[len(d.Values)-1].Value
			}
		}
	}
	return m
}

// ─── Unused import guard ──────────────────────────────────────────────────────
var _ = bytes.NewReader
