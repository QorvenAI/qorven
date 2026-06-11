package gateway

import "testing"

func TestNextArtifactVersion(t *testing.T) {
	if nextArtifactVersion(nil) != 1 { t.Error("first version is 1") }
	v := 3
	if nextArtifactVersion(&v) != 4 { t.Error("bump from 3 -> 4") }
}

func TestArtifactRepoPath(t *testing.T) {
	cases := map[string]string{"prd": "docs/prd.md", "spec": "docs/spec.md", "design": "docs/design.md", "resource_plan": "docs/resource_plan.md"}
	for typ, want := range cases {
		if got := artifactRepoPath(typ); got != want { t.Errorf("%s: %q != %q", typ, got, want) }
	}
}
