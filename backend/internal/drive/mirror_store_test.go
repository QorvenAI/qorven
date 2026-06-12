// Copyright 2026 Qorven AI. All rights reserved.
package drive

import "testing"

func TestMirrorStore_MethodSet(t *testing.T) {
	_ = (*MirrorStore).ListMirrors
	_ = (*MirrorStore).CreateMirror
	_ = (*MirrorStore).DeleteMirror
	_ = (*MirrorStore).MirrorsForFile
	_ = (*MirrorStore).RecordPush
}

func TestMirrorMatchesFile(t *testing.T) {
	depA := "dep-a"
	cases := []struct {
		mScope, mScopeID, mOwner string
		fScope, fScopeID, fOwner string
		want                     bool
	}{
		{"company", "", "", "company", "", "a1", true},
		{"company", "", "", "private", "", "a1", false},
		{"private", "", "a1", "private", "", "a1", true},
		{"private", "", "a2", "private", "", "a1", false},
		{"department", depA, "", "department", depA, "a1", true},
		{"department", depA, "", "department", "dep-b", "a1", false},
	}
	for i, c := range cases {
		got := mirrorMatchesFile(c.mScope, c.mScopeID, c.mOwner, c.fScope, c.fScopeID, c.fOwner)
		if got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}
