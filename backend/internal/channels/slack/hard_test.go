// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package slack

import "testing"

func TestHard_Slack_ChannelInterface(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "xoxb-fake", AppToken: "xapp-fake"}, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "slack" { t.Error("type") }
}

func TestHard_Slack_AllowedHosts(t *testing.T) {
	allowed := []string{"files.slack.com", "avatars.slack-edge.com"}
	blocked := []string{"evil.com", "files.slack.com.evil.com", "slack.com.evil.com"}
	for _, h := range allowed {
		if !isAllowedSlackHost(h) { t.Errorf("%q should be allowed", h) }
	}
	for _, h := range blocked {
		if isAllowedSlackHost(h) { t.Errorf("%q should be blocked", h) }
	}
}
