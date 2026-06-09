// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

// RecordScoped must satisfy providers.Recorder.
func TestCostLedger_RecordScoped_Implements_Recorder(t *testing.T) {
	var _ providers.Recorder = (*CostLedger)(nil)
}

func TestCostLedger_RecordScoped_NilDBNoPanic(t *testing.T) {
	l := &CostLedger{} // db nil
	l.RecordScoped(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a", Origin: providers.OriginMemory},
		"gpt-4o", "openai", "", providers.Usage{PromptTokens: 1, CompletionTokens: 1})
}

func TestRecordScoped_BuildsEntryWithHierarchyIDs(t *testing.T) {
	e := buildScopedEntry(providers.MeterScope{
		TenantID: "t", AgentID: "a", DepartmentID: "d1", ProjectID: "p1", TaskID: "k1",
		Origin: providers.OriginAgent,
	}, "gpt-4o", "openai", "key1", providers.Usage{PromptTokens: 5, CompletionTokens: 5})
	if e.departmentID != "d1" || e.projectID != "p1" || e.taskID != "k1" {
		t.Fatalf("entry missing hierarchy ids: %+v", e)
	}
	if e.tenantID != "t" || e.agentID != "a" || e.modelID != "gpt-4o" {
		t.Fatalf("entry base fields wrong: %+v", e)
	}
}
