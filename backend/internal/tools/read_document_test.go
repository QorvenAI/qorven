// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParsePageRange covers the three supported syntaxes + error paths.
// Guards the PDF tool's most error-prone argument.
func TestParsePageRange(t *testing.T) {
	cases := []struct {
		in        string
		total     int
		wantStart int
		wantEnd   int
		wantErr   bool
	}{
		{"", 10, 1, 10, false},        // empty = all
		{"3", 10, 3, 3, false},        // single page
		{"1-5", 10, 1, 5, false},      // explicit range
		{"1-99", 10, 1, 10, false},    // end clipped to total
		{"0-5", 10, 1, 5, false},      // start promoted to 1
		{"", 0, 1, 0, false},           // zero-total: empty = (1, 0)
		{"11", 10, 0, 0, true},         // out of range
		{"abc", 10, 0, 0, true},        // malformed
		{"3-2", 10, 0, 0, true},        // empty range
		{"1-", 10, 0, 0, true},         // malformed
	}
	for _, c := range cases {
		gotStart, gotEnd, err := parsePageRange(c.in, c.total)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePageRange(%q, %d) should error", c.in, c.total)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePageRange(%q, %d) unexpected err: %v", c.in, c.total, err)
			continue
		}
		if gotStart != c.wantStart || gotEnd != c.wantEnd {
			t.Errorf("parsePageRange(%q, %d) = %d..%d, want %d..%d",
				c.in, c.total, gotStart, gotEnd, c.wantStart, c.wantEnd)
		}
	}
}

// TestReadPlainText_UnderCap: reading a file smaller than the cap
// returns the full content.
func TestReadPlainText_UnderCap(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")
	want := "# Title\n\nbody text\n"
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readPlainText(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("roundtrip mismatch:\ngot  %q\nwant %q", got, want)
	}
}

// TestReadPlainText_OverCap: reading a file bigger than the cap
// truncates and appends the marker. Guards the budget behaviour
// the agent loop relies on.
func TestReadPlainText_OverCap(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "big.txt")
	data := strings.Repeat("x", 2048)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readPlainText(path, 512)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "[truncated to fit byte budget]…\n") {
		t.Errorf("output missing truncation marker; suffix was %q", got[max(0, len(got)-50):])
	}
}

// TestReadDocumentV2_ExtText: the tool dispatches to readPlainText
// for .txt and .md, which means we can end-to-end-test without any
// PDF/DOCX fixtures.
func TestReadDocumentV2_TextDispatch(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hello.md")
	_ = os.WriteFile(path, []byte("# Hello\n\nWorld.\n"), 0o644)

	tool := NewReadDocumentV2Tool(tmp)
	r := tool.Execute(context.Background(), map[string]any{"path": path})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "# Hello") {
		t.Errorf("output should preserve markdown header; got %q", r.ForLLM)
	}
}

// TestReadDocumentV2_UnsupportedExt: asking for a .xlsx or similar
// produces a clean error naming the supported list.
func TestReadDocumentV2_UnsupportedExt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sheet.xlsx")
	_ = os.WriteFile(path, []byte("fake"), 0o644)

	tool := NewReadDocumentV2Tool(tmp)
	r := tool.Execute(context.Background(), map[string]any{"path": path})
	if !r.IsError {
		t.Fatal("unsupported extension should error")
	}
	for _, want := range []string{".pdf", ".docx", ".txt", ".md"} {
		if !strings.Contains(r.ForLLM, want) {
			t.Errorf("error should list supported extensions; missing %q from %q", want, r.ForLLM)
		}
	}
}

// TestReadDocumentV2_PathOutsideWorkspace: without AllowPaths, a path
// outside the workspace must be rejected. Same model as read_file.
func TestReadDocumentV2_PathOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	path := filepath.Join(other, "f.md")
	_ = os.WriteFile(path, []byte("hello"), 0o644)

	tool := NewReadDocumentV2Tool(ws)
	r := tool.Execute(context.Background(), map[string]any{"path": path})
	if !r.IsError {
		t.Fatal("path outside workspace should be rejected without AllowPaths")
	}
	if !strings.Contains(r.ForLLM, "outside") {
		t.Errorf("error should mention outside workspace; got %q", r.ForLLM)
	}
}

// TestReadDocumentV2_AllowPaths: after AllowPaths adds a prefix, the
// path resolves and reads normally.
func TestReadDocumentV2_AllowPaths(t *testing.T) {
	ws := t.TempDir()
	other := t.TempDir()
	path := filepath.Join(other, "f.md")
	_ = os.WriteFile(path, []byte("# H\n"), 0o644)

	tool := NewReadDocumentV2Tool(ws)
	tool.AllowPaths(other)

	r := tool.Execute(context.Background(), map[string]any{"path": path})
	if r.IsError {
		t.Fatalf("allow-listed path should succeed: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "# H") {
		t.Error("content should be returned")
	}
}

// TestReadDocumentV2_MissingPath: explicit error, not a panic, when
// the argument is absent.
func TestReadDocumentV2_MissingPath(t *testing.T) {
	tool := NewReadDocumentV2Tool(t.TempDir())
	r := tool.Execute(context.Background(), map[string]any{})
	if !r.IsError {
		t.Fatal("missing path should error")
	}
}

// TestExtractDOCX_MinimalFixture: build a minimal DOCX in-memory to
// verify the extractor produces expected markdown from a known input.
// The zip structure matches what Word writes: just word/document.xml.
func TestExtractDOCX_MinimalFixture(t *testing.T) {
	tmp := t.TempDir()
	docxPath := filepath.Join(tmp, "test.docx")
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading1"/></w:pPr>
      <w:r><w:t>Main Title</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>This is a paragraph.</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:pStyle w:val="Heading2"/></w:pPr>
      <w:r><w:t>Sub-section</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Another paragraph.</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("word/document.xml")
	_, _ = f.Write([]byte(docXML))
	_ = zw.Close()
	_ = os.WriteFile(docxPath, buf.Bytes(), 0o644)

	got, err := extractDOCX(docxPath, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Main Title", "This is a paragraph.", "### Sub-section", "Another paragraph."} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

// TestExtractDOCX_MalformedZip: a file that's not a valid zip errors
// cleanly rather than panicking.
func TestExtractDOCX_MalformedZip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "not-a-docx.docx")
	_ = os.WriteFile(path, []byte("this is not a zip"), 0o644)

	_, err := extractDOCX(path, 1024)
	if err == nil {
		t.Fatal("malformed docx should error")
	}
}

// TestExtractDOCX_MissingDocumentXML: a zip without word/document.xml
// is technically a valid zip but not a valid DOCX.
func TestExtractDOCX_MissingDocumentXML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.docx")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("some-other-file.txt")
	_, _ = f.Write([]byte("hi"))
	_ = zw.Close()
	_ = os.WriteFile(path, buf.Bytes(), 0o644)

	_, err := extractDOCX(path, 1024)
	if err == nil {
		t.Fatal("docx missing document.xml should error")
	}
	if !strings.Contains(err.Error(), "document.xml") {
		t.Errorf("error should mention document.xml; got %q", err)
	}
}

// helper — Go 1.21+ has min() built-in but tests might run on 1.20
// in some envs, so pin a local version to be safe.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
