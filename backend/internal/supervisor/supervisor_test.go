// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AuditInterval <= 0 { t.Error("audit interval should be > 0") }
	if cfg.BaseSampleLow <= 0 { t.Errorf("sample rate out of range: %f", cfg.BaseSampleLow) }
	if cfg.HeartbeatTTL <= 0 { t.Error("heartbeat TTL should be > 0") }
	if cfg.ResponseTimeout <= 0 { t.Error("response timeout should be > 0") }
	if cfg.BaseSampleLow > cfg.BaseSampleMedium { t.Error("low <= medium") }
	if cfg.BaseSampleMedium > cfg.BaseSampleHigh { t.Error("medium <= high") }
	if cfg.BaseSampleHigh > 1.0 { t.Errorf("high sample <= 1.0, got %.2f", cfg.BaseSampleHigh) }
}

func TestSupervisorConfig_Fields(t *testing.T) {
	cfg := SupervisorConfig{AuditInterval: 5 * time.Minute, BaseSampleLow: 0.1}
	if cfg.AuditInterval != 5*time.Minute { t.Error("wrong interval") }
	if cfg.BaseSampleLow != 0.1 { t.Error("wrong sample low") }
}

func TestAgentInfo_Fields(t *testing.T) {
	info := AgentInfo{ID: "a1", Name: "TestBot", Model: "gpt-4"}
	if info.ID != "a1" { t.Error("wrong id") }
	if info.Name != "TestBot" { t.Error("wrong name") }
}

func TestEvalResult_Fields(t *testing.T) {
	r := EvalResult{Quality: "good"}
	if r.Quality != "good" { t.Error("wrong quality") }
}

func TestEvalResult_Qualities(t *testing.T) {
	for _, q := range []string{"good", "degraded", "bad"} {
		r := EvalResult{Quality: q}
		if r.Quality == "" { t.Error("empty quality") }
	}
}

func TestNewSupervisor(t *testing.T) {
	s := NewSupervisor(nil, nil, DefaultConfig(), "prime")
	if s == nil { t.Fatal("nil supervisor") }
}

func TestSupervisor_Stop_NotStarted(t *testing.T) {
	s := NewSupervisor(nil, nil, DefaultConfig(), "prime")
	s.Stop() // should not panic
}

func TestSupervisor_ShouldSample_Always(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseSampleLow = 1.0; cfg.BaseSampleMedium = 1.0; cfg.BaseSampleHigh = 1.0
	s := NewSupervisor(nil, nil, cfg, "prime")
	if !s.shouldSample("any-agent") { t.Error("100% sample rate should always sample") }
}

func TestSupervisor_ShouldSample_Never(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseSampleLow = 0.0; cfg.BaseSampleMedium = 0.0; cfg.BaseSampleHigh = 0.0
	_ = NewSupervisor(nil, nil, cfg, "prime")
}

func TestAgentHealth_Fields(t *testing.T) {
	h := AgentHealth{AgentID: "a1", AgentName: "TestBot", Status: "healthy"}
	if h.AgentID == "" { t.Error("empty agent ID") }
	if h.Status != "healthy" { t.Error("wrong status") }
}

func TestAgentHealth_StatusValues(t *testing.T) {
	for _, s := range []string{"healthy", "degraded", "unresponsive", "suspended"} {
		h := AgentHealth{Status: s}
		if h.Status != s { t.Errorf("status=%q", h.Status) }
	}
}

func TestFixCatalog_NewCatalog_NotNil(t *testing.T) {
	if NewFixCatalog(FixDependencies{}) == nil { t.Fatal("nil catalog") }
}

func TestFixCatalog_AllFixTypesRegistered(t *testing.T) {
	catalog := NewFixCatalog(FixDependencies{})
	for _, ft := range []FixType{FixRestartCron, FixRetryAPI, FixClearCache, FixSwitchModel,
		FixResetSession, FixRestartChannel, FixAdjustTimeout, FixPurgeOldData} {
		if catalog.Get(ft) == nil { t.Errorf("fix type %q not registered", ft) }
	}
}

func TestFixCatalog_UnknownFixReturnsNil(t *testing.T) {
	if NewFixCatalog(FixDependencies{}).Get("nonexistent") != nil {
		t.Error("unknown fix should return nil")
	}
}

func TestFixCatalog_RiskLevels(t *testing.T) {
	catalog := NewFixCatalog(FixDependencies{})
	fix := catalog.Get(FixSwitchModel)
	if fix == nil { t.Fatal("switch_model not in catalog") }
	if fix.Risk == RiskHigh { t.Error("switch_model should not be high risk") }
}

func TestFixCatalog_Execute_NilDeps_NoPanic(t *testing.T) {
	catalog := NewFixCatalog(FixDependencies{})
	fix := catalog.Get(FixClearCache)
	if fix == nil { t.Skip("FixClearCache not registered") }
	_ = fix.Execute(context.Background(), map[string]any{"key": "test"})
}

func TestFixCatalog_History_InitiallyEmpty(t *testing.T) {
	catalog := NewFixCatalog(FixDependencies{})
	if len(catalog.History(100)) != 0 { t.Error("new catalog history should be empty") }
}

func TestRiskLevel_Values(t *testing.T) {
	if RiskLow == RiskMedium { t.Error("low != medium") }
	if RiskMedium == RiskHigh { t.Error("medium != high") }
	if RiskLow == RiskHigh { t.Error("low != high") }
}

func TestSupervisor_SetListAgents(t *testing.T) {
	s := NewSupervisor(nil, nil, DefaultConfig(), "prime")
	called := false
	s.SetListAgents(func(_ context.Context) ([]AgentInfo, error) {
		called = true
		return []AgentInfo{{ID: "a1", Name: "TestBot"}}, nil
	})
	agents, err := s.listAgents(context.Background())
	if err != nil { t.Fatalf("listAgents error: %v", err) }
	if !called { t.Error("listAgents should be called") }
	if len(agents) != 1 { t.Errorf("expected 1 agent, got %d", len(agents)) }
}

func TestSupervisor_SetEvaluator(t *testing.T) {
	s := NewSupervisor(nil, nil, DefaultConfig(), "prime")
	called := false
	s.SetEvaluator(func(_ context.Context, _, _ string) (*EvalResult, error) {
		called = true
		return &EvalResult{Quality: "good"}, nil
	})
	result, err := s.evaluateOutput(context.Background(), "a1", "some output")
	if err != nil { t.Fatalf("evaluator error: %v", err) }
	if !called { t.Error("evaluator should be called") }
	if result.Quality != "good" { t.Errorf("quality=%q", result.Quality) }
}

func TestBus_NewBus_WithNilEscalation(t *testing.T) {
	b := NewBus(nil)
	if b == nil { t.Fatal("nil bus") }
}

func TestBus_RegisterAndSend(t *testing.T) {
	b := NewBus(nil)
	received := make(chan Message, 1)
	b.Register("agent-1", func(_ context.Context, msg Message) *Message {
		received <- msg
		return nil
	})
	ctx := context.Background()
	b.Send(ctx, Message{From: "prime", To: "agent-1", Intent: IntentStatusRequest})
	select {
	case msg := <-received:
		if msg.From != "prime" { t.Errorf("from=%q", msg.From) }
		if msg.Intent != IntentStatusRequest { t.Errorf("intent=%q", msg.Intent) }
	case <-time.After(2 * time.Second):
		t.Error("message not received within 2 seconds")
	}
}

func TestBus_SendToUnregistered_NoPanic(t *testing.T) {
	b := NewBus(nil)
	b.Send(context.Background(), Message{From: "prime", To: "nonexistent", Intent: IntentStatusRequest})
}





func TestMessage_IntentConstants(t *testing.T) {
	for _, intent := range []Intent{IntentStatusRequest, IntentReviewRequest,
		IntentACK, IntentEscalationNotice, IntentAutoFix, IntentHeartbeat} {
		if intent == "" { t.Error("intent should not be empty") }
	}
}

func TestMessage_Fields(t *testing.T) {
	msg := Message{From: "prime", To: "agent-1", Intent: IntentStatusRequest}
	if msg.From != "prime" { t.Error("wrong From") }
	if msg.To != "agent-1" { t.Error("wrong To") }
	if msg.Intent != IntentStatusRequest { t.Error("wrong Intent") }
}
