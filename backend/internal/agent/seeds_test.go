package agent

import "testing"

func TestMissingCSuiteSeedsExist(t *testing.T) {
	roles := []string{"cto", "cko", "ciso", "cco"}
	for _, r := range roles {
		seed, ok := AgentSeeds[r]
		if !ok {
			t.Fatalf("AgentSeeds missing role %q", r)
		}
		if seed.Soul == "" || seed.Identity == "" || seed.Tools == "" {
			t.Errorf("role %q has empty section: soul=%d identity=%d tools=%d",
				r, len(seed.Soul), len(seed.Identity), len(seed.Tools))
		}
	}
}
