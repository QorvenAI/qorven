// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package delegation holds shared constants for the agent delegation system.
// It is intentionally dependency-free so both the rooms package (which imports
// agent) and the tools package (which is imported by agent) can reference it
// without creating an import cycle.
package delegation

// MaxDepth is the maximum number of delegation hops allowed in a single work
// cascade. Depth 0 is the originating agent (e.g. CEO); depth 1 is its direct
// delegate (COO); depth 2 is an officer; depth 3 is a worker; depth 4 is a
// sub-task runner. A depth check of depth < MaxDepth allows depths 0–3 and
// refuses 4, giving exactly four active hops:
//
//	CEO (0) → COO (1) → officer (2) → worker (3) → sub-task runner (4 — refused)
//
// Both rooms.MaxDelegationDepth and the delegate tool reference this constant.
const MaxDepth = 4
