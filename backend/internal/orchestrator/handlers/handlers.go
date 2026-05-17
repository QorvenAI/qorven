// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package handlers wires concrete node-kind implementations into the
// graph runtime. Phase 2 ships three handlers:
//
//   - Planner: invokes the Prime agent to produce a structured plan,
//     persists it into plan_nodes.artifacts.
//   - HumanFeedback: requests (and waits on) an approval for the current
//     plan; pauses the run until resolved.
//   - AgentTask: delegates the work to a specialist agent via the
//     existing agent loop.
//
// Each handler is a thin adapter between the graph runtime's
// HandlerContext and the gateway's agent / approvals / session layers.
// Handlers are stateless; all state lives in the plans and approvals
// stores.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator/graph"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/tools"
)

// PlannerInputs is the JSON shape written to plan_nodes.inputs for
// planner nodes.
type PlannerInputs struct {
	Description string `json:"description"`
	Stack       string `json:"stack,omitempty"`
	AgentID     string `json:"agent_id,omitempty"` // usually "prime"
	SessionID   string `json:"session_id,omitempty"`
}

// AgentRunner is the minimal interface the planner + agent_task
// handlers depend on. Satisfied by *agent.Loop.
type AgentRunner interface {
	Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error)
}

// TenantToolResolver returns the tenant-scoped dynamic tools that
// should be appended to the LLM's tool set for a single plan run.
// Implementations must NOT mutate any shared state and MUST wrap
// every returned tool with permissions.WrapLazy — that invariant is
// enforced by the Phase 5.2 plugins.Loader. A resolver that returns
// un-gated tools is a security regression.
//
// Nil resolver = no dynamic tools; handlers run unchanged.
type TenantToolResolver interface {
	ToolsForTenant(ctx context.Context, tenantID string) ([]tools.Tool, error)
}

// Config wires application-level services into the handlers.
type Config struct {
	Agent AgentRunner

	// Tools, when non-nil, is consulted before every agent run by
	// both Planner and AgentTask. The resolved tools are merged into
	// RunRequest.ExtraTools for that invocation only. Phase 5.3 uses
	// this seam to surface tenant Wasm plugins to the LLM.
	Tools TenantToolResolver
}

// Planner builds a plan handler that invokes the Prime agent.
// The handler expects node.Inputs to be a JSON-encoded PlannerInputs.
// On success the node's artifacts carry the raw planner output and
// the parsed JSON plan under "plan".
func Planner(cfg Config) graph.Handler {
	return func(ctx context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
		if cfg.Agent == nil {
			return graph.OutcomeError, nil, errors.New("planner: agent runner not configured")
		}
		var in PlannerInputs
		if err := json.Unmarshal(h.Node.Inputs, &in); err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("planner: decode inputs: %w", err)
		}
		if in.Description == "" {
			return graph.OutcomeError, nil, errors.New("planner: description required")
		}
		agentID := in.AgentID
		if agentID == "" {
			agentID = "prime"
		}
		sessionID := in.SessionID
		if sessionID == "" {
			sessionID = h.Plan.SessionID
		}

		prompt := buildPlannerPrompt(in)

		// resolve the plan's tenant-scoped tools (Wasm
		// plugins). Errors are logged-and-continue — a failing
		// resolver must not block the plan. Individual plugin-level
		// failures are already silently skipped inside the loader.
		extras := resolveExtraTools(ctx, cfg.Tools, h.Plan.TenantID)

		var plannerText []byte
		req := agent.RunRequest{
			AgentID:     agentID,
			SessionID:   sessionID,
			UserMessage: prompt,
			Channel:     "plan_graph",
			Stream:      true,
			NoPersist:   true,
			ExtraTools:  extras,
			TenantID:    h.Plan.TenantID,
		}
		_, err := cfg.Agent.Run(ctx, req, func(ev agent.StreamEvent) {
			switch ev.Type {
			case "text_delta":
				if ev.Delta != "" {
					plannerText = append(plannerText, []byte(ev.Delta)...)
				}
			}
		})
		if err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("planner: agent run: %w", err)
		}
		if len(plannerText) == 0 {
			return graph.OutcomeError, nil, errors.New("planner: empty agent output")
		}

		// Extract JSON from the agent's output. We accept either a bare
		// JSON object or prose containing one (the existing project
		// pipeline does the latter).
		raw := string(plannerText)
		objStart, objEnd := findJSONBoundaries(raw)
		if objStart < 0 || objEnd < 0 {
			return graph.OutcomeError, map[string]any{"raw": raw},
				fmt.Errorf("planner: no JSON object found in agent output")
		}
		var parsed map[string]any
		jsonPart := raw[objStart : objEnd+1]
		if err := json.Unmarshal([]byte(jsonPart), &parsed); err != nil {
			return graph.OutcomeError, map[string]any{"raw": raw, "json_fragment": jsonPart},
				fmt.Errorf("planner: parse JSON: %w", err)
		}
		return graph.OutcomeSuccess, map[string]any{
			"raw":  raw,
			"json": jsonPart,
			"plan": parsed,
		}, nil
	}
}

// HumanFeedback builds the approval-gate handler. It requests the
// approval (idempotent), re-reads its state, and returns pause/approved/
// rejected/revision outcomes accordingly.
func HumanFeedback() graph.Handler {
	return func(ctx context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
		if h.Approvals == nil {
			return graph.OutcomeError, nil, errors.New("human_feedback: approvals store not configured")
		}
		appr, err := h.Approvals.Request(ctx, h.Plan.ID, h.Node.ID, "orchestrator", nil)
		if err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("human_feedback: request: %w", err)
		}
		// Re-read to tolerate race with an external resolver.
		fresh, err := h.Approvals.Get(ctx, appr.ID)
		if err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("human_feedback: reread: %w", err)
		}
		switch fresh.State {
		case approvals.StatePending:
			return "", nil, &graph.PauseSignal{
				Reason:   "awaiting approval",
				Metadata: map[string]any{"approval_id": fresh.ID},
			}
		case approvals.StateApproved:
			return graph.OutcomeApproved, map[string]any{
				"approval":    "approved",
				"approval_id": fresh.ID,
			}, nil
		case approvals.StateRejected:
			return graph.OutcomeRejected, map[string]any{
				"approval":    "rejected",
				"approval_id": fresh.ID,
			}, nil
		case approvals.StateRevisionRequested:
			return graph.OutcomeRevision, map[string]any{
				"approval":    "revision_requested",
				"approval_id": fresh.ID,
			}, nil
		}
		return graph.OutcomeError, nil, fmt.Errorf("human_feedback: unexpected state %q", fresh.State)
	}
}

// AgentTaskInputs is the JSON shape written to plan_nodes.inputs for
// agent_task nodes.
type AgentTaskInputs struct {
	AgentID     string `json:"agent_id"`     // soul key to invoke
	SessionID   string `json:"session_id"`
	Instruction string `json:"instruction"`  // the task prompt
}

// AgentTask builds the specialist-agent handler. It runs the soul with
// the node's instruction prompt and captures the response text into
// artifacts.
func AgentTask(cfg Config) graph.Handler {
	return func(ctx context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
		if cfg.Agent == nil {
			return graph.OutcomeError, nil, errors.New("agent_task: agent runner not configured")
		}
		var in AgentTaskInputs
		if err := json.Unmarshal(h.Node.Inputs, &in); err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("agent_task: decode inputs: %w", err)
		}
		if in.AgentID == "" {
			in.AgentID = h.Node.AssigneeSoul
		}
		if in.AgentID == "" {
			return graph.OutcomeError, nil, errors.New("agent_task: agent_id (or assignee_soul) required")
		}
		if in.Instruction == "" {
			return graph.OutcomeError, nil, errors.New("agent_task: instruction required")
		}
		sessionID := in.SessionID
		if sessionID == "" {
			sessionID = h.Plan.SessionID
		}

		// inject tenant-scoped tools (Wasm plugins).
		extras := resolveExtraTools(ctx, cfg.Tools, h.Plan.TenantID)

		var output []byte
		_, err := cfg.Agent.Run(ctx, agent.RunRequest{
			AgentID:     in.AgentID,
			SessionID:   sessionID,
			UserMessage: in.Instruction,
			Channel:     "plan_graph",
			Stream:      true,
			NoPersist:   true,
			ExtraTools:  extras,
			TenantID:    h.Plan.TenantID,
		}, func(ev agent.StreamEvent) {
			switch ev.Type {
			case "text_delta":
				if ev.Delta != "" {
					output = append(output, []byte(ev.Delta)...)
				}
			case "tool_start":
				h.Emit("tool_start", map[string]any{"tool": ev.Tool, "args": ev.Args})
			case "tool_result", "tool_end":
				h.Emit("tool_end", map[string]any{"tool": ev.Tool, "ok": ev.Error == ""})
			}
		})
		if err != nil {
			return graph.OutcomeError, nil, fmt.Errorf("agent_task: run: %w", err)
		}
		return graph.OutcomeSuccess, map[string]any{
			"output":   string(output),
			"agent_id": in.AgentID,
		}, nil
	}
}

// resolveExtraTools centralizes the cfg.Tools lookup. Returns nil on
// any failure or when the resolver is absent — the caller feeds this
// straight into RunRequest.ExtraTools so nil = no-op.
//
// Logging is intentionally slog.Debug, not Warn: a tenant with no
// plugins is the common case and shouldn't spam the gateway log on
// every plan run.
func resolveExtraTools(ctx context.Context, resolver TenantToolResolver, tenantID string) []tools.Tool {
	if resolver == nil || tenantID == "" {
		return nil
	}
	out, err := resolver.ToolsForTenant(ctx, tenantID)
	if err != nil {
		slog.Warn("planner.resolve_extra_tools: failed", "tenant", tenantID, "err", err)
		return nil
	}
	return out
}

// RegisterAll installs Planner, HumanFeedback, and AgentTask on the
// given registry. Call this once at boot.
func RegisterAll(reg *graph.Registry, cfg Config) *graph.Registry {
	reg.Register(plans.KindPlanner, Planner(cfg))
	reg.Register(plans.KindHumanFeedback, HumanFeedback())
	reg.Register(plans.KindAgentTask, AgentTask(cfg))
	return reg
}

// buildPlannerPrompt composes the prompt sent to Prime. Kept separate
// so tests can assert the shape.
func buildPlannerPrompt(in PlannerInputs) string {
	stack := in.Stack
	if stack == "" {
		stack = "unspecified; choose appropriate stack"
	}
	return fmt.Sprintf(`You are Prime, the planner for a Qorven build.
Read the user's description below and produce a structured plan.
Return a SINGLE JSON object with these keys:
  - stack: technology choices (e.g. "nextjs + prisma + postgres")
  - summary: 1-3 sentence synopsis
  - agents: array of {role, tasks[]} — specialist agents to spawn
  - files: array of expected file paths
  - github_repo_name: kebab-case repo name

Description:
%s

Target stack: %s

Respond with the JSON object. Prose before or after is acceptable but
the JSON must be parseable.`, in.Description, stack)
}

// findJSONBoundaries returns the [start, end] inclusive index of the
// first top-level JSON object in s, or (-1, -1) if none. It handles
// nested braces and string literals correctly enough for LLM output.
func findJSONBoundaries(s string) (int, int) {
	start := -1
	depth := 0
	inStr := false
	esc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if start < 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				return start, i
			}
		}
	}
	return -1, -1
}
