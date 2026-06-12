// Copyright 2026 Qorven AI. All rights reserved.
package tools

import (
	"context"
	"testing"
)

func TestKBReadTool_Shape(t *testing.T) {
	tool := NewKBReadTool()
	if tool.Name() != "kb_read" {
		t.Fatalf("name = %q", tool.Name())
	}
	if tool.Description() == "" || tool.Parameters() == nil {
		t.Fatal("description/parameters must be set")
	}
	res := tool.Execute(context.Background(), map[string]any{"query": "policy"})
	if res == nil || !res.IsError {
		t.Fatal("expected error result when resolver not configured")
	}
}
