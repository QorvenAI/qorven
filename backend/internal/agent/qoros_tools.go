// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// --- Sleep Tool ---

type sleepTool struct {
	getQoros func(string) *QorosMode
}

func NewSleepTool(getQoros func(string) *QorosMode) tools.Tool {
	return &sleepTool{getQoros: getQoros}
}

func (t *sleepTool) Name() string        { return "sleep" }
func (t *sleepTool) Description() string {
	return "Pause the proactive tick loop. Use when idle to save API calls. Max 300s."
}
func (t *sleepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"duration_seconds": map[string]any{"type": "integer", "description": "Seconds to sleep (max 300)"},
			"reason":          map[string]any{"type": "string", "description": "Why sleeping"},
		},
		"required": []string{"duration_seconds"},
	}
}
func (t *sleepTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	agentID := tools.AgentIDFromCtx(ctx)
	k := t.getQoros(agentID)
	if k == nil {
		return tools.TextResult("QOROS not active")
	}
	dur := 60
	if v, ok := args["duration_seconds"].(float64); ok {
		dur = int(v)
	}
	reason, _ := args["reason"].(string)
	result := k.HandleSleepTool(SleepToolParams{DurationSeconds: dur, Reason: reason})
	return tools.TextResult(result)
}

// --- Daily Log Tool ---

type dailyLogTool struct {
	getQoros func(string) *QorosMode
}

func NewDailyLogTool(getQoros func(string) *QorosMode) tools.Tool {
	return &dailyLogTool{getQoros: getQoros}
}

func (t *dailyLogTool) Name() string        { return "daily_log" }
func (t *dailyLogTool) Description() string {
	return "Append an entry to today's daily log. Append-only, never rewrite."
}
func (t *dailyLogTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"entry": map[string]any{"type": "string", "description": "Log entry to append"},
		},
		"required": []string{"entry"},
	}
}
func (t *dailyLogTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	agentID := tools.AgentIDFromCtx(ctx)
	k := t.getQoros(agentID)
	if k == nil {
		return tools.TextResult("QOROS not active")
	}
	entry, _ := args["entry"].(string)
	if err := k.AppendDailyLog(entry); err != nil {
		return tools.ErrorResult(fmt.Sprintf("Log failed: %v", err))
	}
	return tools.TextResult(fmt.Sprintf("Logged at %s", time.Now().Format("15:04:05")))
}
