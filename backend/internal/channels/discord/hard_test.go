// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package discord

import "testing"

func TestHard_Discord_ChannelInterface(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "fake"}, nil)
	if ch.Name() != "discord" { t.Error("name") }
	if ch.Type() != "discord" { t.Error("type") }
	if ch.AgentID() != "a1" { t.Error("agent") }
	if ch.IsRunning() { t.Error("should not be running") }
}

func TestHard_Discord_Config(t *testing.T) {
	configs := []Config{
		{AgentID: "a1", BotToken: "token1"},
		{AgentID: "a2", BotToken: "token2"},
		{AgentID: "", BotToken: ""},
	}
	for _, cfg := range configs {
		ch := New(cfg, nil)
		if ch == nil { t.Error("nil channel") }
	}
}
