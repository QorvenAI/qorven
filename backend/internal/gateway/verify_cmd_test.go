package gateway

import (
	"strings"
	"testing"
)

func TestVerifyForMarkers(t *testing.T) {
	if c := verifyForMarkers(map[string]bool{"go.mod": true}); !strings.Contains(c, "go build") {
		t.Errorf("go project should build: %q", c)
	}
	if c := verifyForMarkers(map[string]bool{"package.json": true}); c == "" {
		t.Errorf("node project should have a verify cmd: %q", c)
	}
	if c := verifyForMarkers(map[string]bool{}); c != "" {
		t.Errorf("unknown project should be a no-op: %q", c)
	}
	// Python markers
	if c := verifyForMarkers(map[string]bool{"pyproject.toml": true}); !strings.Contains(c, "python") {
		t.Errorf("python project (pyproject.toml) should have a verify cmd: %q", c)
	}
	if c := verifyForMarkers(map[string]bool{"requirements.txt": true}); !strings.Contains(c, "python") {
		t.Errorf("python project (requirements.txt) should have a verify cmd: %q", c)
	}
	if c := verifyForMarkers(map[string]bool{"setup.py": true}); !strings.Contains(c, "python") {
		t.Errorf("python project (setup.py) should have a verify cmd: %q", c)
	}
}
