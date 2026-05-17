// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestDiscordChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*DiscordChannel)(nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{AgentID: "a1", BotToken: "token123"}
	if cfg.AgentID != "a1" { t.Error("wrong agent") }
	if cfg.BotToken != "token123" { t.Error("wrong token") }
}

func TestDiscordChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "fake"}, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "discord" { t.Errorf("type=%q want discord", ch.Type()) }
	if ch.AgentID() != "a1" { t.Errorf("agentID=%q", ch.AgentID()) }
	if !strings.Contains(ch.Name(), "discord") { t.Errorf("name=%q should contain discord", ch.Name()) }
	if ch.IsRunning() { t.Error("should not be running before Start") }
}

func TestDiscordChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "fake"}, nil)
	ctx := context.Background()
	_ = ch.Start(ctx) // may fail with fake token — expected
	ch.Stop(ctx)
	if ch.IsRunning() { t.Error("should not be running after Stop") }
}

func TestDiscordChannel_RequireMention_Nil(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t"}, nil)
	if ch.cfg.RequireMention != nil { t.Error("RequireMention should default to nil") }
}

func TestDiscordChannel_RequireMention_True(t *testing.T) {
	v := true
	ch := New(Config{AgentID: "a1", BotToken: "t", RequireMention: &v}, nil)
	if ch.cfg.RequireMention == nil || !*ch.cfg.RequireMention {
		t.Error("RequireMention should be true")
	}
}

func TestDiscordChannel_AllowedChannels(t *testing.T) {
	ch := New(Config{
		AgentID:  "a1",
		BotToken: "t",
	}, nil)
	// AllowedChannels is not in the struct — just ensure no panic on basic config
	_ = ch
}

func TestDiscordChannel_GuildID(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", GuildID: "guild123"}, nil)
	if ch.cfg.GuildID != "guild123" { t.Errorf("GuildID=%q", ch.cfg.GuildID) }
}

func TestDiscordChannel_AllowFrom(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", AllowFrom: []string{"user1", "user2"}}, nil)
	if len(ch.cfg.AllowFrom) != 2 { t.Errorf("AllowFrom=%v", ch.cfg.AllowFrom) }
}

func TestDiscordChannel_DMPolicy(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", DMPolicy: "open"}, nil)
	if ch.cfg.DMPolicy != "open" { t.Errorf("DMPolicy=%q", ch.cfg.DMPolicy) }
}

func TestDiscordChannel_HistoryLimit(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t", HistoryLimit: 25}, nil)
	if ch.cfg.HistoryLimit != 25 { t.Errorf("HistoryLimit=%d", ch.cfg.HistoryLimit) }
}

func TestDiscordChannel_NilHandler(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "t"}, nil)
	if ch == nil { t.Fatal("nil with nil handler") }
}
