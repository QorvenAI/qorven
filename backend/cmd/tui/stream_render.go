// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	apievents "github.com/qorvenai/qorven/internal/api/events"
)

func payload(f streamFrame) []byte {
	if len(f.Properties) > 0 {
		return f.Properties
	}
	return f.Data
}

func fingerprint(f streamFrame) string {
	p := payload(f)
	if len(p) > 128 {
		p = p[:128]
	}
	return string(apievents.CanonicalType(f.Type)) + "|" + string(p)
}

const seenFPMax = 10_000

func seenFPInsert(m map[string]struct{}, ord *[]string, fp string) {
	if len(m) >= seenFPMax {
		oldest := (*ord)[0]
		*ord = (*ord)[1:]
		delete(m, oldest)
	}
	m[fp] = struct{}{}
	*ord = append(*ord, fp)
}

func extractTextPayload(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return nil
	}
	var wrap struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil
	}
	return []byte(wrap.Text)
}

func applyAgentProgress(tools []toolEvent, p apievents.AgentProgressProps) []toolEvent {
	name := ""
	args := ""
	if p.Detail != nil {
		if n, ok := p.Detail["tool"].(string); ok {
			name = n
		}
		if a, ok := p.Detail["args"]; ok {
			if s, ok := a.(string); ok {
				args = s
			} else {
				b, _ := json.Marshal(a)
				args = shorten(string(b), 60)
			}
		}
	}
	switch p.Kind {
	case "tool_start":
		if name == "" {
			return tools
		}
		return append(tools, toolEvent{name: name, args: args, status: "running"})
	case "tool_end":
		for i := len(tools) - 1; i >= 0; i-- {
			if tools[i].status == "running" && (name == "" || tools[i].name == name) {
				tools[i].status = "done"
				break
			}
		}
	}
	return tools
}

func handleLegacyPart(tools *[]toolEvent, f streamFrame) {
	var part struct {
		Type       string         `json:"type"`
		ToolName   string         `json:"toolName"`
		ToolArgs   map[string]any `json:"toolArgs,omitempty"`
		ToolResult string         `json:"toolResult,omitempty"`
	}
	if err := json.Unmarshal(payload(f), &part); err != nil {
		return
	}
	switch part.Type {
	case "tool-call":
		args := ""
		if cmd, ok := part.ToolArgs["command"].(string); ok {
			args = cmd
		} else if q, ok := part.ToolArgs["query"].(string); ok {
			args = q
		} else if len(part.ToolArgs) > 0 {
			b, _ := json.Marshal(part.ToolArgs)
			args = shorten(string(b), 60)
		}
		*tools = append(*tools, toolEvent{name: part.ToolName, args: args, status: "running"})
	case "tool-result":
		result := part.ToolResult
		if len(result) > 200 {
			result = result[:200] + "..."
		}
		for i := len(*tools) - 1; i >= 0; i-- {
			if (*tools)[i].name == part.ToolName && (*tools)[i].status == "running" {
				(*tools)[i].status = "done"
				(*tools)[i].result = result
				break
			}
		}
	}
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func renderToolCall(t toolEvent, width int) string {
	var icon string
	switch t.status {
	case "running":
		icon = lipgloss.NewStyle().Foreground(amber).Render("⏺")
	case "done":
		icon = lipgloss.NewStyle().Foreground(green).Render("✓")
	case "error":
		icon = lipgloss.NewStyle().Foreground(red).Render("✗")
	default:
		icon = lipgloss.NewStyle().Foreground(dimText).Render("○")
	}

	name := toolStyle.Render(t.name)
	args := ""
	if t.args != "" {
		argStr := t.args
		if len(argStr) > 60 {
			argStr = argStr[:59] + "…"
		}
		args = "  " + dimStyle.Render(argStr)
	}
	header := fmt.Sprintf("   %s %s%s", icon, name, args)

	if t.result == "" || t.status == "running" {
		return header
	}

	lines := strings.Split(strings.TrimRight(t.result, "\n"), "\n")
	if !t.expanded {
		hint := dimStyle.Render(fmt.Sprintf("  · %d line(s)  e=expand", len(lines)))
		return header + hint
	}

	maxLines := 10
	truncated := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	var output strings.Builder
	output.WriteString(header + "\n")
	bodyWidth := width - 6
	if bodyWidth < 10 {
		bodyWidth = 10
	}
	for _, line := range lines {
		if len(line) > bodyWidth {
			line = line[:bodyWidth] + "…"
		}
		output.WriteString("     " + dimStyle.Render(line) + "\n")
	}
	if truncated {
		output.WriteString("     " + dimStyle.Render("… (truncated — full result in session log)") + "\n")
	}
	output.WriteString("     " + dimStyle.Render("e=collapse") + "\n")
	return output.String()
}

func renderWidgetRef(w widgetRef, width int) string {
	var icon, typeStr string
	var style lipgloss.Style
	switch w.widgetType {
	case "image":
		icon, typeStr, style = "🖼 ", "image", lipgloss.NewStyle().Foreground(cyan)
	case "audio":
		icon, typeStr, style = "🔊 ", "audio", lipgloss.NewStyle().Foreground(green)
	case "video":
		icon, typeStr, style = "🎬 ", "video", lipgloss.NewStyle().Foreground(purple)
	default:
		icon, typeStr, style = "📎 ", w.widgetType, dimStyle
	}
	header := fmt.Sprintf("   %s%s", icon, style.Render(typeStr))
	if w.label != "" {
		header += "  " + dimStyle.Render(w.label)
	}
	path := w.path
	if len(path) > width-6 {
		path = "…" + path[len(path)-(width-7):]
	}
	return header + "\n     " + dimStyle.Render(path)
}

func saveWidgetToTmp(raw []byte) (widgetRef, bool) {
	var envelope struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Type == "" {
		return widgetRef{}, false
	}
	data := envelope.Data

	label := ""
	if p, ok := data["prompt"].(string); ok {
		label = p
	} else if t, ok := data["text"].(string); ok {
		label = t
	}
	if len(label) > 60 {
		label = label[:60] + "…"
	}

	ts := time.Now().Format("150405")

	switch envelope.Type {
	case "image":
		src, _ := data["url"].(string)
		if src == "" {
			return widgetRef{}, false
		}
		if strings.HasPrefix(src, "data:") {
			ext, b64 := parseDataURI(src)
			if b64 == nil {
				return widgetRef{}, false
			}
			path := filepath.Join(os.TempDir(), fmt.Sprintf("qorven-img-%s.%s", ts, ext))
			if err := os.WriteFile(path, b64, 0600); err != nil {
				return widgetRef{}, false
			}
			return widgetRef{widgetType: "image", path: path, label: label}, true
		}
		return widgetRef{widgetType: "image", path: src, label: label}, true

	case "audio":
		src, _ := data["src"].(string)
		if src == "" {
			return widgetRef{}, false
		}
		ext, b64 := parseDataURI(src)
		if b64 == nil {
			return widgetRef{}, false
		}
		path := filepath.Join(os.TempDir(), fmt.Sprintf("qorven-audio-%s.%s", ts, ext))
		if err := os.WriteFile(path, b64, 0600); err != nil {
			return widgetRef{}, false
		}
		return widgetRef{widgetType: "audio", path: path, label: label}, true

	case "video":
		src, _ := data["url"].(string)
		if src == "" {
			return widgetRef{}, false
		}
		return widgetRef{widgetType: "video", path: src, label: label}, true
	}
	return widgetRef{}, false
}

func parseDataURI(uri string) (string, []byte) {
	rest, ok := strings.CutPrefix(uri, "data:")
	if !ok {
		return "", nil
	}
	semi := strings.Index(rest, ";")
	if semi < 0 {
		return "", nil
	}
	mime := rest[:semi]
	rest = rest[semi+1:]
	rest, _ = strings.CutPrefix(rest, "base64,")
	decoded, err := base64.StdEncoding.DecodeString(rest)
	if err != nil {
		return "", nil
	}
	ext := mimeToExt(mime)
	return ext, decoded
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "audio/ogg":
		return "ogg"
	case "audio/opus":
		return "opus"
	default:
		if idx := strings.Index(mime, "/"); idx >= 0 {
			return mime[idx+1:]
		}
		return "bin"
	}
}
