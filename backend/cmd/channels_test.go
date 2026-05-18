// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"strings"
	"testing"
)

func TestChannelSetupGuides_AllChannelTypes(t *testing.T) {
	allTypes := []string{
		"telegram", "discord", "slack", "whatsapp", "email", "sms",
		"teams", "github", "webchat", "webhook",
		"signal", "imessage", "facebook", "line", "zalo",
		"feishu", "dingtalk", "wecom", "matrix", "mattermost",
	}
	for _, ct := range allTypes {
		guide, ok := channelSetupGuides[ct]
		if !ok {
			t.Errorf("channelSetupGuides missing entry for %q", ct)
			continue
		}
		if strings.TrimSpace(guide) == "" {
			t.Errorf("channelSetupGuides[%q] is empty", ct)
		}
	}
}

func TestChannelOfficialLinks_AllChannelTypes(t *testing.T) {
	allTypes := []string{
		"telegram", "discord", "slack", "whatsapp", "email", "sms",
		"teams", "github", "webchat", "webhook",
		"signal", "imessage", "facebook", "line", "zalo",
		"feishu", "dingtalk", "wecom", "matrix", "mattermost",
	}
	for _, ct := range allTypes {
		// officialLink can be empty (e.g. webchat, webhook) but key must exist
		_, ok := channelOfficialLinks[ct]
		if !ok {
			t.Errorf("channelOfficialLinks missing entry for %q", ct)
		}
	}
}
