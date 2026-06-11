// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import "testing"

func TestParseUnifiedPatch_LineNumbers(t *testing.T) {
	patch := "@@ -1,3 +1,4 @@\n ctx0\n-removed\n+added1\n+added2\n ctx1"
	lines := parseUnifiedPatch(patch)
	if len(lines) != 5 {
		t.Fatalf("want 5 lines got %d: %+v", len(lines), lines)
	}
	if lines[0].Type != "eq" || lines[0].OldLine != 1 || lines[0].NewLine != 1 {
		t.Errorf("ctx0 wrong: %+v", lines[0])
	}
	if lines[1].Type != "del" || lines[1].OldLine != 2 || lines[1].NewLine != 0 {
		t.Errorf("removed wrong: %+v", lines[1])
	}
	if lines[2].Type != "add" || lines[2].NewLine != 2 || lines[2].OldLine != 0 {
		t.Errorf("added1 wrong: %+v", lines[2])
	}
	if lines[3].Type != "add" || lines[3].NewLine != 3 {
		t.Errorf("added2 wrong: %+v", lines[3])
	}
	if lines[4].Type != "eq" || lines[4].OldLine != 3 || lines[4].NewLine != 4 {
		t.Errorf("ctx1 wrong: %+v", lines[4])
	}
}

func TestParseUnifiedPatch_MultiHunk(t *testing.T) {
	patch := "@@ -1,1 +1,1 @@\n-a\n+b\n@@ -10,1 +10,2 @@\n ctx\n+new"
	lines := parseUnifiedPatch(patch)
	var ctx *patchLine
	for i := range lines {
		if lines[i].Content == "ctx" {
			ctx = &lines[i]
		}
	}
	if ctx == nil || ctx.OldLine != 10 || ctx.NewLine != 10 {
		t.Errorf("multi-hunk ctx wrong: %+v", ctx)
	}
}

func TestReviewAnchor_AddedLineUsesRightSide(t *testing.T) {
	l := patchLine{Type: "add", NewLine: 7, OldLine: 0}
	line, side := reviewAnchor(l)
	if line != 7 || side != "RIGHT" {
		t.Errorf("added anchor wrong: %d %s", line, side)
	}
	d := patchLine{Type: "del", OldLine: 4, NewLine: 0}
	line2, side2 := reviewAnchor(d)
	if line2 != 4 || side2 != "LEFT" {
		t.Errorf("removed anchor wrong: %d %s", line2, side2)
	}
}
