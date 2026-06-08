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
