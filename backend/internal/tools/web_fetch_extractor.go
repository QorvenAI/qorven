// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ContentExtractor extracts readable content from a URL.
type ContentExtractor interface {
	Extract(ctx context.Context, rawURL string) (string, error)
	Name() string
}

// ExtractResult holds the output from a successful extraction.
type ExtractResult struct {
	Content   string
	Extractor string
}

// ExtractorChain tries extractors in order until one returns quality content.
type ExtractorChain struct {
	extractors []ContentExtractor
	maxRetries []int
	timeouts   []time.Duration
}

// NewExtractorChain creates a chain from ordered extractors.
func NewExtractorChain(extractors ...ContentExtractor) *ExtractorChain {
	maxRetries := make([]int, len(extractors))
	timeouts := make([]time.Duration, len(extractors))
	for i := range extractors {
		maxRetries[i] = 1
	}
	return &ExtractorChain{extractors: extractors, maxRetries: maxRetries, timeouts: timeouts}
}

// Extract runs each extractor in order with per-entry retry.
func (c *ExtractorChain) Extract(ctx context.Context, rawURL string) (ExtractResult, error) {
	var lastErr error
	for i, ext := range c.extractors {
		maxRetries := c.maxRetries[i]

		for attempt := 1; attempt <= maxRetries; attempt++ {
			callCtx, cancel := ctx, context.CancelFunc(nil)
			if timeout := c.timeouts[i]; timeout > 0 {
				callCtx, cancel = context.WithTimeout(ctx, timeout)
			}

			content, err := ext.Extract(callCtx, rawURL)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				lastErr = err
				if ctx.Err() != nil {
					return ExtractResult{}, fmt.Errorf("context cancelled: %w", lastErr)
				}
				if attempt < maxRetries {
					slog.Warn("extractor_chain: attempt failed, retrying",
						"extractor", ext.Name(), "url", rawURL,
						"attempt", attempt, "error", err)
				}
				continue
			}
			if !isQualityContent(content) {
				lastErr = fmt.Errorf("%s: content below quality threshold (%d chars)", ext.Name(), len(content))
				break // low quality — cascade to next
			}
			return ExtractResult{Content: content, Extractor: ext.Name()}, nil
		}
	}
	if lastErr != nil {
		return ExtractResult{}, fmt.Errorf("all extractors failed for %s: %w", rawURL, lastErr)
	}
	return ExtractResult{}, fmt.Errorf("no extractors configured")
}

func isQualityContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) < 100 {
		return false
	}
	return len(strings.Fields(trimmed)) >= 10
}

// ExtractorEntry represents a single extractor in chain settings.
type ExtractorEntry struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Timeout    int    `json:"timeout,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

type extractorChainSettings struct {
	Extractors []ExtractorEntry `json:"extractors,omitempty"`
}

// ResolveExtractorChain builds an ordered ExtractorChain.
// Default: InProcess only (no external extractors).
func ResolveExtractorChain(tool *WebFetchTool) *ExtractorChain {
	return NewExtractorChain(&InProcessExtractor{tool: tool})
}

func parseExtractorChainSettings(raw []byte, tool *WebFetchTool) *ExtractorChain {
	var settings extractorChainSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		slog.Warn("web_fetch: failed to parse extractor chain settings", "error", err)
		return nil
	}

	var extractors []ContentExtractor
	var maxRetries []int
	var timeouts []time.Duration
	for _, entry := range settings.Extractors {
		if !entry.Enabled || entry.Name == "" {
			continue
		}
		switch entry.Name {
		case "defuddle":
			extractors = append(extractors, NewDefuddleExtractorFromEntry(entry))
		case "html-to-markdown":
			extractors = append(extractors, &InProcessExtractor{tool: tool})
		default:
			slog.Warn("web_fetch: unknown extractor", "name", entry.Name)
			continue
		}
		retries := entry.MaxRetries
		if retries <= 0 {
			retries = 1
		}
		maxRetries = append(maxRetries, retries)
		timeouts = append(timeouts, time.Duration(entry.Timeout)*time.Second)
	}
	if len(extractors) == 0 {
		return nil
	}
	return &ExtractorChain{extractors: extractors, maxRetries: maxRetries, timeouts: timeouts}
}

// InProcessExtractor uses WebFetchTool's doFetch for HTML→markdown.
type InProcessExtractor struct {
	tool *WebFetchTool
}

func (e *InProcessExtractor) Name() string { return "html-to-markdown" }

func (e *InProcessExtractor) Extract(ctx context.Context, rawURL string) (string, error) {
	e.tool.mu.RLock()
	policy := e.tool.policy
	e.tool.mu.RUnlock()

	return e.tool.doFetch(ctx, rawURL, "markdown", defaultFetchMaxChars, policy)
}
