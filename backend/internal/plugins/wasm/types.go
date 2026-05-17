// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package wasm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qorvenai/qorven/internal/tools"
)

// ToolDescriptor is metadata supplied at plugin registration. It
// shapes how the Wasm-backed tool appears in the tools.Registry and
// in the LLM tool list. A plugin author (human or AI) provides this
// alongside the .wasm binary; the gateway bootstrap passes it to
// Host.Register.
type ToolDescriptor struct {
	// Name is the tool's canonical id — must match the existing
	// naming convention (lowercase_snake). Shows up in the LLM
	// tool manifest.
	Name string `json:"name"`

	// Description is a one-sentence summary for the LLM.
	Description string `json:"description"`

	// Parameters is the JSON Schema for the tool's args. Matches
	// the tools.Tool.Parameters() shape exactly.
	Parameters map[string]any `json:"parameters"`
}

// BridgeTool wraps a Host + plugin name as a standard tools.Tool so
// the existing Registry, permission gate, and destructive-manifest
// machinery all work unchanged on Wasm plugins.
//
// A destructive Wasm tool MUST still be wrapped with
// permissions.WrapLazy at registration — Wasm plugins don't get a
// free pass on the permission gate. See AGENTS.md §1.2.
type BridgeTool struct {
	host    *Host
	desc    ToolDescriptor
	pluginName string
}

// NewBridgeTool constructs a tools.Tool that invokes the named Wasm
// plugin. The plugin must already be loaded via host.LoadPlugin.
func NewBridgeTool(host *Host, pluginName string, desc ToolDescriptor) *BridgeTool {
	return &BridgeTool{host: host, desc: desc, pluginName: pluginName}
}

func (b *BridgeTool) Name() string                 { return b.desc.Name }
func (b *BridgeTool) Description() string          { return b.desc.Description }
func (b *BridgeTool) Parameters() map[string]any   { return b.desc.Parameters }

// Execute marshals the args to JSON, invokes the plugin, and unwraps
// the reply. Every code path produces a *tools.Result — the plugin
// runtime errors (timeout, trap, malformed JSON) become error Results
// so the LLM sees a structured "the tool failed" rather than an
// uncaught exception.
func (b *BridgeTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	payload, err := json.Marshal(args)
	if err != nil {
		return tools.ErrorResult("wasm plugin " + b.desc.Name +
			": marshal args: " + err.Error())
	}

	res, err := b.host.Invoke(ctx, b.pluginName, payload)
	if err != nil {
		// Host-side setup failure (plugin not loaded, payload too big).
		return tools.ErrorResult("wasm plugin " + b.desc.Name + ": " + err.Error())
	}

	// Guest-side error (trap, timeout, non-zero exit).
	if res.Err != nil {
		// Include stderr if the guest left diagnostics, but cap it so
		// one badly-behaved plugin can't flood the LLM context.
		stderr := string(res.Stderr)
		if len(stderr) > 512 {
			stderr = stderr[:512] + "… [truncated]"
		}
		msg := fmt.Sprintf("wasm plugin %s failed (exit=%d, %s): %v",
			b.desc.Name, res.ExitCode, res.Elapsed, res.Err)
		if stderr != "" {
			msg += "; stderr=" + stderr
		}
		return tools.ErrorResult(msg)
	}

	// Happy path: STDOUT is the reply. The guest is expected to
	// emit JSON, but we don't force a parse here — the tool runner
	// feeds the raw string to the LLM. A guest that emits non-JSON
	// is self-describing broken to the model, and that's an OK
	// signal for the LLM to back off.
	return tools.TextResult(string(res.Stdout))
}

// Compile-time assertion.
var _ tools.Tool = (*BridgeTool)(nil)
