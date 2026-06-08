// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "context"

// MeteredProvider decorates a Provider with budget enforcement (before the
// call) and cost recording (after a successful call). It is the universal
// choke point: because the registry wraps every provider in one of these,
// no caller can make an un-metered LLM call. Attribution rides on the
// MeterScope carried by the request context.
type MeteredProvider struct {
	inner    Provider
	enforcer Enforcer
	recorder Recorder
}

// NewMeteredProvider wraps inner. enforcer and recorder may be nil, in which
// case the corresponding step is skipped (defensive — should be set in prod).
func NewMeteredProvider(inner Provider, enforcer Enforcer, recorder Recorder) *MeteredProvider {
	return &MeteredProvider{inner: inner, enforcer: enforcer, recorder: recorder}
}

func (m *MeteredProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	scope := MeterScopeFromCtx(ctx)
	if m.enforcer != nil {
		if err := m.enforcer.Check(ctx, scope); err != nil {
			return nil, err
		}
	}
	resp, err := m.inner.Chat(ctx, req)
	m.record(ctx, scope, req, resp, err)
	return resp, err
}

func (m *MeteredProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	scope := MeterScopeFromCtx(ctx)
	if m.enforcer != nil {
		if err := m.enforcer.Check(ctx, scope); err != nil {
			return nil, err
		}
	}
	resp, err := m.inner.ChatStream(ctx, req, onChunk)
	m.record(ctx, scope, req, resp, err)
	return resp, err
}

// record reports a completed call to the recorder. It is a no-op on error, on
// a nil response, or when the provider reported no usage (nothing to meter).
func (m *MeteredProvider) record(ctx context.Context, scope MeterScope, req ChatRequest, resp *ChatResponse, err error) {
	if err != nil || resp == nil || resp.Usage == nil || m.recorder == nil {
		return
	}
	model := req.Model
	if model == "" {
		model = m.inner.DefaultModel()
	}
	m.recorder.RecordScoped(ctx, scope, model, m.inner.Name(), "", *resp.Usage)
}

func (m *MeteredProvider) DefaultModel() string { return m.inner.DefaultModel() }
func (m *MeteredProvider) Name() string         { return m.inner.Name() }
