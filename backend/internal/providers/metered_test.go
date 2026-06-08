// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"errors"
	"testing"
)

type fakeProvider struct {
	chatCalled bool
	resp       *ChatResponse
	err        error
}

func (f *fakeProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	f.chatCalled = true
	return f.resp, f.err
}
func (f *fakeProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	f.chatCalled = true
	return f.resp, f.err
}
func (f *fakeProvider) DefaultModel() string { return "fake-model" }
func (f *fakeProvider) Name() string         { return "fake" }

type fakeEnforcer struct {
	err      error
	gotScope MeterScope
}

func (e *fakeEnforcer) Check(ctx context.Context, s MeterScope) error { e.gotScope = s; return e.err }

type recordCall struct {
	scope    MeterScope
	model    string
	provider string
	keyID    string
	usage    Usage
}
type fakeRecorder struct{ calls []recordCall }

func (r *fakeRecorder) RecordScoped(ctx context.Context, s MeterScope, model, providerID, keyID string, u Usage) {
	r.calls = append(r.calls, recordCall{s, model, providerID, keyID, u})
}

func TestMetered_EnforceBeforeCall_BlockSkipsInner(t *testing.T) {
	inner := &fakeProvider{resp: &ChatResponse{}}
	enf := &fakeEnforcer{err: errors.New("over budget")}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, enf, rec)
	_, err := m.Chat(context.Background(), ChatRequest{Model: "x"})
	if err == nil {
		t.Fatal("expected block error")
	}
	if inner.chatCalled {
		t.Fatal("inner provider must NOT be called when enforcer blocks")
	}
	if len(rec.calls) != 0 {
		t.Fatal("must not record when blocked")
	}
}

func TestMetered_RecordsAfterSuccess_WithScope(t *testing.T) {
	inner := &fakeProvider{resp: &ChatResponse{Usage: &Usage{PromptTokens: 10, CompletionTokens: 20}}}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, &fakeEnforcer{}, rec)
	ctx := WithMeterScope(context.Background(), MeterScope{TenantID: "t", AgentID: "a", Origin: OriginAgent})
	_, err := m.Chat(ctx, ChatRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 record, got %d", len(rec.calls))
	}
	c := rec.calls[0]
	if c.scope.AgentID != "a" || c.model != "gpt-4o" || c.usage.PromptTokens != 10 {
		t.Fatalf("record carried wrong data: %+v", c)
	}
}

func TestMetered_NoRecordOnInnerError(t *testing.T) {
	inner := &fakeProvider{err: errors.New("provider 500")}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, &fakeEnforcer{}, rec)
	_, _ = m.Chat(context.Background(), ChatRequest{Model: "x"})
	if len(rec.calls) != 0 {
		t.Fatal("must not record a failed call")
	}
}

func TestMetered_NilResponseNotRecorded(t *testing.T) {
	inner := &fakeProvider{resp: nil}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, &fakeEnforcer{}, rec)
	_, _ = m.Chat(context.Background(), ChatRequest{Model: "x"})
	if len(rec.calls) != 0 {
		t.Fatal("nil response → no record")
	}
}

func TestMetered_NilUsageNotRecorded(t *testing.T) {
	inner := &fakeProvider{resp: &ChatResponse{Usage: nil}}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, &fakeEnforcer{}, rec)
	_, _ = m.Chat(context.Background(), ChatRequest{Model: "x"})
	if len(rec.calls) != 0 {
		t.Fatal("nil usage → no record (nothing to meter)")
	}
}

func TestMetered_PassThroughNameAndDefaultModel(t *testing.T) {
	m := NewMeteredProvider(&fakeProvider{}, &fakeEnforcer{}, &fakeRecorder{})
	if m.Name() != "fake" || m.DefaultModel() != "fake-model" {
		t.Fatal("Name/DefaultModel must pass through")
	}
}

func TestMetered_StreamRecordsFromFinalResp(t *testing.T) {
	inner := &fakeProvider{resp: &ChatResponse{Usage: &Usage{PromptTokens: 5, CompletionTokens: 7}}}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, &fakeEnforcer{}, rec)
	_, _ = m.ChatStream(context.Background(), ChatRequest{Model: "claude-haiku-4-5"}, func(StreamChunk) {})
	if len(rec.calls) != 1 || rec.calls[0].usage.CompletionTokens != 7 {
		t.Fatalf("stream must record from final resp.Usage, got %+v", rec.calls)
	}
}

func TestMetered_BypassSkipsEnforceAndRecord(t *testing.T) {
	inner := &fakeProvider{resp: &ChatResponse{Usage: &Usage{PromptTokens: 9}}}
	enf := &fakeEnforcer{err: errors.New("would block")}
	rec := &fakeRecorder{}
	m := NewMeteredProvider(inner, enf, rec)
	ctx := WithMeterBypass(context.Background())
	resp, err := m.Chat(ctx, ChatRequest{Model: "x"})
	if err != nil {
		t.Fatalf("bypass must skip the (blocking) enforcer, got %v", err)
	}
	if resp == nil || !inner.chatCalled {
		t.Fatal("bypass must still call the inner provider")
	}
	if len(rec.calls) != 0 {
		t.Fatal("bypass must NOT record (pipeline records instead)")
	}
}
