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
	want := []string{"spec", "design"}
	if len(got) != len(want) { t.Fatalf("got %v want %v", got, want) }
	for i := range want { if got[i] != want[i] { t.Errorf("idx %d: %q != %q", i, got[i], want[i]) } }
	if len(DownstreamArtifacts("design")) != 0 { t.Error("design has no downstream") }
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
	// resource_plan is owned by 8B (CFO), not part of the request-changes cascade.
	if len(DownstreamArtifacts("resource_plan")) != 0 { t.Error("resource_plan has no cascade downstream") }
}
