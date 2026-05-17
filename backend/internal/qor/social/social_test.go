// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package social

import (
	"testing"
	"time"
)

func TestPlatform_Constants(t *testing.T) {
	platforms := []Platform{PlatformTwitter, PlatformLinkedIn, PlatformInstagram, PlatformFacebook, PlatformTikTok, PlatformYouTube, PlatformReddit, PlatformThreads}
	seen := map[Platform]bool{}
	for _, p := range platforms {
		if p == "" { t.Error("empty platform") }
		if seen[p] { t.Errorf("duplicate: %s", p) }
		seen[p] = true
	}
}

func TestPostStatus_Constants(t *testing.T) {
	if PostDraft != "draft" { t.Error("wrong draft") }
	if PostScheduled != "scheduled" { t.Error("wrong scheduled") }
	if PostPublished != "published" { t.Error("wrong published") }
	if PostFailed != "failed" { t.Error("wrong failed") }
}

func TestPost_Fields(t *testing.T) {
	now := time.Now()
	scheduled := now.Add(24 * time.Hour)
	p := Post{
		ID: "p1", Content: "Hello world!", Platforms: []Platform{PlatformTwitter, PlatformLinkedIn},
		Tags: []string{"tech", "ai"}, Status: PostDraft, ScheduledAt: &scheduled,
		AgentID: "agent1", TeamID: "team1", CreatedAt: now,
	}
	if len(p.Platforms) != 2 { t.Error("wrong platform count") }
	if len(p.Tags) != 2 { t.Error("wrong tag count") }
	if p.ScheduledAt.Before(now) { t.Error("scheduled should be future") }
}

func TestAutoPost_Fields(t *testing.T) {
	ap := AutoPost{
		Name: "Daily digest", Source: "rss", SourceURL: "https://blog.example.com/feed",
		Platforms: []Platform{PlatformTwitter}, Schedule: "0 9 * * *", Active: true,
	}
	if ap.Source != "rss" { t.Error("wrong source") }
	if ap.Schedule != "0 9 * * *" { t.Error("wrong schedule") }
}

func TestIntegration_Fields(t *testing.T) {
	expiry := time.Now().Add(3600 * time.Second)
	i := Integration{
		Platform: PlatformTwitter, AccountName: "QorvenAI", AccountID: "12345",
		AccessToken: "secret", RefreshToken: "refresh", TokenExpiry: &expiry, Active: true,
	}
	if i.AccessToken != "secret" { t.Error("wrong token") }
	if !i.Active { t.Error("should be active") }
}

func TestPostResult_Success(t *testing.T) {
	r := PostResult{Platform: PlatformTwitter, Success: true, PostURL: "https://twitter.com/i/status/123", PostID: "123"}
	if !r.Success { t.Error("should be success") }
	if r.PostURL == "" { t.Error("missing URL") }
}

func TestPostResult_Failure(t *testing.T) {
	r := PostResult{Platform: PlatformLinkedIn, Success: false, Error: "auth expired"}
	if r.Success { t.Error("should be failure") }
	if r.Error == "" { t.Error("missing error") }
}

func TestAnalytics_Fields(t *testing.T) {
	a := Analytics{PostID: "p1", Platform: PlatformTwitter, Impressions: 1000, Likes: 50, Shares: 10, Comments: 5, Clicks: 25}
	if a.Impressions != 1000 { t.Error("wrong impressions") }
	if a.Likes+a.Shares+a.Comments+a.Clicks != 90 { t.Error("wrong engagement total") }
}

func TestPublisher_New(t *testing.T) {
	p := NewPublisher()
	if p == nil { t.Fatal("nil publisher") }
	if p.client == nil { t.Error("nil client") }
}

func TestPublisher_UnsupportedPlatform(t *testing.T) {
	p := NewPublisher()
	result, err := p.Publish(nil, "myspace", "token", "hello", nil)
	if err != nil { t.Fatal(err) }
	if result.Success { t.Error("unsupported platform should fail") }
	if result.Error == "" { t.Error("should have error message") }
}

func TestParsePlatforms_Empty(t *testing.T) {
	platforms := parsePlatforms(map[string]any{})
	if len(platforms) != 1 { t.Error("empty should default to twitter") }
	if platforms[0] != PlatformTwitter { t.Error("default should be twitter") }
}

func TestParsePlatforms_Single(t *testing.T) {
	platforms := parsePlatforms(map[string]any{"platforms": "linkedin"})
	if len(platforms) != 1 { t.Errorf("got %d", len(platforms)) }
	if platforms[0] != PlatformLinkedIn { t.Error("wrong platform") }
}

func TestParsePlatforms_Multiple(t *testing.T) {
	platforms := parsePlatforms(map[string]any{"platforms": "twitter,linkedin,facebook"})
	if len(platforms) != 3 { t.Errorf("got %d", len(platforms)) }
}

func TestTruncate_Short(t *testing.T) {
	if truncate("hello", 10) != "hello" { t.Error("short should not truncate") }
}

func TestTruncate_Long(t *testing.T) {
	result := truncate("hello world this is long", 10)
	if len(result) > 15 { t.Errorf("should truncate: %q", result) }
}

func TestSocialTool_Name(t *testing.T) {
	tool := NewSocialTool(nil)
	if tool.Name() != "qorven_social" { t.Errorf("name=%q", tool.Name()) }
}

func TestSocialTool_Description(t *testing.T) {
	tool := NewSocialTool(nil)
	if tool.Description() == "" { t.Error("empty description") }
}

func TestSocialTool_Parameters(t *testing.T) {
	tool := NewSocialTool(nil)
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok { t.Fatal("no properties") }
	if _, ok := props["action"]; !ok { t.Error("missing action") }
	if _, ok := props["content"]; !ok { t.Error("missing content") }
	if _, ok := props["platforms"]; !ok { t.Error("missing platforms") }
}
