// Copyright 2026 Qorven AI. All rights reserved.
package drive

import "testing"

func TestScopeConstants(t *testing.T) {
	if ScopePrivate != "private" || ScopeCompany != "company" || ScopeDepartment != "department" || ScopeCustom != "custom" {
		t.Fatalf("scope constants drifted: %q %q %q %q", ScopePrivate, ScopeCompany, ScopeDepartment, ScopeCustom)
	}
}

func TestDecideAccess(t *testing.T) {
	cases := []struct {
		name        string
		scope       string
		scopeID     string
		ownerAgent  string
		callerAgent string
		callerDepts map[string]bool
		hasGrant    bool
		isAdminUser bool
		want        bool
	}{
		{"private-owner", "private", "", "a1", "a1", nil, false, false, true},
		{"private-other", "private", "", "a1", "a2", nil, false, false, false},
		{"private-admin-user", "private", "", "a1", "", nil, false, true, true},
		{"company-any-agent", "company", "", "a1", "a2", nil, false, false, true},
		{"company-user", "company", "", "a1", "", nil, false, false, true},
		{"dept-member", "department", "d1", "a1", "a2", map[string]bool{"d1": true}, false, false, true},
		{"dept-nonmember", "department", "d1", "a1", "a2", map[string]bool{"d2": true}, false, false, false},
		{"custom-granted", "custom", "", "a1", "a2", nil, true, false, true},
		{"custom-notgranted", "custom", "", "a1", "a2", nil, false, false, false},
		{"custom-owner-always", "custom", "", "a1", "a1", nil, false, false, true},
	}
	for _, c := range cases {
		got := decideAccess(c.scope, c.scopeID, c.ownerAgent, c.callerAgent, c.callerDepts, c.hasGrant, c.isAdminUser)
		if got != c.want {
			t.Errorf("%s: decideAccess = %v, want %v", c.name, got, c.want)
		}
	}
}
