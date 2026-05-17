// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/api/events"
)

func newTestServer(submit SubmitFunc, runner CommandRunner) *Server {
	return &Server{
		Emitter: events.NewEmitter(),
		Drafts:  NewDraftStore(0),
		Submit:  submit,
		Run:     runner,
	}
}

func post(h http.Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON response: %v (%s)", err, w.Body.String())
	}
	return resp
}

func TestAppendAndSubmit(t *testing.T) {
	var submitted struct {
		sid, agent, text string
	}
	submit := func(_ context.Context, sid, agent, text string, _ map[string]string) (string, error) {
		submitted.sid = sid
		submitted.agent = agent
		submitted.text = text
		return "msg-1", nil
	}
	s := newTestServer(submit, nil)
	h := s.Routes()

	w := post(h, "/append_prompt", AppendPromptRequest{SessionID: "sess", Text: "hello "})
	if w.Code != 200 {
		t.Fatalf("append: %d %s", w.Code, w.Body.String())
	}
	w = post(h, "/append_prompt", AppendPromptRequest{SessionID: "sess", Text: "world"})
	if w.Code != 200 {
		t.Fatalf("append 2: %d", w.Code)
	}

	w = post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "sess", AgentID: "prime"})
	if w.Code != 202 {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	if submitted.text != "hello world" {
		t.Fatalf("text: %q", submitted.text)
	}
	if submitted.agent != "prime" || submitted.sid != "sess" {
		t.Fatalf("args: %+v", submitted)
	}
	// Draft should be consumed.
	if s.Drafts.Get("sess") != "" {
		t.Fatalf("draft not cleared")
	}
}

func TestSubmit_InlineText(t *testing.T) {
	var got string
	s := newTestServer(func(_ context.Context, _, _, text string, _ map[string]string) (string, error) {
		got = text
		return "m", nil
	}, nil)
	h := s.Routes()

	// Pre-fill draft that should be wiped by inline text.
	s.Drafts.Append("sess", "stale draft")

	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "sess", Text: "new prompt"})
	if w.Code != 202 {
		t.Fatalf("code: %d %s", w.Code, w.Body.String())
	}
	if got != "new prompt" {
		t.Fatalf("got %q", got)
	}
	if s.Drafts.Get("sess") != "" {
		t.Fatalf("draft not cleared")
	}
}

func TestClearPrompt(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()
	s.Drafts.Append("sess", "anything")
	w := post(h, "/clear_prompt", ClearPromptRequest{SessionID: "sess"})
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if s.Drafts.Get("sess") != "" {
		t.Fatalf("not cleared")
	}
}

func TestSubmit_EmptyRejected(t *testing.T) {
	s := newTestServer(func(_ context.Context, _, _, _ string, _ map[string]string) (string, error) {
		t.Fatalf("submit must not be called with empty text")
		return "", nil
	}, nil)
	h := s.Routes()
	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "sess"})
	if w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "empty prompt") {
		t.Fatalf("body: %s", w.Body.String())
	}
}

func TestExecuteCommand(t *testing.T) {
	s := newTestServer(nil, func(_ context.Context, cmd string, args json.RawMessage) (map[string]any, error) {
		switch cmd {
		case "echo":
			return map[string]any{"raw": string(args)}, nil
		case "boom":
			return nil, errors.New("explosion")
		case "weird":
			return nil, errors.New("unknown command weird")
		}
		return nil, errors.New("unknown command " + cmd)
	})
	h := s.Routes()

	w := post(h, "/execute", ExecuteCommandRequest{Command: "echo", Args: json.RawMessage(`{"k":1}`)})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}

	w = post(h, "/execute", ExecuteCommandRequest{Command: "boom"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	w = post(h, "/execute", ExecuteCommandRequest{Command: "weird"})
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	r := decodeResp(t, w)
	if r.Code != "unknown_command" {
		t.Fatalf("code: %s", r.Code)
	}

	w = post(h, "/execute", ExecuteCommandRequest{})
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestToast(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()

	w := post(h, "/toast", ShowToastRequest{Level: "info", Message: "hello"})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = post(h, "/toast", ShowToastRequest{Level: "weird", Message: "x"})
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
	w = post(h, "/toast", ShowToastRequest{Level: "info"})
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestResize(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()
	w := post(h, "/resize", ResizeRequest{Cols: 80, Rows: 24})
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	w = post(h, "/resize", ResizeRequest{Cols: 0, Rows: 24})
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

func TestAppendRejectsLargeText(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()
	large := strings.Repeat("x", 64*1024+1)
	w := post(h, "/append_prompt", AppendPromptRequest{SessionID: "sess", Text: large})
	if w.Code != 413 {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestContentTypeEnforced(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()
	r := httptest.NewRequest(http.MethodPost, "/append_prompt", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("expected 415, got %d", w.Code)
	}
}

func TestUnknownFieldsRejected(t *testing.T) {
	s := newTestServer(nil, nil)
	h := s.Routes()
	r := httptest.NewRequest(http.MethodPost, "/append_prompt", strings.NewReader(`{"session_id":"s","text":"t","mystery":true}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestResolverUsed(t *testing.T) {
	s := newTestServer(func(_ context.Context, sid, _, _ string, _ map[string]string) (string, error) {
		if sid != "canonical-uuid" {
			t.Fatalf("session id not resolved, got %q", sid)
		}
		return "m", nil
	}, nil)
	s.Resolve = func(_ context.Context, sid string) (string, error) {
		if sid == "alias" {
			return "canonical-uuid", nil
		}
		return "", errors.New("not found")
	}
	h := s.Routes()
	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "alias", Text: "hi"})
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestOwnerCheck_Permit(t *testing.T) {
	s := newTestServer(func(_ context.Context, _, _, _ string, _ map[string]string) (string, error) {
		return "m", nil
	}, nil)
	s.OwnerCheck = func(_ context.Context, _ string) error { return nil }
	h := s.Routes()
	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "s", Text: "hi"})
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d %s", w.Code, w.Body.String())
	}
}

func TestOwnerCheck_Forbidden_Submit(t *testing.T) {
	s := newTestServer(func(_ context.Context, _, _, _ string, _ map[string]string) (string, error) {
		t.Fatalf("submit must not run when authz denies")
		return "", nil
	}, nil)
	s.OwnerCheck = func(_ context.Context, _ string) error {
		return &AuthzError{Reason: "not_owner", Code: "not_owner"}
	}
	h := s.Routes()
	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "s", Text: "hi"})
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeResp(t, w)
	if resp.Code != "not_owner" {
		t.Fatalf("code: %s", resp.Code)
	}
}

func TestOwnerCheck_Forbidden_AppendAndClear(t *testing.T) {
	s := newTestServer(nil, nil)
	s.OwnerCheck = func(_ context.Context, _ string) error {
		return &AuthzError{Reason: "not_owner", Code: "not_owner"}
	}
	h := s.Routes()
	w := post(h, "/append_prompt", AppendPromptRequest{SessionID: "s", Text: "hi"})
	if w.Code != 403 {
		t.Fatalf("append: expected 403, got %d", w.Code)
	}
	w = post(h, "/clear_prompt", ClearPromptRequest{SessionID: "s"})
	if w.Code != 403 {
		t.Fatalf("clear: expected 403, got %d", w.Code)
	}
}

func TestOwnerCheck_ExecuteForwardsSessionID(t *testing.T) {
	called := false
	s := newTestServer(nil, func(_ context.Context, _ string, _ json.RawMessage) (map[string]any, error) {
		called = true
		return nil, nil
	})
	s.OwnerCheck = func(_ context.Context, sid string) error {
		if sid != "sess-X" {
			t.Fatalf("owner check saw sid=%q", sid)
		}
		return &AuthzError{Reason: "blocked", Code: "forbidden"}
	}
	h := s.Routes()
	w := post(h, "/execute", ExecuteCommandRequest{
		Command: "abort",
		Args:    json.RawMessage(`{"session_id":"sess-X"}`),
	})
	if w.Code != 403 {
		t.Fatalf("expected 403, got %d %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatalf("runner must not be invoked when authz denies")
	}
}

func TestOwnerCheck_InternalErrorReturns500(t *testing.T) {
	s := newTestServer(nil, nil)
	s.OwnerCheck = func(_ context.Context, _ string) error {
		return errors.New("db down")
	}
	h := s.Routes()
	w := post(h, "/submit_prompt", SubmitPromptRequest{SessionID: "s", Text: "hi"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestDraftStore_GuardedPanicsOnMultiReplica(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on replicas=2")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T %v", r, r)
		}
		if !strings.Contains(msg, "MUST NOT run with 2 replicas") {
			t.Fatalf("unexpected panic: %s", msg)
		}
	}()
	_ = NewDraftStoreGuarded(time.Minute, 2)
}

func TestDraftStore_GuardedOKOnSingleReplica(t *testing.T) {
	s := NewDraftStoreGuarded(time.Minute, 1)
	if s == nil {
		t.Fatalf("nil store")
	}
	s.Append("sess", "x")
	if s.Get("sess") != "x" {
		t.Fatalf("draft lost")
	}
}

func TestDraftStore_TTLSweep(t *testing.T) {
	// Use a real time.Time in the past — sweep should keep entries younger
	// than the cutoff, drop entries older.
	now := time.Now()
	s := NewDraftStore(time.Second)
	s.ttl = time.Hour
	s.Append("a", "x")
	// Cutoff far in the past → entry survives.
	s.sweep(now)
	if s.Get("a") == "" {
		t.Fatalf("sweep with recent cutoff should keep fresh entries")
	}
	// Force the entry to appear stale by rewriting its timestamp.
	s.mu.Lock()
	s.drafts["a"].updatedAt = now.Add(-2 * time.Hour)
	s.mu.Unlock()
	s.sweep(now)
	if s.Get("a") != "" {
		t.Fatalf("sweep should have dropped stale entry")
	}
}
