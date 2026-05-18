// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"time"
)

type PlanModeHook struct{ loop *Loop }

func NewPlanModeHook(l *Loop) *PlanModeHook { return &PlanModeHook{loop: l} }
func (h *PlanModeHook) Name() string        { return "plan_mode" }
func (h *PlanModeHook) PreRun(ctx context.Context, req *RunRequest) error {
	if req.Channel == "btw" || req.Channel == "fork" || req.NoTools { return nil }
	if ShouldEnterPlanMode(req.UserMessage) {
		slog.Info("plan_mode.auto_trigger", "session", req.SessionID)
		req.Mode = "plan"
	}
	return nil
}
func (h *PlanModeHook) PostRun(_ context.Context, _ *RunRequest, _ *RunResult, _ time.Duration) error { return nil }
