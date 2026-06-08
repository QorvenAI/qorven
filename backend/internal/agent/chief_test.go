// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "testing"

func TestChiefSpec_OrgFields(t *testing.T) {
	spec := ChiefSpec()
	if spec.AgentKey != "chief" {
		t.Fatalf("want agent_key=chief, got %q", spec.AgentKey)
	}
	if spec.OrgRole != "coo" {
		t.Errorf("want org_role=coo, got %q", spec.OrgRole)
	}
	if spec.OrgLevel != "l1" {
		t.Errorf("want org_level=l1, got %q", spec.OrgLevel)
	}
	if spec.Title != "COO" {
		t.Errorf("want title=COO, got %q", spec.Title)
	}
}
