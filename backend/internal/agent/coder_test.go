// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "testing"

func TestCoderSpec(t *testing.T) {
	spec := CoderSpec()
	if spec.AgentKey != "coder" {
		t.Fatalf("want agent_key=coder, got %q", spec.AgentKey)
	}
	if spec.Role != "code" {
		t.Fatalf("want role=code, got %q", spec.Role)
	}
	if spec.MaxToolIterations < 20 {
		t.Fatalf("want MaxToolIterations>=20, got %d", spec.MaxToolIterations)
	}
}
