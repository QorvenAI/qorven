// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import "strings"

// Middleware transforms a message in the agent pipeline.
type Middleware func(msg string, ctx *MiddlewareContext) string

// MiddlewareContext provides metadata for middleware decisions.
type MiddlewareContext struct {
	AgentID   string
	Channel   string // "chat", "telegram", "slack", "email"
	Direction string // "inbound" or "outbound"
}

// MiddlewarePipeline runs middleware in order.
type MiddlewarePipeline struct {
	inbound  []Middleware
	outbound []Middleware
}

func NewMiddlewarePipeline() *MiddlewarePipeline { return &MiddlewarePipeline{} }

func (p *MiddlewarePipeline) AddInbound(m Middleware)  { p.inbound = append(p.inbound, m) }
func (p *MiddlewarePipeline) AddOutbound(m Middleware) { p.outbound = append(p.outbound, m) }

func (p *MiddlewarePipeline) RunInbound(msg string, ctx *MiddlewareContext) string {
	ctx.Direction = "inbound"
	for _, m := range p.inbound {
		msg = m(msg, ctx)
	}
	return msg
}

func (p *MiddlewarePipeline) RunOutbound(msg string, ctx *MiddlewareContext) string {
	ctx.Direction = "outbound"
	for _, m := range p.outbound {
		msg = m(msg, ctx)
	}
	return msg
}

// --- Built-in Middleware ---

// StripDeliveryKeywords removes "dm me", "email me", etc. from the message
// so the LLM sees clean task text.
func StripDeliveryKeywords(msg string, _ *MiddlewareContext) string {
	for _, kw := range []string{"dm me", "dm it", "dm that", "dm this",
		"via dm", "email me", "telegram me", "send it to my dm"} {
		msg = strings.ReplaceAll(strings.ToLower(msg), kw, "")
	}
	return strings.TrimSpace(msg)
}

// StripSchedulingWords removes scheduling phrases after they've been parsed.
func StripSchedulingWords(msg string, _ *MiddlewareContext) string {
	for _, kw := range []string{"every day", "every morning", "every evening",
		"daily", "weekly", "monthly"} {
		msg = strings.ReplaceAll(strings.ToLower(msg), kw, "")
	}
	return strings.TrimSpace(msg)
}

// TruncateForChannel enforces channel-specific length limits.
func TruncateForChannel(msg string, ctx *MiddlewareContext) string {
	limits := map[string]int{"telegram": 4096, "sms": 160, "slack": 40000}
	if limit, ok := limits[ctx.Channel]; ok && len(msg) > limit {
		return msg[:limit-3] + "..."
	}
	return msg
}

// SanitizeOutput strips hallucinated XML tool tags from output.
func SanitizeOutput(msg string, _ *MiddlewareContext) string {
	// Remove <tool_call>...</tool_call> and <function_call>...</function_call>
	for _, tag := range []string{"tool_call", "function_call"} {
		for {
			start := strings.Index(msg, "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(msg[start:], "</"+tag+">")
			if end < 0 {
				break
			}
			msg = msg[:start] + msg[start+end+len("</"+tag+">"):]
		}
	}
	return strings.TrimSpace(msg)
}
