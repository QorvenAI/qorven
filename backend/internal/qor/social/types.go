// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package social

import "time"

// types.go — Social media management types for Qorven-Social.

// Platform represents a social media platform.
type Platform string

const (
	PlatformTwitter   Platform = "twitter"
	PlatformLinkedIn  Platform = "linkedin"
	PlatformInstagram Platform = "instagram"
	PlatformFacebook  Platform = "facebook"
	PlatformTikTok    Platform = "tiktok"
	PlatformYouTube   Platform = "youtube"
	PlatformReddit    Platform = "reddit"
	PlatformThreads   Platform = "threads"
	PlatformBluesky   Platform = "bluesky"
	PlatformMastodon  Platform = "mastodon"
	PlatformPinterest Platform = "pinterest"
)

// Post represents a social media post.
type Post struct {
	ID          string            `json:"id"`
	Content     string            `json:"content"`
	MediaURLs   []string          `json:"media_urls,omitempty"`
	Platforms   []Platform        `json:"platforms"`
	Tags        []string          `json:"tags,omitempty"`
	Status      PostStatus        `json:"status"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	PublishedAt *time.Time        `json:"published_at,omitempty"`
	AgentID     string            `json:"agent_id"`
	TeamID      string            `json:"team_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type PostStatus string

const (
	PostDraft     PostStatus = "draft"
	PostScheduled PostStatus = "scheduled"
	PostPublished PostStatus = "published"
	PostFailed    PostStatus = "failed"
)

// AutoPost is a recurring posting rule.
type AutoPost struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Source      string     `json:"source"`       // rss, webhook, manual
	SourceURL   string     `json:"source_url,omitempty"`
	Platforms   []Platform `json:"platforms"`
	Schedule    string     `json:"schedule"`      // cron expression
	Template    string     `json:"template,omitempty"` // Go template for content
	Active      bool       `json:"active"`
	AgentID     string     `json:"agent_id"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Integration represents a connected social media account.
type Integration struct {
	ID           string   `json:"id"`
	Platform     Platform `json:"platform"`
	AccountName  string   `json:"account_name"`
	AccountID    string   `json:"account_id"`
	AccessToken  string   `json:"-"` // never expose
	RefreshToken string   `json:"-"`
	TokenExpiry  *time.Time `json:"token_expiry,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	AgentID      string   `json:"agent_id"`
	Active       bool     `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
}

// PostResult holds the outcome of publishing to a platform.
type PostResult struct {
	Platform  Platform `json:"platform"`
	Success   bool     `json:"success"`
	PostURL   string   `json:"post_url,omitempty"`
	PostID    string   `json:"post_id,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Analytics holds post performance metrics.
type Analytics struct {
	PostID      string `json:"post_id"`
	Platform    Platform `json:"platform"`
	Impressions int    `json:"impressions"`
	Likes       int    `json:"likes"`
	Shares      int    `json:"shares"`
	Comments    int    `json:"comments"`
	Clicks      int    `json:"clicks"`
	FetchedAt   time.Time `json:"fetched_at"`
}
