// Copyright 2026 Qorven AI. All rights reserved.
package calendar

import "testing"

func TestSyncStore_MethodSet(t *testing.T) {
	_ = (*SyncStore).ListSyncs
	_ = (*SyncStore).CreateSync
	_ = (*SyncStore).DeleteSync
	_ = (*SyncStore).RecordEventPush
	_ = (*SyncStore).RemoteEventID
}

func TestSyncMatchesItem(t *testing.T) {
	depA := "dep-a"
	cases := []struct {
		sScope, sScopeID, sOwner string
		iAgent                   string
		iAgentDept               string
		want                     bool
	}{
		{"company", "", "", "a1", "dep-a", true},
		{"private", "", "a1", "a1", "dep-a", true},
		{"private", "", "a2", "a1", "dep-a", false},
		{"department", depA, "", "a1", "dep-a", true},
		{"department", "dep-b", "", "a1", "dep-a", false},
	}
	for i, c := range cases {
		got := SyncMatchesItem(c.sScope, c.sScopeID, c.sOwner, c.iAgent, c.iAgentDept)
		if got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}
