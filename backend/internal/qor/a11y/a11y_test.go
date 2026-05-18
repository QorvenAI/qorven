// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package a11y

import (
	"strings"
	"fmt"
	"testing"
)

func TestSerialize_Nil(t *testing.T) {
	text, refs := Serialize(nil)
	if text != "" { t.Error("nil should produce empty") }
	if refs != nil { t.Error("nil should produce nil refs") }
}

func TestSerialize_SingleButton(t *testing.T) {
	root := &Node{Role: "button", Name: "Submit"}
	text, refs := Serialize(root)
	if !strings.Contains(text, "button") { t.Error("should contain role") }
	if !strings.Contains(text, "Submit") { t.Error("should contain name") }
	if len(refs) != 1 { t.Errorf("button should get ref: %d", len(refs)) }
}

func TestSerialize_SkipGeneric(t *testing.T) {
	root := &Node{Role: "generic", Children: []*Node{
		{Role: "button", Name: "Click"},
	}}
	text, refs := Serialize(root)
	if strings.Contains(text, "generic") { t.Error("generic should be skipped") }
	if !strings.Contains(text, "button") { t.Error("child button should appear") }
	if len(refs) != 1 { t.Error("button should get ref") }
}

func TestSerialize_SkipNone(t *testing.T) {
	root := &Node{Role: "none", Children: []*Node{{Role: "link", Name: "Home"}}}
	text, _ := Serialize(root)
	if strings.Contains(text, "none") { t.Error("none should be skipped") }
	if !strings.Contains(text, "link") { t.Error("child should appear") }
}

func TestSerialize_NestedTree(t *testing.T) {
	root := &Node{Role: "navigation", Name: "Main", Children: []*Node{
		{Role: "link", Name: "Home"},
		{Role: "link", Name: "About"},
		{Role: "button", Name: "Login"},
	}}
	text, refs := Serialize(root)
	if !strings.Contains(text, "navigation") { t.Error("missing navigation") }
	if !strings.Contains(text, "Home") { t.Error("missing Home") }
	if !strings.Contains(text, "About") { t.Error("missing About") }
	if !strings.Contains(text, "Login") { t.Error("missing Login") }
	// 2 links + 1 button = 3 interactive refs
	if len(refs) != 3 { t.Errorf("expected 3 refs, got %d", len(refs)) }
}

func TestSerialize_ValueAndState(t *testing.T) {
	root := &Node{Role: "textbox", Name: "Email", Value: "user@example.com", Focused: true}
	text, _ := Serialize(root)
	if !strings.Contains(text, "value=user@example.com") { t.Error("missing value") }
	if !strings.Contains(text, "[focused]") { t.Error("missing focused") }
}

func TestSerialize_DisabledAndChecked(t *testing.T) {
	root := &Node{Role: "checkbox", Name: "Agree", Disabled: true, Checked: "true"}
	text, _ := Serialize(root)
	if !strings.Contains(text, "[disabled]") { t.Error("missing disabled") }
	if !strings.Contains(text, "[checked]") { t.Error("missing checked") }
}

func TestSerialize_Expanded(t *testing.T) {
	root := &Node{Role: "treeitem", Name: "Folder", Expanded: "true"}
	text, _ := Serialize(root)
	if !strings.Contains(text, "[expanded=true]") { t.Error("missing expanded") }
}

func TestSerialize_RefIDs_Sequential(t *testing.T) {
	root := &Node{Role: "group", Children: []*Node{
		{Role: "button", Name: "A"},
		{Role: "button", Name: "B"},
		{Role: "button", Name: "C"},
	}}
	text, refs := Serialize(root)
	if !strings.Contains(text, "[e1]") { t.Error("missing e1") }
	if !strings.Contains(text, "[e2]") { t.Error("missing e2") }
	if !strings.Contains(text, "[e3]") { t.Error("missing e3") }
	if refs["e1"].Name != "A" { t.Error("e1 should be A") }
	if refs["e3"].Name != "C" { t.Error("e3 should be C") }
}

func TestSerialize_Indentation(t *testing.T) {
	root := &Node{Role: "main", Children: []*Node{
		{Role: "heading", Name: "Title", Children: []*Node{
			{Role: "link", Name: "Deep"},
		}},
	}}
	text, _ := Serialize(root)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 3 { t.Fatalf("expected 3+ lines, got %d", len(lines)) }
	// Check indentation increases
	indent0 := len(lines[0]) - len(strings.TrimLeft(lines[0], " "))
	indent1 := len(lines[1]) - len(strings.TrimLeft(lines[1], " "))
	indent2 := len(lines[2]) - len(strings.TrimLeft(lines[2], " "))
	if indent1 <= indent0 { t.Error("child should be more indented") }
	if indent2 <= indent1 { t.Error("grandchild should be more indented") }
}

func TestSerializeCompact(t *testing.T) {
	root := &Node{Role: "main", Name: "Page", Children: []*Node{
		{Role: "button", Name: "Click"},
		{Role: "link", Name: "Home"},
	}}
	text := SerializeCompact(root)
	if !strings.Contains(text, "main:Page") { t.Error("missing main") }
	if !strings.Contains(text, "button:Click") { t.Error("missing button") }
	if !strings.Contains(text, " | ") { t.Error("should use pipe separator") }
}

func TestSerializeCompact_Nil(t *testing.T) {
	if SerializeCompact(nil) != "" { t.Error("nil should be empty") }
}

func TestCountInteractive(t *testing.T) {
	root := &Node{Role: "main", Children: []*Node{
		{Role: "button", Name: "A"},
		{Role: "heading", Name: "Title"},
		{Role: "link", Name: "B"},
		{Role: "textbox", Name: "C"},
		{Role: "paragraph", Name: "text"},
	}}
	count := CountInteractive(root)
	if count != 3 { t.Errorf("expected 3 interactive, got %d", count) }
}

func TestCountInteractive_Nil(t *testing.T) {
	if CountInteractive(nil) != 0 { t.Error("nil should be 0") }
}

func TestCountInteractive_DeepNested(t *testing.T) {
	root := &Node{Role: "main", Children: []*Node{
		{Role: "form", Children: []*Node{
			{Role: "textbox", Name: "Name"},
			{Role: "textbox", Name: "Email"},
			{Role: "checkbox", Name: "Agree"},
			{Role: "button", Name: "Submit"},
		}},
	}}
	if CountInteractive(root) != 4 { t.Error("should count nested interactive") }
}

func TestTruncStr(t *testing.T) {
	if truncStr("hello", 10) != "hello" { t.Error("short") }
	if truncStr("hello world", 5) != "hello…" { t.Error("long") }
	if truncStr("", 5) != "" { t.Error("empty") }
}

func TestNodeRef_Fields(t *testing.T) {
	ref := NodeRef{Role: "button", Name: "Submit", NodeID: "n1", BackendID: 42}
	if ref.Role != "button" { t.Error("wrong role") }
}

func TestNode_Properties(t *testing.T) {
	n := &Node{
		ID: "n1", Role: "textbox", Name: "Email",
		Value: "test@example.com", Focused: true, Disabled: false,
		Properties: map[string]string{"level": "2"},
	}
	if n.Value != "test@example.com" { t.Error("wrong value") }
	if !n.Focused { t.Error("should be focused") }
	if n.Properties["level"] != "2" { t.Error("wrong level") }
}

func TestFrameInfo_Fields(t *testing.T) {
	f := FrameInfo{FrameID: "f1", ParentID: "f0", URL: "https://example.com", Name: "main"}
	if f.FrameID != "f1" { t.Error("wrong frame id") }
	if f.ParentID != "f0" { t.Error("wrong parent") }
}

// === HARD A11Y TESTS ===

func TestSerialize_LargeTree(t *testing.T) {
	// Build a tree with 100 nodes
	root := &Node{Role: "main", Name: "Page"}
	for i := 0; i < 10; i++ {
		section := &Node{Role: "region", Name: "Section " + string(rune('A'+i))}
		for j := 0; j < 10; j++ {
			child := &Node{Role: "button", Name: "Button " + string(rune('0'+j))}
			section.Children = append(section.Children, child)
		}
		root.Children = append(root.Children, section)
	}
	text, refs := Serialize(root)
	if text == "" { t.Error("empty tree text") }
	if len(refs) != 100 { t.Errorf("expected 100 button refs, got %d", len(refs)) }
	// Verify sequential ref IDs
	for i := 1; i <= 100; i++ {
		key := "e" + fmt.Sprintf("%d", i)
		if _, ok := refs[key]; !ok { t.Errorf("missing ref %s", key) }
	}
}

func TestSerialize_MixedInteractive(t *testing.T) {
	root := &Node{Role: "form", Name: "Login", Children: []*Node{
		{Role: "textbox", Name: "Username"},
		{Role: "heading", Name: "Password Section"},
		{Role: "textbox", Name: "Password", Value: "***"},
		{Role: "checkbox", Name: "Remember me", Checked: "true"},
		{Role: "button", Name: "Login"},
		{Role: "link", Name: "Forgot password?"},
		{Role: "paragraph", Name: "Terms apply"},
	}}
	text, refs := Serialize(root)
	// 4 interactive: 2 textbox + 1 checkbox + 1 button + 1 link = 5
	if len(refs) != 5 { t.Errorf("expected 5 interactive refs, got %d", len(refs)) }
	if !strings.Contains(text, "[checked]") { t.Error("missing checked state") }
	if !strings.Contains(text, "value=***") { t.Error("missing password value") }
}

func TestCountInteractive_LargeTree(t *testing.T) {
	root := &Node{Role: "main"}
	for i := 0; i < 50; i++ {
		root.Children = append(root.Children, &Node{Role: "button", Name: "B"})
		root.Children = append(root.Children, &Node{Role: "heading", Name: "H"})
	}
	count := CountInteractive(root)
	if count != 50 { t.Errorf("expected 50, got %d", count) }
}

func TestSerializeCompact_LargeTree(t *testing.T) {
	root := &Node{Role: "nav", Name: "Main", Children: []*Node{
		{Role: "link", Name: "Home"},
		{Role: "link", Name: "About"},
		{Role: "link", Name: "Contact"},
		{Role: "button", Name: "Menu"},
	}}
	text := SerializeCompact(root)
	parts := strings.Split(text, " | ")
	if len(parts) < 4 { t.Errorf("expected 4+ parts, got %d", len(parts)) }
}

func TestSerialize_AllInteractiveRoles(t *testing.T) {
	roles := []string{"button", "link", "textbox", "checkbox", "radio", "combobox",
		"menuitem", "tab", "switch", "slider", "spinbutton", "searchbox",
		"option", "menuitemcheckbox", "menuitemradio", "treeitem"}
	root := &Node{Role: "group"}
	for _, role := range roles {
		root.Children = append(root.Children, &Node{Role: role, Name: role + "_test"})
	}
	_, refs := Serialize(root)
	if len(refs) != len(roles) { t.Errorf("expected %d refs, got %d", len(roles), len(refs)) }
}
