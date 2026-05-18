// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// SemanticFilename generates a meaningful filename using LLM hints.
// Instead of "output_1748293847.png", generates "quarterly-sales-chart.png".
func SemanticFilename(ctx context.Context, provider providers.Provider, model, content, ext string) string {
	if provider == nil || content == "" {
		return ""
	}

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model: model,
		Messages: []providers.Message{
			{Role: "user", Content: fmt.Sprintf(
				"Generate a short, descriptive filename (no extension) for this content. Use lowercase-kebab-case. Max 5 words. Content: %s",
				content[:min(len(content), 200)])},
		},
		Options: map[string]any{"temperature": 0, "max_tokens": 20},
	})
	if err != nil {
		return ""
	}

	name := strings.TrimSpace(resp.Content)
	name = sanitizeFilename(name)
	if name == "" {
		return ""
	}
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return name + ext
}

var filenameRe = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeFilename(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = filenameRe.ReplaceAllString(name, "")
	// Remove leading/trailing dashes
	name = strings.Trim(name, "-")
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// SuggestFilename returns a semantic name or falls back to the original.
func SuggestFilename(ctx context.Context, provider providers.Provider, model, originalPath, content string) string {
	ext := filepath.Ext(originalPath)
	semantic := SemanticFilename(ctx, provider, model, content, ext)
	if semantic != "" {
		return semantic
	}
	return filepath.Base(originalPath)
}
