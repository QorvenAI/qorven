// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

// Classification is the sensitivity level of a knowledge item.
type Classification int

const (
	ClassPublic       Classification = 0
	ClassInternal     Classification = 1
	ClassConfidential Classification = 2
	ClassRestricted   Classification = 3
)

func (c Classification) String() string {
	switch c {
	case ClassPublic:
		return "public"
	case ClassInternal:
		return "internal"
	case ClassConfidential:
		return "confidential"
	case ClassRestricted:
		return "restricted"
	default:
		return "internal"
	}
}

func ClassificationFromString(s string) Classification {
	switch s {
	case "public":
		return ClassPublic
	case "internal":
		return ClassInternal
	case "confidential":
		return ClassConfidential
	case "restricted":
		return ClassRestricted
	default:
		return ClassInternal
	}
}

// CanWriteToScope validates that content at this classification level
// can be stored in the given scope. Restricted content cannot go to
// shared scopes; confidential cannot go to company scope.
func (c Classification) CanWriteToScope(scope Scope) bool {
	switch c {
	case ClassRestricted:
		return scope == ScopeAgent || scope == ScopeSession || scope == ScopeTask
	case ClassConfidential:
		return scope != ScopeCompany
	default:
		return true
	}
}
