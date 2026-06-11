package gateway

import "testing"

func TestStageOrder_NextStage(t *testing.T) {
	cases := []struct{ cur, want string }{
		{"intake", "clarify"}, {"clarify", "prd"}, {"prd", "spec"},
		{"spec", "design"}, {"design", "resource_plan"}, {"resource_plan", "approved"},
	}
	for _, c := range cases {
		if got := NextStage(c.cur); got != c.want {
			t.Errorf("NextStage(%q)=%q want %q", c.cur, got, c.want)
		}
	}
	if NextStage("approved") != "approved" { t.Error("terminal stage stays") }
}

func TestArtifactStageFor(t *testing.T) {
	if ArtifactStage("prd") != "prd" { t.Error("prd") }
	if ArtifactStage("design") != "design" { t.Error("design") }
}

func TestDownstreamArtifacts(t *testing.T) {
	got := DownstreamArtifacts("prd")
	want := []string{"spec", "design", "resource_plan"}
	if len(got) != len(want) { t.Fatalf("got %v want %v", got, want) }
	for i := range want { if got[i] != want[i] { t.Errorf("idx %d: %q != %q", i, got[i], want[i]) } }
	ds := DownstreamArtifacts("design")
	if len(ds) != 1 || ds[0] != "resource_plan" { t.Errorf("design downstream should be [resource_plan], got %v", ds) }
}

func TestCanAdvance(t *testing.T) {
	if !CanAdvanceTo("spec", map[string]string{"prd": "approved"}) { t.Error("should advance") }
	if CanAdvanceTo("spec", map[string]string{"prd": "in_review"}) { t.Error("should block") }
}

func TestCanAdvance_EdgeCases(t *testing.T) {
	// First artifact (prd) has no prerequisite.
	if !CanAdvanceTo("prd", map[string]string{}) { t.Error("prd should always advance (no prerequisite)") }
	// resource_plan requires design approved.
	if !CanAdvanceTo("resource_plan", map[string]string{"design": "approved"}) { t.Error("resource_plan should advance when design approved") }
	if CanAdvanceTo("resource_plan", map[string]string{"design": "in_review"}) { t.Error("resource_plan blocked when design not approved") }
	// Non-artifact stage (no gate) advances.
	if !CanAdvanceTo("clarify", map[string]string{}) { t.Error("non-artifact stage advances") }
}

func TestDownstreamArtifacts_ResourcePlan(t *testing.T) {
	// resource_plan is the LAST cascade member, so it has no downstream of its own
	// (but upstream changes — prd/spec/design — do reopen it).
	if len(DownstreamArtifacts("resource_plan")) != 0 { t.Error("resource_plan has no cascade downstream") }
}

func TestDownstreamArtifacts_IncludesResourcePlan(t *testing.T) {
	// changing the design must now reopen the resource_plan (it depends on scope)
	got := DownstreamArtifacts("design")
	found := false
	for _, d := range got { if d == "resource_plan" { found = true } }
	if !found { t.Error("design change should reopen resource_plan") }
}
