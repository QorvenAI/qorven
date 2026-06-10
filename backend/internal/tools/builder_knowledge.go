// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/builderkb"
)

// GetBuilderKnowledgeTool returns embedded knowledge for building apps on
// Qorven (the manifest, scaffold/UI-bundle flow, DB conventions, the design
// system, and external apps). It backs the always-on prompt summary so agents
// can pull deep detail on demand without bloating every prompt.
type GetBuilderKnowledgeTool struct{}

func NewGetBuilderKnowledgeTool() *GetBuilderKnowledgeTool { return &GetBuilderKnowledgeTool{} }

func (t *GetBuilderKnowledgeTool) Name() string { return "get_builder_knowledge" }

func (t *GetBuilderKnowledgeTool) Description() string {
	return "Get detailed knowledge for building apps/plugins on Qorven. Topics: " +
		strings.Join(builderkb.Topics(), ", ") +
		". Call this when building or editing a Qorven app and you need the manifest schema, the UI-bundle pattern, DB/migration rules, the design system, or external-app guidance."
}

func (t *GetBuilderKnowledgeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic": map[string]any{
				"type":        "string",
				"description": "Which topic to retrieve.",
				"enum":        builderkb.Topics(),
			},
		},
		"required": []string{"topic"},
	}
}

func (t *GetBuilderKnowledgeTool) Execute(_ context.Context, args map[string]any) *Result {
	topic, _ := args["topic"].(string)
	content, ok := builderkb.Get(topic)
	if !ok {
		// Unknown/empty topic — content is the overview + the valid-topic list.
		return TextResult(content)
	}
	return TextResult(content)
}
