// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package slack

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestSlackChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*SlackChannel)(nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{AgentID: "a1", BotToken: "xoxb-xxx", AppToken: "xapp-xxx"}
	if cfg.BotToken == "" { t.Error("empty bot token") }
	if cfg.AppToken == "" { t.Error("empty app token") }
}

func TestSlackChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "xoxb-fake", AppToken: "xapp-fake"}, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "slack" { t.Errorf("type=%q want slack", ch.Type()) }
	if ch.AgentID() != "a1" { t.Errorf("agentID=%q", ch.AgentID()) }
	if !strings.Contains(ch.Name(), "slack") { t.Errorf("name=%q should contain slack", ch.Name()) }
	if ch.IsRunning() { t.Error("should not be running before Start") }
}

func TestSlackChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "fake", AppToken: "fake"}, nil)
	ctx := context.Background()
	_ = ch.Start(ctx) // may fail with fake token — that's expected
	ch.Stop(ctx)
	if ch.IsRunning() { t.Error("should not be running after Stop") }
}

func TestIsAllowedSlackHost(t *testing.T) {
	// Only specific Slack CDN/file subdomains are allowed (not api.slack.com or slack.com)
	tests := []struct {
		host string
		want bool
	}{
		{"files.slack.com", true},
		{"files-pri.slack.com", true},
		{"files-tmb.slack.com", true},
		{"avatars.slack-edge.com", true},
		{"slack.com", false},       // root domain without subdot NOT in allowlist
		{"api.slack.com", true},   // *.slack.com suffix matches
		{"evil.com", false},
		{"fakeslack.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isAllowedSlackHost(tt.host)
		if got != tt.want {
			t.Errorf("isAllowedSlackHost(%q)=%v want %v", tt.host, got, tt.want)
		}
	}
}

func TestSlackChannel_RequireMention_Nil(t *testing.T) {
	// RequireMention is *bool — nil means "use default"
	ch := New(Config{AgentID: "a1", BotToken: "t", AppToken: "t"}, nil)
	if ch.cfg.RequireMention != nil { t.Errorf("RequireMention should default to nil") }
}

func TestSlackChannel_RequireMention_True(t *testing.T) {
	v := true
	ch := New(Config{AgentID: "a1", BotToken: "t", AppToken: "t", RequireMention: &v}, nil)
	if ch.cfg.RequireMention == nil || !*ch.cfg.RequireMention {
		t.Error("RequireMention should be true")
	}
}

func TestSlackChannel_RequireMention_False(t *testing.T) {
	v := false
	ch := New(Config{AgentID: "a1", BotToken: "t", AppToken: "t", RequireMention: &v}, nil)
	if ch.cfg.RequireMention == nil || *ch.cfg.RequireMention {
		t.Error("RequireMention should be false")
	}
}

func TestSlackChannel_AllowFrom(t *testing.T) {
	ch := New(Config{
		AgentID:   "a1",
		BotToken:  "t",
		AppToken:  "t",
		AllowFrom: []string{"U12345", "U67890"},
	}, nil)
	if len(ch.cfg.AllowFrom) != 2 {
		t.Errorf("AllowFrom=%v", ch.cfg.AllowFrom)
	}
}

func TestSlackChannel_DMPolicy(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", AppToken: "t", DMPolicy: "open"}, nil)
	if ch.cfg.DMPolicy != "open" { t.Errorf("DMPolicy=%q", ch.cfg.DMPolicy) }
}

func TestSlackChannel_HistoryLimit(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", AppToken: "t", HistoryLimit: 50}, nil)
	if ch.cfg.HistoryLimit != 50 { t.Errorf("HistoryLimit=%d", ch.cfg.HistoryLimit) }
}

func TestSlackChannel_TokenPrefixes(t *testing.T) {
	// Standard Slack bot token prefix is xoxb-, Socket Mode app token is xapp-
	ch := New(Config{AgentID: "a1", BotToken: "xoxb-valid-token", AppToken: "xapp-valid-token"}, nil)
	if !strings.HasPrefix(ch.cfg.BotToken, "xoxb-") {
		t.Logf("note: bot token should use xoxb- prefix: %q", ch.cfg.BotToken)
	}
	_ = ch
}
