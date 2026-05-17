// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestScreenShareStore_RoundTrip: store and retrieve a frame — the
// happy path every other test builds on.
func TestScreenShareStore_RoundTrip(t *testing.T) {
	s := NewScreenShareStore()
	s.Store("tenant1", []byte("fake-jpeg"), 1920, 1080)

	got := s.Latest("tenant1")
	if got == nil {
		t.Fatal("Latest returned nil for a freshly stored frame")
	}
	if string(got.JPEG) != "fake-jpeg" || got.Width != 1920 || got.Height != 1080 {
		t.Errorf("frame mismatch: %+v", got)
	}
}

// TestScreenShareStore_TenantIsolation: two tenants that store at
// the same time must not see each other's frames. Table-stakes for
// multi-tenant isolation.
func TestScreenShareStore_TenantIsolation(t *testing.T) {
	s := NewScreenShareStore()
	s.Store("a", []byte("a-jpeg"), 100, 100)
	s.Store("b", []byte("b-jpeg"), 200, 200)

	fa := s.Latest("a")
	fb := s.Latest("b")
	if fa == nil || string(fa.JPEG) != "a-jpeg" {
		t.Error("tenant A frame mixed up")
	}
	if fb == nil || string(fb.JPEG) != "b-jpeg" {
		t.Error("tenant B frame mixed up")
	}
}

// TestScreenShareStore_Stale: frames older than frameTTL must be
// rejected. Otherwise the LLM could reason over a yesterday-screenshot.
func TestScreenShareStore_Stale(t *testing.T) {
	s := NewScreenShareStore()
	// Force an old frame by reaching into the map — production code
	// never does this, but it's the only way to time-travel in tests.
	s.mu.Lock()
	s.frames["stale"] = &latestFrame{
		JPEG:       []byte("old"),
		ReceivedAt: time.Now().Add(-2 * frameTTL),
	}
	s.mu.Unlock()

	if got := s.Latest("stale"); got != nil {
		t.Error("stale frame must not be returned")
	}
}

// TestScreenShareStore_Clear: after Clear, Latest returns nil.
func TestScreenShareStore_Clear(t *testing.T) {
	s := NewScreenShareStore()
	s.Store("t", []byte("x"), 1, 1)
	if s.Latest("t") == nil {
		t.Fatal("store-then-latest should work")
	}
	s.Clear("t")
	if got := s.Latest("t"); got != nil {
		t.Errorf("Latest after Clear should be nil; got %+v", got)
	}
}

// TestScreenShareStore_Replace: a second Store on the same tenant
// replaces the first. We don't want frame history — the agent should
// only ever see "now".
func TestScreenShareStore_Replace(t *testing.T) {
	s := NewScreenShareStore()
	s.Store("t", []byte("first"), 1, 1)
	s.Store("t", []byte("second"), 2, 2)
	got := s.Latest("t")
	if got == nil {
		t.Fatal("latest is nil")
	}
	if string(got.JPEG) != "second" || got.Width != 2 {
		t.Errorf("replace didn't take: %+v", got)
	}
}

// TestUserScreenCaptureTool_NotSharing: when nothing is stored, the
// tool returns a helpful actionable error so the agent can tell the
// user what to click.
func TestUserScreenCaptureTool_NotSharing(t *testing.T) {
	s := NewScreenShareStore()
	tool := NewUserScreenCaptureTool(s, "tenant1")
	r := tool.Execute(context.Background(), nil)
	if !r.IsError {
		t.Fatal("expected error when no frame stored")
	}
	// The error should tell the user what UI action to take.
	if !strings.Contains(r.ForLLM, "Share Screen") {
		t.Errorf("error should reference the UI control; got %q", r.ForLLM)
	}
}

// TestUserScreenCaptureTool_Success: with a frame stored, the tool
// returns a data:image/jpeg;base64 payload the vision LLM can ingest.
func TestUserScreenCaptureTool_Success(t *testing.T) {
	s := NewScreenShareStore()
	s.Store("tenant1", []byte{0xff, 0xd8, 0xff, 0xe0}, 1920, 1080) // JPEG magic
	tool := NewUserScreenCaptureTool(s, "tenant1")

	r := tool.Execute(context.Background(), nil)
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "data:image/jpeg;base64,") {
		t.Errorf("output missing data URL; got %q", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "1920x1080") {
		t.Errorf("output missing dimensions; got %q", r.ForLLM)
	}
}

// TestUserScreenCaptureTool_MetadataStable: Name and Parameters are
// part of the contract between the registry and the LLM. Guard
// against silent rename.
func TestUserScreenCaptureTool_MetadataStable(t *testing.T) {
	tool := NewUserScreenCaptureTool(NewScreenShareStore(), "t")
	if tool.Name() != "user_screen_capture" {
		t.Errorf("name drift: %q", tool.Name())
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("params type should be object")
	}
	if _, ok := params["properties"]; !ok {
		t.Error("params missing properties field")
	}
	if len(tool.Description()) < 50 {
		t.Errorf("description too short (%d); LLM needs context", len(tool.Description()))
	}
}
