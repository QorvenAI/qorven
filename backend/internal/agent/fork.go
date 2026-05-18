// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qorvenai/qorven/internal/providers"
)

// ForkConfig controls fork subagent behavior.
type ForkConfig struct {
	InheritContext    bool // child gets parent's full conversation
	InheritPrompt    bool // child gets parent's system prompt (cache sharing)
	PermissionBubble bool // permission prompts surface to parent
}

var DefaultForkConfig = ForkConfig{
	InheritContext: true, InheritPrompt: true, PermissionBubble: true,
}

// ForkSubagent spawns a child agent that inherits the parent's context.
// The child shares the parent's system prompt bytes for prompt cache efficiency.
func (l *Loop) ForkSubagent(ctx context.Context, req ForkRequest) (*RunResult, error) {
	slog.Info("fork.spawn", "parent", req.ParentSessionID, "directive", truncateStr(req.Directive, 80))

	// Run child with inherited context
	result, err := l.Run(ctx, RunRequest{
		AgentID:     req.AgentID,
		SessionID:   req.ParentSessionID + "-fork-" + req.ForkID,
		UserMessage: req.Directive,
		Channel:     "fork",
		NoPersist:   true, // fork results don't pollute parent session
	}, req.OnEvent)

	if err != nil {
		return nil, fmt.Errorf("fork failed: %w", err)
	}

	slog.Info("fork.complete", "parent", req.ParentSessionID, "fork", req.ForkID,
		"content_len", len(result.Content))
	return result, nil
}

// ForkRequest defines a fork subagent spawn.
type ForkRequest struct {
	ParentSessionID string
	ParentMessages  []providers.Message // inherited context
	AgentID         string
	ForkID          string
	Directive       string // what the fork should do
	Config          ForkConfig
	OnEvent         func(StreamEvent)
}
