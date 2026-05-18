// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import "testing"

func TestHard_Social_AllPlatforms(t *testing.T) {
	platforms := []Platform{PlatformTwitter, PlatformLinkedIn, PlatformInstagram, PlatformFacebook, PlatformTikTok, PlatformYouTube, PlatformReddit, PlatformThreads}
	for _, p := range platforms {
		if p == "" { t.Error("empty platform") }
	}
	if len(platforms) != 8 { t.Errorf("expected 8 platforms, got %d", len(platforms)) }
}

func TestHard_Social_PostLifecycle(t *testing.T) {
	statuses := []PostStatus{PostDraft, PostScheduled, PostPublished, PostFailed}
	for _, s := range statuses {
		p := Post{Status: s}
		if p.Status != s { t.Errorf("status=%q", p.Status) }
	}
}

func TestHard_Social_AutoPostConfig(t *testing.T) {
	ap := AutoPost{
		Name: "Daily Digest", Source: "rss", SourceURL: "https://blog.qorven.io/feed",
		Platforms: []Platform{PlatformTwitter, PlatformLinkedIn},
		Schedule: "0 9 * * *", Active: true, AgentID: "agent1",
	}
	if len(ap.Platforms) != 2 { t.Error("platforms") }
	if ap.Schedule != "0 9 * * *" { t.Error("schedule") }
}

func TestHard_Social_Publisher(t *testing.T) {
	pub := NewPublisher()
	if pub == nil { t.Fatal("nil") }
	// Unsupported platform should return error result
	result, _ := pub.Publish(nil, "myspace", "token", "hello", nil)
	if result.Success { t.Error("myspace should fail") }
	if result.Error == "" { t.Error("should have error message") }
}

func TestHard_Social_Tool(t *testing.T) {
	tool := NewSocialTool(nil)
	if tool.Name() != "qorven_social" { t.Error("name") }
	params := tool.Parameters()
	props, _ := params["properties"].(map[string]any)
	required := []string{"action", "content", "platforms"}
	for _, r := range required {
		if props[r] == nil { t.Errorf("missing param: %s", r) }
	}
}
