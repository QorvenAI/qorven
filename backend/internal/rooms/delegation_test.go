package rooms

import (
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
)

func ptr(s string) *string { return &s }

func TestIsSubordinate(t *testing.T) {
	head := "h1"
	subs := []*agent.Agent{
		{ID: "w1", ManagerID: ptr("h1")},
		{ID: "w2", ManagerID: ptr("h1")},
	}
	if !IsSubordinate("w1", subs) {
		t.Errorf("w1 should be a subordinate")
	}
	if IsSubordinate("x9", subs) {
		t.Errorf("x9 is not in the subordinate list")
	}
	if IsSubordinate(head, subs) {
		t.Errorf("the head is not its own subordinate")
	}
	if IsSubordinate("w1", nil) {
		t.Errorf("no subordinates → false")
	}
}

func TestCanDelegate(t *testing.T) {
	const maxDepth = 1
	if !CanDelegate(0, maxDepth, true) {
		t.Errorf("depth 0 within maxDepth and budget ok → allowed")
	}
	if CanDelegate(1, maxDepth, true) {
		t.Errorf("depth 1 == maxDepth → denied (no deeper delegation)")
	}
	if CanDelegate(0, maxDepth, false) {
		t.Errorf("budget not ok → denied")
	}
}
