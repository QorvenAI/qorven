// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	cronpkg "github.com/qorvenai/qorven/internal/cron"
)

// startWorkflowCron polls enabled cron-trigger workflows and fires those due.
// Reuses the cron expression parser; tracks each workflow's next-run in memory.
func (gw *Gateway) startWorkflowCron(ctx context.Context) {
	if gw.wfExecutor == nil || gw.wfStore == nil {
		return
	}
	next := map[string]time.Time{}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				gw.tickWorkflowCron(ctx, next)
			}
		}
	}()
	slog.Info("workflow cron poller started")
}

func (gw *Gateway) tickWorkflowCron(ctx context.Context, next map[string]time.Time) {
	wfs, err := gw.wfStore.List(ctx, defaultTenant)
	if err != nil {
		return
	}
	now := time.Now()
	for i := range wfs {
		wf := wfs[i] // copy so the goroutine below captures a stable value
		if !wf.Enabled || wf.TriggerType != "cron" {
			continue
		}
		expr := workflowCronExpr(wf.TriggerConfig)
		if expr == "" || !cronpkg.IsValidExpr(expr) {
			continue
		}
		nr, seen := next[wf.ID]
		if !seen {
			// First sighting: schedule forward, don't fire immediately on boot.
			next[wf.ID] = cronpkg.NextRunFromExpr(expr)
			continue
		}
		if now.After(nr) {
			wfRun := wf // capture for the goroutine — avoids loop-variable capture
			go func() {
				if _, runErr := gw.wfExecutor.Run(context.Background(), &wfRun, nil); runErr != nil {
					slog.Warn("workflow.cron.run_failed", "workflow", wfRun.ID, "error", runErr)
				}
			}()
			next[wf.ID] = cronpkg.NextRunFromExpr(expr)
			slog.Info("workflow.cron.fired", "workflow", wf.ID, "name", wf.Name)
		}
	}
}

// validateWorkflowCron returns an error message if trigger_type=="cron" but the
// expression is missing or invalid, empty string otherwise.  Centralised here so
// both handleCreateWorkflow and handleUpdateWorkflow share the same check.
func validateWorkflowCron(triggerType string, triggerConfig json.RawMessage) string {
	if triggerType != "cron" {
		return ""
	}
	expr := workflowCronExpr(triggerConfig)
	if expr == "" || !cronpkg.IsValidExpr(expr) {
		return "cron trigger requires a valid cron expression in trigger_config"
	}
	return ""
}

// workflowCronExpr extracts the cron expression from a workflow's trigger_config.
// It checks a set of common key names so callers can use any convention.
func workflowCronExpr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	for _, k := range []string{"cron", "expression", "cron_expression", "schedule"} {
		if s, ok := cfg[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
