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
