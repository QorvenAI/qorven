// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

type codeCompletionReq struct {
	FilePath  string `json:"file_path"`
	Prefix    string `json:"prefix"`
	Suffix    string `json:"suffix"`
	Language  string `json:"language"`
	ProjectID string `json:"project_id"`
}

type codeCompletionResp struct {
	Completion string `json:"completion"`
	Model      string `json:"model,omitempty"`
	LatencyMs  int64  `json:"latency_ms"`
}

// handleCodeCompletions provides AI-powered inline code completions (ghost text).
// Uses the fastest available model (flash/haiku) for sub-second responses.
func (gw *Gateway) handleCodeCompletions(w http.ResponseWriter, r *http.Request) {
	var req codeCompletionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Prefix == "" {
		writeJSON(w, http.StatusOK, codeCompletionResp{})
		return
	}

	start := time.Now()

	// Build a FIM (fill-in-the-middle) prompt
	prompt := buildFIMPrompt(req.Prefix, req.Suffix, req.Language, req.FilePath)

	// Use the fastest available provider for completions
	completion, model, err := gw.generateCompletion(r.Context(), prompt)
	if err != nil {
		slog.Debug("code_completions.error", "err", err)
		writeJSON(w, http.StatusOK, codeCompletionResp{})
		return
	}

	// Clean up the completion
	completion = cleanCompletion(completion, req.Prefix)

	writeJSON(w, http.StatusOK, codeCompletionResp{
		Completion: completion,
		Model:      model,
		LatencyMs:  time.Since(start).Milliseconds(),
	})
}

func buildFIMPrompt(prefix, suffix, language, filePath string) string {
	var sb strings.Builder
	sb.WriteString("You are an expert code completion engine. Complete the code at the cursor position.\n")
	sb.WriteString("Return ONLY the code that should be inserted — no explanation, no markdown, no backticks.\n")
	sb.WriteString("Keep completions short (1-3 lines). Match the existing style and indentation.\n\n")

	if language != "" {
		sb.WriteString(fmt.Sprintf("Language: %s\n", language))
	}
	if filePath != "" {
		sb.WriteString(fmt.Sprintf("File: %s\n", filePath))
	}

	sb.WriteString("\n--- Code before cursor ---\n")
	sb.WriteString(prefix)
	sb.WriteString("\n--- Code after cursor ---\n")
	if suffix != "" {
		sb.WriteString(suffix)
	}
	sb.WriteString("\n--- Insert completion here (1-3 lines max) ---\n")

	return sb.String()
}

// generateCompletion uses the fastest available model via the provider registry.
func (gw *Gateway) generateCompletion(ctx context.Context, prompt string) (string, string, error) {
	if gw.providerReg == nil {
		return "", "", fmt.Errorf("no providers available")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	configs := gw.providerReg.List()
	if len(configs) == 0 {
		return "", "", fmt.Errorf("no providers configured")
	}

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		provider, ok := gw.providerReg.Get(cfg.ID)
		if !ok {
			continue
		}

		model := selectFastModel(cfg.ProviderType)
		if model == "" {
			model = provider.DefaultModel()
		}

		resp, err := provider.Chat(ctx, providers.ChatRequest{
			Model: model,
			Messages: []providers.Message{
				{Role: "user", Content: prompt},
			},
			Options: map[string]any{
				"max_tokens":  150,
				"temperature": 0.0,
			},
		})
		if err != nil {
			slog.Debug("code_completions.provider_fail", "provider", cfg.ID, "err", err)
			continue
		}

		return resp.Content, model, nil
	}

	return "", "", fmt.Errorf("no provider could serve completion")
}

// selectFastModel picks the fastest model from a provider type for inline completions.
func selectFastModel(providerType string) string {
	fastModels := map[string]string{
		"anthropic_native": "claude-haiku-4-5-20251001",
		"openai_compat":    "gpt-4o-mini",
		"gemini_native":    "gemini-2.0-flash",
		"groq":             "llama-3.1-8b-instant",
		"openrouter":       "google/gemini-flash-1.5",
		"bedrock":          "anthropic.claude-3-haiku-20240307-v1:0",
	}
	return fastModels[providerType]
}

func cleanCompletion(completion, prefix string) string {
	// Remove any markdown code fence wrapping
	completion = strings.TrimPrefix(completion, "```")
	if idx := strings.Index(completion, "\n"); idx >= 0 && idx < 20 {
		first := completion[:idx]
		if !strings.Contains(first, " ") && !strings.Contains(first, "(") {
			completion = completion[idx+1:]
		}
	}
	completion = strings.TrimSuffix(completion, "```")

	// Trim leading/trailing whitespace only if prefix ends with newline
	if strings.HasSuffix(prefix, "\n") {
		completion = strings.TrimRight(completion, "\n")
	}

	// Stop at logical boundaries instead of a hard line cap.
	// Walk lines and cut at the first natural stopping point beyond line 1.
	lines := strings.Split(completion, "\n")
	maxLines := 8
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		// Blank line = paragraph break — stop before it.
		if trimmed == "" {
			lines = lines[:i]
			break
		}
		// Closing brace/bracket alone = end of block — include it and stop.
		if trimmed == "}" || trimmed == "};" || trimmed == ")" || trimmed == "]" {
			lines = lines[:i+1]
			break
		}
		// New top-level definition = we've gone too far.
		if !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "\t") {
			if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "def ") ||
				strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "export ") ||
				strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "type ") {
				lines = lines[:i]
				break
			}
		}
	}

	return strings.Join(lines, "\n")
}
