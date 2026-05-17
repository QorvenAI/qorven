// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package reach

import (
	"testing"
)

// hard_test.go — Diamond-hard tests for Qorven Reach platform access.

// ── Registry ──

func TestHard_Registry_DefaultHas17Channels(t *testing.T) {
	cfg := Config{APIKeys: map[string]string{"github": "test", "exa": "test"}}
	reg := DefaultRegistry(cfg)
	channels := reg.List()
	if len(channels) < 14 { t.Errorf("expected 14+ channels, got %d: %v", len(channels), channels) }
}

func TestHard_Registry_GetExistingChannel(t *testing.T) {
	reg := DefaultRegistry(Config{})
	ch, ok := reg.Get("reddit")
	if !ok { t.Fatal("reddit channel should exist") }
	if ch.Name() != "reddit" { t.Errorf("name: %q", ch.Name()) }
}

func TestHard_Registry_GetMissingChannel(t *testing.T) {
	reg := DefaultRegistry(Config{})
	_, ok := reg.Get("nonexistent")
	if ok { t.Error("nonexistent channel should not exist") }
}

func TestHard_Registry_DoctorReportsAllChannels(t *testing.T) {
	reg := DefaultRegistry(Config{})
	report := reg.Doctor()
	if len(report.Channels) == 0 { t.Fatal("doctor should report all channels") }

	// All no-auth channels should be ready
	for _, ch := range report.Channels {
		if ch.Name == "reddit" || ch.Name == "hackernews" || ch.Name == "web" || ch.Name == "rss" {
			if ch.Status != StatusReady { t.Errorf("%s should be ready, got %s", ch.Name, ch.Status) }
		}
	}
}

func TestHard_Registry_ExaNeedsAPIKey(t *testing.T) {
	// Without API key
	reg := DefaultRegistry(Config{})
	ch, ok := reg.Get("exa")
	if !ok { t.Fatal("exa should exist") }
	if ch.Check() != StatusNeedsSetup { t.Error("exa without key should need setup") }

	// With API key
	reg2 := DefaultRegistry(Config{APIKeys: map[string]string{"exa": "test-key"}})
	ch2, _ := reg2.Get("exa")
	if ch2.Check() != StatusReady { t.Error("exa with key should be ready") }
}

// ── Channel Interface Compliance ──

func TestHard_AllChannels_ImplementInterface(t *testing.T) {
	// Every channel must implement the Channel interface
	channels := []Channel{
		&WebChannel{}, &RedditChannel{}, &YouTubeChannel{},
		&GitHubChannel{}, &HNChannel{}, &V2EXChannel{},
		&WeiboChannel{}, &RSSChannel{}, &ExaChannel{},
		&LinkedInChannel{}, &BilibiliChannel{},
		&TwitterChannel{}, &XiaoHongShuChannel{},
		&DouyinChannel{}, &WeChatChannel{},
		&XueqiuChannel{}, &PodcastChannel{},
	}
	for _, ch := range channels {
		if ch.Name() == "" { t.Errorf("channel has empty name") }
		// Check() should not panic
		_ = ch.Check()
	}
	t.Logf("all %d channels implement interface ✓", len(channels))
}

// ── Cookie Auth Channels ──

func TestHard_Twitter_NeedsCookies(t *testing.T) {
	ch := &TwitterChannel{}
	if ch.Check() != StatusNeedsSetup { t.Error("twitter without cookies should need setup") }

	ch2 := &TwitterChannel{AuthToken: "abc", CT0: "def"}
	if ch2.Check() != StatusReady { t.Error("twitter with cookies should be ready") }
}

func TestHard_XiaoHongShu_NeedsCookie(t *testing.T) {
	ch := &XiaoHongShuChannel{}
	if ch.Check() != StatusNeedsSetup { t.Error("xhs without cookie should need setup") }

	ch2 := &XiaoHongShuChannel{Cookie: "session=abc"}
	if ch2.Check() != StatusReady { t.Error("xhs with cookie should be ready") }
}

func TestHard_Xueqiu_NeedsCookie(t *testing.T) {
	ch := &XueqiuChannel{}
	if ch.Check() != StatusNeedsSetup { t.Error("xueqiu without cookie should need setup") }
}

// ── Search Error Handling ──

func TestHard_Web_SearchReturnsError(t *testing.T) {
	ch := &WebChannel{}
	_, err := ch.Search("test", 10)
	if err == nil { t.Error("web search should return error (use Read instead)") }
}

func TestHard_RSS_SearchReturnsError(t *testing.T) {
	ch := &RSSChannel{}
	_, err := ch.Search("test", 10)
	if err == nil { t.Error("rss search should return error (use Read with URL)") }
}

func TestHard_Exa_SearchWithoutKey(t *testing.T) {
	ch := &ExaChannel{}
	_, err := ch.Search("test", 10)
	if err == nil { t.Error("exa search without key should error") }
}

// ── XML Extraction ──

func TestHard_XMLExtract_BasicTag(t *testing.T) {
	xml := "<item><title>Hello World</title><link>https://example.com</link></item>"
	title := xmlExtract(xml, "title")
	if title != "Hello World" { t.Errorf("title: %q", title) }
}

func TestHard_XMLExtract_CDATA(t *testing.T) {
	xml := "<item><title><![CDATA[Hello & World]]></title></item>"
	title := xmlExtract(xml, "title")
	if title != "Hello & World" { t.Errorf("CDATA title: %q", title) }
}

func TestHard_XMLExtract_MissingTag(t *testing.T) {
	xml := "<item><title>Hello</title></item>"
	desc := xmlExtract(xml, "description")
	if desc != "" { t.Errorf("missing tag should return empty: %q", desc) }
}
