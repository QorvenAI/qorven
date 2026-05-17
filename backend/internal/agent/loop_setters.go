// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"sync"

	"github.com/qorvenai/qorven/internal/billing"
	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/plugin"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/tools"
)

func (l *Loop) SetSkillStore(store *skills.Store) { l.skillStore = store }

func (l *Loop) SetConnectorKB(kb *connectors.KnowledgeStore, mgr *mcp.Manager) {
	l.connKB = kb
	l.mcpMgr = mgr
}

func (l *Loop) SetBillingStore(bs *billing.Store) { l.billingStore = bs }

func (l *Loop) SetBundleStore(bs *BundleStore) { l.bundleStore = bs }

func (l *Loop) SetSystemKnowledge(content string)                              { l.systemKnowledge = content }
func (l *Loop) SetProjectRegistry(reg *tools.ProjectRegistry)                 { l.projectReg = reg }
func (l *Loop) SetAuditFn(fn func(agent, tool, session string, isError bool)) { l.auditFn = fn }

func (l *Loop) SetSkillLearner(learner *skills.Learner) { l.skillLearner = learner }

func (l *Loop) SetPluginManager(mgr *plugin.Manager) { l.pluginMgr = mgr }

func (l *Loop) SetPrimeDelegation(pd *PrimeDelegation) { l.primeDelegation = pd }

func (l *Loop) SetPromptCache(pc *PromptCache) { l.promptCache = pc }

func (l *Loop) runParallelTools(ctx context.Context, toolCtx context.Context, req RunRequest, calls []providers.ToolCall, allowed map[string]bool) map[string]*tools.Result {
	if len(calls) <= 1 {
		return nil // single tool call — let sequential handle it
	}

	// Check if ALL calls are read-only and allowed
	for _, tc := range calls {
		if !allowed[tc.Name] || isMutatingTool(tc.Name) {
			return nil // has mutating or blocked tool — fall back to sequential
		}
	}

	// All read-only — run in parallel
	results := make(map[string]*tools.Result, len(calls))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tc := range calls {
		wg.Add(1)
		go func(call providers.ToolCall) {
			defer wg.Done()
			r := l.executeTool(toolCtx, req, call.Name, call.Arguments)
			mu.Lock()
			results[call.ID] = r
			mu.Unlock()
		}(tc)
	}
	wg.Wait()

	slog.Info("agent.loop.parallel_batch", "tools", len(calls))
	return results
}

func (l *Loop) InvalidatePromptCache(agentID string) {
	if l.promptCache != nil {
		l.promptCache.Invalidate(agentID)
	}
}
