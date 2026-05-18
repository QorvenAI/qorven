// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// extractNarratedToolCalls parses tool calls that LLMs output as text instead
// of proper function_call format. Handles three patterns:
//
//   1. Gemini tool_code:  ```tool_code\nfunc(arg='val')\n```
//   2. JSON tool_code:    ```tool_code\n{"name":"func","args":{...}}\n```
//   3. XML tool_call:     <tool_call><func><arg>val</arg></func></tool_call>
//
// Returns extracted tool calls and the content with tool blocks removed.
func extractNarratedToolCalls(content string, knownTools map[string]bool) ([]providers.ToolCall, string) {
	if content == "" || len(knownTools) == 0 {
		return nil, content
	}

	var calls []providers.ToolCall
	cleaned := content

	// Pattern 1: ```tool_code\nfunc(args)\n```
	calls, cleaned = extractToolCodeBlocks(cleaned, knownTools)

	// Pattern 2: XML <tool_call> blocks
	if len(calls) == 0 {
		calls, cleaned = extractXMLToolCalls(cleaned, knownTools)
	}

	return calls, strings.TrimSpace(cleaned)
}

// --- Pattern 1: tool_code blocks ---

var toolCodeBlockRe = regexp.MustCompile("(?s)```tool_code\\s*\\n(.*?)```")

func extractToolCodeBlocks(content string, knownTools map[string]bool) ([]providers.ToolCall, string) {
	matches := toolCodeBlockRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, content
	}

	var calls []providers.ToolCall
	for _, m := range matches {
		body := strings.TrimSpace(m[1])
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if tc := parseToolCodeLine(line, knownTools); tc != nil {
				calls = append(calls, *tc)
			}
		}
	}

	if len(calls) == 0 {
		return nil, content
	}

	cleaned := toolCodeBlockRe.ReplaceAllString(content, "")
	return calls, cleaned
}

// parseToolCodeLine handles: func(arg1='val1', arg2='val2')
// and: func: command args
func parseToolCodeLine(line string, knownTools map[string]bool) *providers.ToolCall {
	// Try func(args) format
	if idx := strings.Index(line, "("); idx > 0 {
		name := strings.TrimSpace(line[:idx])
		if !knownTools[name] {
			return nil
		}
		argsStr := line[idx+1:]
		argsStr = strings.TrimSuffix(argsStr, ")")
		args := parseKwargs(argsStr)
		return &providers.ToolCall{
			ID:        fmt.Sprintf("narrated_%s", name),
			Name:      name,
			Arguments: args,
		}
	}

	// Try "func: args" format (exec: ls -la)
	if idx := strings.Index(line, ":"); idx > 0 {
		name := strings.TrimSpace(line[:idx])
		if !knownTools[name] {
			return nil
		}
		rest := strings.TrimSpace(line[idx+1:])
		args := map[string]any{"command": rest}
		if name != "exec" {
			args = map[string]any{"input": rest}
		}
		return &providers.ToolCall{
			ID:        fmt.Sprintf("narrated_%s", name),
			Name:      name,
			Arguments: args,
		}
	}

	return nil
}

// parseKwargs parses: arg1='val1', arg2='val2'
func parseKwargs(s string) map[string]any {
	args := make(map[string]any)

	// Try JSON first
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) == nil {
			return m
		}
	}

	// Parse Python-style kwargs: key='value', key='value'
	for _, part := range splitKwargs(s) {
		part = strings.TrimSpace(part)
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		val = strings.Trim(val, "'\"")
		args[key] = val
	}
	return args
}

// splitKwargs splits on commas but respects quotes
func splitKwargs(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' || c == '"' {
			if inQuote == 0 {
				inQuote = c
			} else if inQuote == c {
				inQuote = 0
			}
		}
		if c == ',' && inQuote == 0 {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// --- Pattern 2: XML tool calls ---

var xmlToolCallRe = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>`)

func extractXMLToolCalls(content string, knownTools map[string]bool) ([]providers.ToolCall, string) {
	matches := xmlToolCallRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, content
	}

	var calls []providers.ToolCall
	for _, m := range matches {
		body := strings.TrimSpace(m[1])
		// Find tool name: <tool_name> or <tool_name attr="val">
		nameRe := regexp.MustCompile(`<(\w+)`)
		nameMatch := nameRe.FindStringSubmatch(body)
		if nameMatch == nil {
			continue
		}
		name := nameMatch[1]
		if !knownTools[name] {
			continue
		}
		// Extract args from child elements: <key>value</key>
		args := make(map[string]any)
		argRe := regexp.MustCompile(`<(\w+)>(.*?)</\w+>`)
		for _, am := range argRe.FindAllStringSubmatch(body, -1) {
			if am[1] != name {
				args[am[1]] = am[2]
			}
		}
		calls = append(calls, providers.ToolCall{
			ID:        fmt.Sprintf("narrated_%s", name),
			Name:      name,
			Arguments: args,
		})
	}

	if len(calls) == 0 {
		return nil, content
	}

	cleaned := xmlToolCallRe.ReplaceAllString(content, "")
	return calls, cleaned
}
