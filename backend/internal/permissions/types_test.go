package permissions_test

import (
	"testing"
	"github.com/qorvenai/qorven/internal/permissions"
)

func TestScopeConstants(t *testing.T) {
	if permissions.ScopeAutoApproved != "auto_approved" {
		t.Fatalf("ScopeAutoApproved = %q, want auto_approved", permissions.ScopeAutoApproved)
	}
	if permissions.ScopeAskFirst != "ask_first" {
		t.Fatalf("ScopeAskFirst = %q, want ask_first", permissions.ScopeAskFirst)
	}
	if permissions.ScopeBlocked != "blocked" {
		t.Fatalf("ScopeBlocked = %q, want blocked", permissions.ScopeBlocked)
	}
}

func TestPolicyEntry_Fields(t *testing.T) {
	p := permissions.PolicyEntry{
		ID:      "abc",
		Tool:    "cron",
		Scope:   permissions.ScopeAutoApproved,
		AgentID: "agent-1",
	}
	if p.Tool != "cron" {
		t.Fatal("Tool field missing")
	}
	if !p.IsAutoApproved() {
		t.Fatal("IsAutoApproved should return true")
	}
}
