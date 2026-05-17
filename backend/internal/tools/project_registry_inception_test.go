// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools_test

import (
	"testing"

	"github.com/qorvenai/qorven/internal/tools"
)

func TestCreateFromInception_StoresBriefID(t *testing.T) {
	dir := t.TempDir()
	r := tools.NewProjectRegistry(dir)

	p := r.CreateFromInception("my-app", "My App", "/tmp/my-app", "brief-abc123")

	if p.InceptionBriefID != "brief-abc123" {
		t.Fatalf("want brief-abc123 got %q", p.InceptionBriefID)
	}
	got := r.GetByBriefID("brief-abc123")
	if got == nil || got.ID != p.ID {
		t.Fatal("GetByBriefID: expected to find the project, got nil")
	}
	if r.GetByBriefID("nonexistent") != nil {
		t.Fatal("GetByBriefID: expected nil for unknown brief")
	}
	// Verify persistence survives a reload
	r2 := tools.NewProjectRegistry(dir)
	got2 := r2.GetByBriefID("brief-abc123")
	if got2 == nil || got2.InceptionBriefID != "brief-abc123" {
		t.Fatalf("brief ID not persisted after reload: got %v", got2)
	}
}
