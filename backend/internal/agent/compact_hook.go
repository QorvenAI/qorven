// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"time"
)

type AutoCompactHook struct {
	loop  *Loop
	turns map[string]int
}

func NewAutoCompactHook(l *Loop) *AutoCompactHook {
	return &AutoCompactHook{loop: l, turns: make(map[string]int)}
}
func (h *AutoCompactHook) Name() string { return "auto_compact" }
func (h *AutoCompactHook) PreRun(_ context.Context, _ *RunRequest) error { return nil }
func (h *AutoCompactHook) PostRun(ctx context.Context, req *RunRequest, _ *RunResult, _ time.Duration) error {
	if req.NoPersist { return nil }
	h.turns[req.SessionID]++
	if h.turns[req.SessionID]%10 == 0 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			history := h.loop.loadHistory(bgCtx, req.SessionID)
			h.loop.ExtractSessionMemory(bgCtx, req.SessionID, history)
			slog.Info("auto_compact.session_memory", "session", req.SessionID)
		}()
	}
	return nil
}
