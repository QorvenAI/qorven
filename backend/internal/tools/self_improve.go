// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SelfImprove analyzes agent logs and implements improvements.
type SelfImprove struct{ root string }

func NewSelfImprove(root string) *SelfImprove { return &SelfImprove{root: root} }
func (s *SelfImprove) Name() string           { return "self_improve" }
func (s *SelfImprove) Description() string    { return "Analyze codebase for improvements" }
func (s *SelfImprove) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":      map[string]any{"type": "string", "description": "analyze, suggest, or implement"},
		"description": map[string]any{"type": "string", "description": "what to improve"},
	}, "required": []string{"action"}}
}

func (s *SelfImprove) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	switch action {
	case "analyze":
		// Check recent git log for patterns
		log, _ := exec.CommandContext(ctx, "git", "-C", s.root, "log", "--oneline", "-50").Output()
		// Check for build warnings
		build, _ := exec.CommandContext(ctx, "go", "vet", "./...").CombinedOutput()
		// Check for TODO/FIXME
		todos, _ := exec.CommandContext(ctx, "grep", "-rn", "TODO\\|FIXME\\|HACK", s.root+"/internal/", "--include=*.go", "-c").Output()
		return TextResult(fmt.Sprintf("=== Recent Changes ===\n%s\n=== Vet Warnings ===\n%s\n=== TODOs ===\n%s",
			truncate(string(log), 1500), truncate(string(build), 1000), truncate(string(todos), 500)))
	case "suggest":
		// Run staticcheck-style analysis
		vet, _ := exec.CommandContext(ctx, "go", "vet", "./...").CombinedOutput()
		// Count test coverage
		ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		cov, _ := exec.CommandContext(ctx2, "go", "test", "-cover", "./...", "-count=1", "-timeout=30s").CombinedOutput()
		lines := strings.Split(string(cov), "\n")
		var lowCov []string
		for _, l := range lines {
			if strings.Contains(l, "coverage:") && !strings.Contains(l, "100.0%") {
				lowCov = append(lowCov, strings.TrimSpace(l))
			}
		}
		return TextResult(fmt.Sprintf("=== Vet Issues ===\n%s\n=== Low Coverage ===\n%s",
			truncate(string(vet), 1500), truncate(strings.Join(lowCov, "\n"), 1500)))
	case "implement":
		desc, _ := args["description"].(string)
		if desc == "" { return TextResult("error: description required — what to improve") }
		return TextResult("To implement: use self_patch tool with the specific file and content changes. Description: " + desc)
	default:
		return TextResult("actions: analyze (scan codebase), suggest (find improvements), implement (description)")
	}
}

func truncate(s string, max int) string {
	if len(s) > max { return s[:max] + "\n[...truncated]" }
	return s
}
