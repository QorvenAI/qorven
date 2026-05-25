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
// of proper function_call format. Handles four patterns:
//
//  1. Gemini tool_code (single-line): ```tool_code\nfunc(arg='val')\n```
//  2. Gemini tool_code (multi-line):  ```tool_code\nprint(default_api.write_file(path='p', content='''...'''))\n```
//  3. JSON tool_code:                 ```tool_code\n{"name":"func","args":{...}}\n```
//  4. XML tool_call:                  <tool_call><func><arg>val</arg></func></tool_call>
//
// Returns extracted tool calls and the content with tool blocks removed.
func extractNarratedToolCalls(content string, knownTools map[string]bool) ([]providers.ToolCall, string) {
	if content == "" || len(knownTools) == 0 {
		return nil, content
	}

	var calls []providers.ToolCall
	cleaned := content

	// Pattern 1: ```tool_code\nfunc(args)\n``` (includes multi-line Gemini default_api style)
	calls, cleaned = extractToolCodeBlocks(cleaned, knownTools)

	// Pattern 2: XML <tool_call> blocks
	if len(calls) == 0 {
		calls, cleaned = extractXMLToolCalls(cleaned, knownTools)
	}

	return calls, strings.TrimSpace(cleaned)
}

// --- Pattern 1: tool_code blocks ---

var toolCodeBlockRe = regexp.MustCompile("(?s)```tool_code\\s*\\n(.*?)```")

// defaultAPICallRe matches: print(default_api.func_name(key='val', key2='''multi\nline'''))
// or simply: default_api.func_name(key='val')
var defaultAPICallRe = regexp.MustCompile(`(?s)(?:print\s*\()?\s*default_api\.(\w+)\(`)

func extractToolCodeBlocks(content string, knownTools map[string]bool) ([]providers.ToolCall, string) {
	matches := toolCodeBlockRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, content
	}

	var calls []providers.ToolCall
	for _, m := range matches {
		body := strings.TrimSpace(m[1])

		// Try multi-line Gemini default_api style first (handles triple-quoted strings)
		if mCalls := extractDefaultAPICalls(body, knownTools); len(mCalls) > 0 {
			calls = append(calls, mCalls...)
			continue
		}

		// Fall back to line-by-line parsing for simple single-line tool_code
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

// extractDefaultAPICalls parses Gemini's default_api.func_name(key=value, key='''multiline''') style.
// Handles both print(default_api.func(...)) and bare default_api.func(...).
func extractDefaultAPICalls(body string, knownTools map[string]bool) []providers.ToolCall {
	var calls []providers.ToolCall

	// Find each default_api.funcname( occurrence
	nameMatches := defaultAPICallRe.FindAllStringSubmatchIndex(body, -1)
	for _, loc := range nameMatches {
		// loc[2], loc[3] = start/end of captured func name
		funcName := body[loc[2]:loc[3]]
		if !knownTools[funcName] {
			continue
		}

		// Find the argument start: after the opening (
		argStart := loc[1] // right after the matched prefix including '('

		// Walk forward counting parens/quotes to find matching close paren
		argsStr := extractBalancedArgs(body, argStart)
		if argsStr == "" {
			continue
		}

		args := parseDefaultAPIArgs(argsStr)
		calls = append(calls, providers.ToolCall{
			ID:        fmt.Sprintf("narrated_%s", funcName),
			Name:      funcName,
			Arguments: args,
		})
	}
	return calls
}

// extractBalancedArgs extracts argument text starting at pos (after the opening paren).
// Handles nested parens and triple-quoted strings.
func extractBalancedArgs(s string, pos int) string {
	depth := 1
	i := pos
	for i < len(s) && depth > 0 {
		if i+2 < len(s) && (s[i:i+3] == "'''" || s[i:i+3] == `"""`) {
			// Skip triple-quoted string
			quote := s[i : i+3]
			i += 3
			for i+2 < len(s) {
				if s[i:i+3] == quote {
					i += 3
					break
				}
				i++
			}
			continue
		}
		if s[i] == '\'' || s[i] == '"' {
			// Skip single-quoted string
			q := s[i]
			i++
			for i < len(s) && s[i] != q {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			i++ // closing quote
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return s[pos:i]
			}
		}
		i++
	}
	return ""
}

// parseDefaultAPIArgs parses keyword args from Gemini's call style.
// Handles: key='val', key='''multiline content''', key="val"
func parseDefaultAPIArgs(s string) map[string]any {
	args := make(map[string]any)
	s = strings.TrimSpace(s)

	i := 0
	for i < len(s) {
		// Skip whitespace and commas
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}

		// Read key (up to '=')
		keyStart := i
		for i < len(s) && s[i] != '=' {
			i++
		}
		if i >= len(s) {
			break
		}
		key := strings.TrimSpace(s[keyStart:i])
		i++ // skip '='

		if i >= len(s) {
			break
		}

		// Read value
		var val string
		if i+2 < len(s) && (s[i:i+3] == "'''" || s[i:i+3] == `"""`) {
			// Triple-quoted string
			quote := s[i : i+3]
			i += 3
			valStart := i
			for i+2 < len(s) {
				if s[i:i+3] == quote {
					val = s[valStart:i]
					i += 3
					break
				}
				i++
			}
		} else if s[i] == '\'' || s[i] == '"' {
			// Single-quoted string
			q := s[i]
			i++
			var sb strings.Builder
			for i < len(s) && s[i] != q {
				if s[i] == '\\' && i+1 < len(s) {
					i++
					sb.WriteByte(s[i])
				} else {
					sb.WriteByte(s[i])
				}
				i++
			}
			i++ // closing quote
			val = sb.String()
		} else {
			// Unquoted value — read until comma or end
			valStart := i
			for i < len(s) && s[i] != ',' {
				i++
			}
			val = strings.TrimSpace(s[valStart:i])
		}

		if key != "" {
			args[key] = val
		}
	}

	return args
}

// parseToolCodeLine handles: func(arg1='val1', arg2='val2')
// and: default_api.func(arg='val') and: print(default_api.func(arg='val'))
// and: func: command args
func parseToolCodeLine(line string, knownTools map[string]bool) *providers.ToolCall {
	// Strip print(...) wrapper
	if strings.HasPrefix(line, "print(") && strings.HasSuffix(line, "))") {
		line = line[len("print(") : len(line)-1]
	}
	// Strip default_api. prefix
	if idx := strings.Index(line, "default_api."); idx >= 0 {
		line = line[idx+len("default_api."):]
	}

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
