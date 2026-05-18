// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package sandbox

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var codeBlockRe = regexp.MustCompile("(?s)```(python|javascript|bash|sh|go|typescript)\\n(.*?)```")

// CodeBlock represents a detected code block in LLM output.
type CodeBlock struct {
	Language string
	Code     string
	StartIdx int
	EndIdx   int
}

// DetectCodeBlocks finds executable code blocks in text.
func DetectCodeBlocks(text string) []CodeBlock {
	matches := codeBlockRe.FindAllStringSubmatchIndex(text, -1)
	blocks := []CodeBlock{}
	for _, m := range matches {
		if len(m) >= 6 {
			lang := text[m[2]:m[3]]
			code := text[m[4]:m[5]]
			if strings.TrimSpace(code) != "" {
				blocks = append(blocks, CodeBlock{Language: lang, Code: strings.TrimSpace(code), StartIdx: m[0], EndIdx: m[1]})
			}
		}
	}
	return blocks
}

// ExecuteBlocks runs detected code blocks and returns results.
func (s *Store) ExecuteBlocks(ctx context.Context, agentID string, blocks []CodeBlock) []BlockResult {
	results := []BlockResult{}
	for _, b := range blocks {
		cmd := langCommand(b.Language)
		run, err := s.Execute(ctx, agentID, cmd, b.Language, b.Code)
		r := BlockResult{Language: b.Language, Code: b.Code}
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Output = run.Output
			r.ExitCode = run.ExitCode
			r.DurationMs = run.DurationMs
		}
		results = append(results, r)
	}
	return results
}

// BlockResult is the execution result of a code block.
type BlockResult struct {
	Language   string `json:"language"`
	Code       string `json:"code"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int    `json:"duration_ms"`
}

// FormatResults formats execution results for injection back into chat.
func FormatResults(results []BlockResult) string {
	if len(results) == 0 { return "" }
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\n\n**Code Execution [%s] #%d:**\n", r.Language, i+1))
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("❌ Error: %s\n", r.Error))
		} else if r.ExitCode != 0 {
			sb.WriteString(fmt.Sprintf("⚠️ Exit code %d (%dms)\n```\n%s\n```\n", r.ExitCode, r.DurationMs, truncate(r.Output, 2000)))
		} else {
			sb.WriteString(fmt.Sprintf("✅ Success (%dms)\n```\n%s\n```\n", r.DurationMs, truncate(r.Output, 2000)))
		}
	}
	return sb.String()
}

func langCommand(lang string) string {
	switch lang {
	case "python": return "python3"
	case "javascript": return "node"
	case "typescript": return "npx tsx"
	case "bash", "sh": return "bash"
	case "go": return "go run"
	default: return "bash"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max] + "\n... (truncated)"
}
