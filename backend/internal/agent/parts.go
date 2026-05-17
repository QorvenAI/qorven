// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

// MessagePart is a typed segment of an assistant message.
// Each part renders with its own UI component.
type MessagePart struct {
	Type string `json:"type"` // text, reasoning, tool-call, tool-result, widget, source, code, image, error

	// Text / Reasoning
	Content string `json:"content,omitempty"`

	// Tool
	ToolName   string `json:"toolName,omitempty"`
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolArgs   any    `json:"toolArgs,omitempty"`
	ToolResult any    `json:"toolResult,omitempty"`
	Duration   int    `json:"duration,omitempty"` // ms

	// Widget (weather, calc, chart, etc.)
	WidgetType string `json:"widgetType,omitempty"`
	WidgetData any    `json:"widgetData,omitempty"`

	// Source
	Sources []SourceRef `json:"sources,omitempty"`

	// Code execution
	Language string `json:"language,omitempty"`
	Code     string `json:"code,omitempty"`
	Output   string `json:"output,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`

	// Image
	URL string `json:"url,omitempty"`
	Alt string `json:"alt,omitempty"`
}

type SourceRef struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Helper constructors
func TextPart(content string) MessagePart {
	return MessagePart{Type: "text", Content: content}
}

func ReasoningPart(content string) MessagePart {
	return MessagePart{Type: "reasoning", Content: content}
}

func ToolCallPart(name, callID string, args any) MessagePart {
	return MessagePart{Type: "tool-call", ToolName: name, ToolCallID: callID, ToolArgs: args}
}

func ToolResultPart(name, callID string, result any, durationMs int) MessagePart {
	return MessagePart{Type: "tool-result", ToolName: name, ToolCallID: callID, ToolResult: result, Duration: durationMs}
}

func WidgetPart(widgetType string, data any) MessagePart {
	return MessagePart{Type: "widget", WidgetType: widgetType, WidgetData: data}
}

func SourcePart(sources []SourceRef) MessagePart {
	return MessagePart{Type: "source", Sources: sources}
}

func CodeExecPart(lang, code, output string, exitCode int) MessagePart {
	return MessagePart{Type: "code", Language: lang, Code: code, Output: output, ExitCode: exitCode}
}

func ImagePart(url, alt string) MessagePart {
	return MessagePart{Type: "image", URL: url, Alt: alt}
}

func ErrorPart(msg string) MessagePart {
	return MessagePart{Type: "error", Content: msg}
}
