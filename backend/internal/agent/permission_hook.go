// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/permissions"
)

type PermissionHook struct {
	mode      permissions.PermissionMode
	approvals map[string]*permissions.ToolPermission
}

func NewPermissionHook(mode permissions.PermissionMode) *PermissionHook {
	return &PermissionHook{mode: mode, approvals: make(map[string]*permissions.ToolPermission)}
}
func (h *PermissionHook) Name() string { return "permissions" }
func (h *PermissionHook) PreRun(_ context.Context, req *RunRequest) error {
	if req.Mode == "plan" {
		h.mode = permissions.ModePlan
		slog.Info("permission.plan_mode", "session", req.SessionID)
	}
	return nil
}
func (h *PermissionHook) PostRun(_ context.Context, _ *RunRequest, _ *RunResult, _ time.Duration) error { return nil }
func (h *PermissionHook) CheckTool(tool string) (bool, bool) {
	return permissions.CheckPermission(tool, h.mode, h.approvals)
}
