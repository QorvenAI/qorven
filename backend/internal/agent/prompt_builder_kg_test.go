// Copyright 2026 Qorven AI. All rights reserved.
package agent

import (
	"strings"
	"testing"
)

func TestSectionKnowledge(t *testing.T) {
	pb := &PromptBuilder{knowledge: "### Entities\n- **Acme** (org)\n"}
	out := pb.sectionKnowledge()
	if !strings.Contains(out, "## Relevant Knowledge") || !strings.Contains(out, "Acme") {
		t.Fatalf("sectionKnowledge missing content: %q", out)
	}
	empty := &PromptBuilder{}
	if empty.sectionKnowledge() != "" {
		t.Fatal("empty knowledge must yield empty section")
	}
}
