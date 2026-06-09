package rooms

import (
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
)

func agents() []*agent.Agent {
	return []*agent.Agent{
		{ID: "a1", AgentKey: "prime-abc", OrgRole: "coo", DisplayName: "Prime"},
		{ID: "a2", AgentKey: "eng-cto", OrgRole: "cto", DisplayName: "CTO"},
		{ID: "a3", AgentKey: "researcher", OrgRole: "", DisplayName: "Researcher"},
	}
}

func TestResolveMention_ByOrgRole(t *testing.T) {
	got := ResolveMention("COO", agents())
	if got == nil || got.ID != "a1" {
		t.Fatalf("@COO should resolve to a1 (coo), got %+v", got)
	}
	got2 := ResolveMention("cto", agents())
	if got2 == nil || got2.ID != "a2" {
		t.Fatalf("@cto should resolve to a2, got %+v", got2)
	}
}

func TestResolveMention_FallbackToAgentKey(t *testing.T) {
	got := ResolveMention("Researcher", agents())
	if got == nil || got.ID != "a3" {
		t.Fatalf("@Researcher should resolve by key to a3, got %+v", got)
	}
}

func TestResolveMention_OrgRoleWinsOverKey(t *testing.T) {
	ags := []*agent.Agent{
		{ID: "a1", AgentKey: "cto", OrgRole: "", DisplayName: "Keyed CTO"},
		{ID: "a2", AgentKey: "eng-lead", OrgRole: "cto", DisplayName: "Real CTO"},
	}
	got := ResolveMention("cto", ags)
	if got == nil || got.ID != "a2" {
		t.Fatalf("org_role match should win, want a2, got %+v", got)
	}
}

func TestResolveMention_NoMatch(t *testing.T) {
	if ResolveMention("nobody", agents()) != nil {
		t.Errorf("unknown mention should return nil")
	}
}
