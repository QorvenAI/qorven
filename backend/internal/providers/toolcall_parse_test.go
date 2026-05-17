// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package providers

import "testing"

// Table-driven tests for parseToolArgs. The regression we're guarding
// is "truncated stream parses to an empty map and the tool executes
// with no args" — that used to be the silent behavior when the raw
// json.Unmarshal result was ignored.

func TestParseToolArgs_Valid(t *testing.T) {
	raw := `{"path":"/tmp/x","content":"hi"}`
	args, ok := parseToolArgsString(raw, "write_file")
	if !ok {
		t.Fatalf("valid JSON reported parse failure")
	}
	if args["path"] != "/tmp/x" || args["content"] != "hi" {
		t.Fatalf("wrong args: %+v", args)
	}
}

func TestParseToolArgs_Empty(t *testing.T) {
	// Tools with zero params get an empty JSON body; must be accepted.
	args, ok := parseToolArgsString("", "list_agents")
	if !ok {
		t.Fatalf("empty args should be ok=true")
	}
	if len(args) != 0 {
		t.Fatalf("empty args should produce empty map, got %+v", args)
	}
}

func TestParseToolArgs_EmptyObject(t *testing.T) {
	args, ok := parseToolArgsString("{}", "ping")
	if !ok {
		t.Fatal("'{}' should be valid")
	}
	if len(args) != 0 {
		t.Fatalf("empty object should produce empty map, got %+v", args)
	}
}

func TestParseToolArgs_Truncated(t *testing.T) {
	// The hostile case: stream cut off mid-value. Must report ok=false
	// so the caller refuses to execute the tool. Previously this
	// parsed to an empty map and ran with no args — write_file with
	// no path, exec with no command, etc.
	cases := []string{
		`{"path":"/tmp/x","cont`,         // mid-key
		`{"path":"/tmp/x","content":"h`,  // mid-value
		`{"path":`,                        // mid-colon
		`{"path":"/tmp/x"`,                // missing close brace
		`not json at all`,
		`{bad`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, ok := parseToolArgsString(c, "write_file")
			if ok {
				t.Fatalf("truncated/invalid JSON %q reported ok=true", c)
			}
		})
	}
}

func TestParseToolArgs_NullBecomesEmpty(t *testing.T) {
	// json.Unmarshal("null", &m) leaves m nil; we normalise so
	// downstream code doesn't have to nil-guard every access.
	args, ok := parseToolArgsString("null", "noop")
	if !ok {
		t.Fatal("'null' should parse cleanly")
	}
	if args == nil {
		t.Fatal("null args should be normalised to non-nil empty map")
	}
	if len(args) != 0 {
		t.Fatalf("null args should produce empty map, got %+v", args)
	}
}

func TestParseToolArgs_WrongRootType(t *testing.T) {
	// An array at the root can't unmarshal into a map — must report
	// failure rather than silently losing data.
	_, ok := parseToolArgsString(`[1,2,3]`, "something")
	if ok {
		t.Fatal("array root should report parse failure for map target")
	}
}
