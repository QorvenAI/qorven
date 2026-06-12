// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import "context"

// KBReadTool lets an agent read company/department knowledge-base documents from
// Drive, gated by Drive's access-control predicate (the agent only sees docs its
// scope permits). The Drive lookup is injected at boot (ReadKB) so this package
// does not import the drive/gateway packages (avoids an import cycle).
type KBReadTool struct {
	ReadKB func(ctx context.Context, agentID, query string) (string, error)
}

func NewKBReadTool() *KBReadTool { return &KBReadTool{} }

func (t *KBReadTool) SetReader(fn func(ctx context.Context, agentID, query string) (string, error)) {
	t.ReadKB = fn
}

func (t *KBReadTool) Name() string { return "kb_read" }
func (t *KBReadTool) Description() string {
	return "Search and read the company/department knowledge base (documents in Drive you have access to). Use to look up policies, specs, or reference docs before acting."
}
func (t *KBReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Name or keywords of the document to find"},
		},
		"required": []string{"query"},
	}
}

func (t *KBReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.ReadKB == nil {
		return ErrorResult("knowledge base not available")
	}
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query is required")
	}
	agentID := AgentIDFromCtx(ctx)
	out, err := t.ReadKB(ctx, agentID, query)
	if err != nil {
		return ErrorResult("kb_read failed: " + err.Error())
	}
	if out == "" {
		return TextResult("No matching knowledge-base documents found.")
	}
	return TextResult(out)
}
