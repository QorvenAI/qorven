package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/memory"
)

func TestCKOCurator_Refresh_WritesBrief(t *testing.T) {
	var written *memory.Brief
	cur := &CKOCurator{
		TenantID: "t1",
		GatherSources: func(ctx context.Context, scope, scopeKey string) ([]string, memory.Classification) {
			return []string{"decision: ship v2", "doc: style guide"}, memory.ClassInternal
		},
		Synthesize: func(ctx context.Context, scope, scopeKey string, facts []string) (string, error) {
			return "BRIEF(" + strings.Join(facts, "; ") + ")", nil
		},
		WriteBrief: func(ctx context.Context, b memory.Brief) error {
			written = &b
			return nil
		},
		ExternalResearchEnabled: false,
	}

	err := cur.Refresh(context.Background(), "company", "")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if written == nil {
		t.Fatal("expected a brief to be written")
	}
	if written.Scope != "company" || written.Clearance != memory.ClassInternal {
		t.Errorf("unexpected brief meta: %+v", written)
	}
	if !strings.Contains(written.Content, "ship v2") {
		t.Errorf("brief content missing facts: %q", written.Content)
	}
}

func TestCKOCurator_Refresh_NoFactsSkips(t *testing.T) {
	calls := 0
	cur := &CKOCurator{
		TenantID:      "t1",
		GatherSources: func(ctx context.Context, scope, scopeKey string) ([]string, memory.Classification) { return nil, memory.ClassPublic },
		Synthesize:    func(ctx context.Context, scope, scopeKey string, facts []string) (string, error) { return "x", nil },
		WriteBrief:    func(ctx context.Context, b memory.Brief) error { calls++; return nil },
	}
	if err := cur.Refresh(context.Background(), "role", "code"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if calls != 0 {
		t.Errorf("expected no write when there are no facts, got %d", calls)
	}
}
