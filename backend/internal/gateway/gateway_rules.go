// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/rules"
)

// ─── Adapters ─────────────────────────────────────────────────────────────────

// ruleAgentWaker adapts *agent.RuntimeManager to rules.AgentWaker.
type ruleAgentWaker struct {
	mgr *agent.RuntimeManager
}

func (w *ruleAgentWaker) WakeWithMessage(agentID, message string) bool {
	if w.mgr == nil {
		return false
	}
	return w.mgr.WakeAgent(agentID, agent.WakeupSignal{
		Source:  agent.WakeupManual,
		Message: message,
	})
}

// ruleNotifier adapts the gateway's rtHub to rules.Notifier.
type ruleNotifier struct {
	gw *Gateway
}

func (n *ruleNotifier) BroadcastNotification(title, body string) {
	if n.gw == nil || n.gw.rtHub == nil {
		return
	}
	n.gw.rtHub.BroadcastNotification(title, body)
}

// ruleAgentLookup adapts the gateway's agent store to rules.AgentLookup.
type ruleAgentLookup struct {
	gw *Gateway
}

func (l *ruleAgentLookup) IDByKey(ctx context.Context, key string) string {
	if l.gw.agents == nil {
		return ""
	}
	a, err := l.gw.agents.GetByKey(ctx, key)
	if err != nil || a == nil {
		return ""
	}
	return a.ID
}

// ─── Startup ─────────────────────────────────────────────────────────────────

// FireRuleEvent evaluates all enabled threshold and event rules against the
// given event name and data map. Call from anywhere in the gateway to trigger
// rules without going through the HTTP layer.
//
// Examples:
//
//	gw.FireRuleEvent(ctx, "device.offline",   map[string]any{"device": "PC-04"})
//	gw.FireRuleEvent(ctx, "invoice.received", map[string]any{"amount": 1200.0, "from": "Acme"})
//	gw.FireRuleEvent(ctx, "metric",           map[string]any{"cpu": 97.5})
func (gw *Gateway) FireRuleEvent(ctx context.Context, eventName string, data map[string]any) {
	if gw.ruleEngine == nil {
		return
	}
	gw.ruleEngine.FireEvent(ctx, eventName, data)
}

// startRuleEngine creates and starts the background rule execution engine.
// No-op if the DB is not available.
func (gw *Gateway) startRuleEngine(ctx context.Context) {
	if gw.db == nil {
		slog.Info("rule_engine.skip", "reason", "no database")
		return
	}

	gw.ruleEngine = rules.New(
		gw.db.Pool,
		defaultTenant,
		&ruleAgentWaker{mgr: gw.runtimeMgr},
		&ruleNotifier{gw: gw},
		&ruleAgentLookup{gw: gw},
	)
	gw.ruleEngine.Start(ctx)
}
