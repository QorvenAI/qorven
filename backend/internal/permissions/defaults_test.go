package permissions_test

import (
	"testing"
	"github.com/qorvenai/qorven/internal/permissions"
)

func TestDefaultsForRole_General(t *testing.T) {
	defs := permissions.DefaultsForRole("general")
	found := false
	for _, d := range defs {
		if d.Tool == "cron" && d.Scope == permissions.ScopeAutoApproved {
			found = true
		}
	}
	if !found {
		t.Error("expected cron to be auto_approved in general role defaults")
	}
}

func TestDefaultsForRole_Researcher(t *testing.T) {
	defs := permissions.DefaultsForRole("researcher")
	for _, d := range defs {
		if d.Tool == "exec" && d.Scope != permissions.ScopeBlocked {
			t.Errorf("exec should be blocked in researcher defaults, got scope=%s", d.Scope)
		}
	}
}

func TestDefaultsForRole_Unknown(t *testing.T) {
	defs := permissions.DefaultsForRole("nonexistent_role")
	if len(defs) == 0 {
		t.Error("expected fallback defaults for unknown role")
	}
}
