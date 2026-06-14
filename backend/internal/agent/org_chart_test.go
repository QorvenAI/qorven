// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"testing"

	"github.com/google/uuid"
)

// mapParentOf builds a parentOf closure from a map[child]parent for use in
// walkIsSubordinate tests. A node with no entry in the map is treated as a root
// (returns nil, false).
func mapParentOf(tree map[uuid.UUID]*uuid.UUID) func(uuid.UUID) (*uuid.UUID, bool) {
	return func(id uuid.UUID) (*uuid.UUID, bool) {
		parent, ok := tree[id]
		if !ok {
			return nil, false
		}
		return parent, true
	}
}

func TestWalkIsSubordinate(t *testing.T) {
	// IDs for the test tree:
	//
	//   prime  (root — no entry)
	//     └── coo
	//           └── worker
	//                 └── subagent
	//
	// Plus a completely unrelated node.
	prime    := uuid.New()
	coo      := uuid.New()
	worker   := uuid.New()
	subagent := uuid.New()
	other    := uuid.New()

	tree := map[uuid.UUID]*uuid.UUID{
		coo:      &prime,
		worker:   &coo,
		subagent: &worker,
		// prime has no parent entry (root)
		// other has no parent entry (disconnected root)
	}
	parentOf := mapParentOf(tree)

	tests := []struct {
		name     string
		ancestor uuid.UUID
		desc     uuid.UUID
		want     bool
	}{
		{"direct child", prime, coo, true},
		{"grandchild", prime, worker, true},
		{"great-grandchild", prime, subagent, true},
		{"middle ancestor", coo, worker, true},
		{"middle ancestor 2-hops", coo, subagent, true},
		{"upward — child is not below its parent", coo, prime, false},
		{"unrelated node", prime, other, false},
		{"same node is not its own subordinate", prime, prime, false},
		{"no parent for root", coo, prime, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := walkIsSubordinate(tc.ancestor, tc.desc, parentOf, 10)
			if got != tc.want {
				t.Errorf("walkIsSubordinate(ancestor=%s, desc=%s) = %v; want %v",
					tc.ancestor, tc.desc, got, tc.want)
			}
		})
	}
}

// TestWalkIsSubordinate_Cycle confirms that a cycle in the tree does not cause an
// infinite loop — the bound kicks in and returns false.
func TestWalkIsSubordinate_Cycle(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	c := uuid.New()
	// a → b → c → a (cycle)
	cycleTree := map[uuid.UUID]*uuid.UUID{
		a: &b,
		b: &c,
		c: &a, // cycle
	}
	parentOf := mapParentOf(cycleTree)

	// d is NOT in the cycle; the walk from d should exhaust the bound and return false.
	d := uuid.New()
	got := walkIsSubordinate(d, a, parentOf, 10)
	if got {
		t.Error("expected false for cycle tree; walkIsSubordinate must be cycle-safe")
	}
}

// TestWalkIsSubordinate_DepthBound confirms that a chain longer than maxHops
// returns false (bounded, no hang).
func TestWalkIsSubordinate_DepthBound(t *testing.T) {
	// Build a linear chain of 15 nodes: nodes[0] is root, nodes[14] is leaf.
	const n = 15
	nodes := make([]uuid.UUID, n)
	for i := range nodes {
		nodes[i] = uuid.New()
	}
	tree := map[uuid.UUID]*uuid.UUID{}
	for i := 1; i < n; i++ {
		parent := nodes[i-1]
		tree[nodes[i]] = &parent
	}
	parentOf := mapParentOf(tree)

	// With maxHops=5 the walk from nodes[14] can only see 5 parents (nodes[13]..nodes[9]).
	// nodes[0] is too far — should return false.
	got := walkIsSubordinate(nodes[0], nodes[n-1], parentOf, 5)
	if got {
		t.Error("expected false: chain longer than maxHops should not report subordinate")
	}

	// With maxHops=14 the full 14-hop chain is reachable.
	got = walkIsSubordinate(nodes[0], nodes[n-1], parentOf, 14)
	if !got {
		t.Error("expected true: chain is exactly 14 hops and maxHops=14 should suffice")
	}
}
