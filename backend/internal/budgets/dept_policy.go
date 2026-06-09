// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package budgets

import "strings"

// Department autonomy policies.
const (
	PolicyAuto         = "auto_within_budget" // proceed if the CFO projection says it fits
	PolicyUserApproval = "user_approval"      // always ask the user
	PolicyBoth         = "both"               // auto when it fits AND is under threshold; else ask
)

// DepartmentDecision decides whether a planned department spend should apply
// immediately or be proposed to the user, given the department's policy, its
// threshold (µUSD), the plan amount (µUSD), and whether the CFO projection says
// it fits the budget. Returns "apply" or "propose" (same contract as AuthorityDecision).
func DepartmentDecision(policy string, thresholdUUSD, planUUSD int64, fits bool) string {
	switch policy {
	case PolicyUserApproval:
		return "propose"
	case PolicyBoth:
		if fits && planUUSD <= thresholdUUSD {
			return "apply"
		}
		return "propose"
	default: // PolicyAuto and any unknown policy
		if fits {
			return "apply"
		}
		return "propose"
	}
}

// itKeywords are department-name fragments that mark an IT/Code department,
// which defaults to the stricter 'both' policy (big builds ask; small auto).
var itKeywords = []string{"engineering", "it", "code", "coding", "technology", "tech", "dev", "software", "devops", "platform", "infra"}

// DefaultPolicyForDepartment returns the autonomy policy a new department should
// start with: 'both' for IT/Code-type departments, 'auto_within_budget' otherwise.
func DefaultPolicyForDepartment(name string) string {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" {
		return PolicyAuto
	}
	fields := strings.FieldsFunc(low, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '/' || r == '&'
	})
	for _, f := range fields {
		for _, kw := range itKeywords {
			if f == kw {
				return PolicyBoth
			}
		}
	}
	return PolicyAuto
}
