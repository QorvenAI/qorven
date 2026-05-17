// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package agent

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/security/promptguard"
)

// PromptInjectionPolicy is how the agent should react when promptguard
// flags an input. Callers wire a policy via Loop.SetPromptGuard.
type PromptInjectionPolicy int

const (
	// PromptGuardOff — no scanning. The default.
	PromptGuardOff PromptInjectionPolicy = iota

	// PromptGuardWarn — scan and log; let the message through.
	// Good for pilot deployments where you want to observe false-
	// positive rates before enabling blocking.
	PromptGuardWarn

	// PromptGuardBlock — refuse any input flagged Likely+ (>= 0.6).
	// The user gets a short refusal and the audit log records the
	// reason. Recommended default for production.
	PromptGuardBlock

	// PromptGuardStrict — refuse any input flagged Suspicious+
	// (>= 0.3). Higher false-positive rate in exchange for tighter
	// security. Use only when handling sensitive downstream tools
	// (financial transactions, destructive ops, etc).
	PromptGuardStrict
)

// PromptGuardResult is what Loop.RunPromptGuard returns. If `Block`
// is true, the loop halts and returns `UserMessage` verbatim as the
// agent's reply; the LLM is not called. Otherwise the loop continues,
// but the detections are recorded in audit.
type PromptGuardResult struct {
	Block       bool
	UserMessage string // the pre-canned refusal to show the user
	Report      *promptguard.Report
}

// scanPromptInjection applies the active policy. Returns (nil, nil)
// when off or clean — zero overhead beyond one regex pass.
func (l *Loop) scanPromptInjection(text string) *PromptGuardResult {
	if l.PromptGuardPolicy == PromptGuardOff || strings.TrimSpace(text) == "" {
		return nil
	}
	r := promptguard.Scan(text)
	if r.Score == 0 {
		return nil
	}

	// Always log at the right level so ops can observe the false-
	// positive rate before tightening the policy.
	topRule := ""
	if len(r.Detections) > 0 {
		topRule = r.Detections[0].Rule
	}
	switch l.PromptGuardPolicy {
	case PromptGuardWarn:
		slog.Info("promptguard.warn",
			"score", r.Score, "top_rule", topRule, "count", len(r.Detections))
		return &PromptGuardResult{Block: false, Report: r}

	case PromptGuardBlock:
		if r.Likely() {
			slog.Warn("promptguard.blocked",
				"score", r.Score, "top_rule", topRule, "count", len(r.Detections))
			return &PromptGuardResult{
				Block:       true,
				UserMessage: refusalMessage(r),
				Report:      r,
			}
		}
		if r.Suspicious() {
			slog.Info("promptguard.suspicious",
				"score", r.Score, "top_rule", topRule)
		}
		return &PromptGuardResult{Block: false, Report: r}

	case PromptGuardStrict:
		if r.Suspicious() {
			slog.Warn("promptguard.blocked_strict",
				"score", r.Score, "top_rule", topRule, "count", len(r.Detections))
			return &PromptGuardResult{
				Block:       true,
				UserMessage: refusalMessage(r),
				Report:      r,
			}
		}
		return &PromptGuardResult{Block: false, Report: r}
	}
	return nil
}

// refusalMessage formats a short, non-leaky message for the user.
// We deliberately DON'T echo the attacker's text or the rule name —
// that would teach them which phrases tripped the filter and let
// them iterate around it.
func refusalMessage(r *promptguard.Report) string {
	category := "input"
	if len(r.Detections) > 0 {
		switch r.Detections[0].Category {
		case "override", "role_injection":
			category = "instruction override"
		case "exfil":
			category = "credential disclosure"
		case "jailbreak":
			category = "jailbreak"
		case "encoded":
			category = "encoded payload"
		}
	}
	return fmt.Sprintf(
		"I can't process that message — it contains content that looks like a %s attempt. "+
			"If you believe this is a mistake, please rephrase your request.",
		category)
}

// SetPromptGuardPolicy wires a new policy. Safe to call before Run;
// not safe to call concurrently from multiple goroutines.
func (l *Loop) SetPromptGuardPolicy(p PromptInjectionPolicy) { l.PromptGuardPolicy = p }
