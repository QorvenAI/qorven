// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
)

// HandleBTW processes a /btw ephemeral side question.
// Uses session context but no tools, response not persisted.
func (l *Loop) HandleBTW(ctx context.Context, agentID, sessionID, question string) (string, error) {
	question = strings.TrimPrefix(strings.TrimSpace(question), "/btw")
	question = strings.TrimSpace(question)
	if question == "" { return "Usage: /btw <question>", nil }

	result, err := l.Run(ctx, RunRequest{
		AgentID:     agentID,
		SessionID:   sessionID + "-btw",
		UserMessage: question,
		Channel:     "btw",
		NoTools:     true,
		NoPersist:   true,
	}, func(event StreamEvent) {})
	if err != nil { return "", err }
	return result.Content, nil
}
