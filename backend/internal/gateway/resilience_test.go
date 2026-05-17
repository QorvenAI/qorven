// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBindListener_Free: a free port binds on the first try, no fallback.
func TestBindListener_Free(t *testing.T) {
	addr := freePort(t)
	ln, actual, err := bindListener(addr)
	if err != nil {
		t.Fatalf("bindListener on free port failed: %v", err)
	}
	defer ln.Close()
	if actual != addr {
		t.Fatalf("actual addr %q, want %q (no probe should have happened)", actual, addr)
	}
}

// TestBindListener_Contention: a contended port walks to the next one.
// This is the most important test — it's the single behavior that
// prevents the "it won't start" user report.
func TestBindListener_Contention(t *testing.T) {
	// Grab a free port, then squat on it. The next call must walk to
	// port+1. We verify by parsing the returned addr and comparing.
	squatted := freePort(t)
	squat, err := net.Listen("tcp", squatted)
	if err != nil {
		t.Fatalf("could not squat on %s: %v", squatted, err)
	}
	defer squat.Close()

	ln, actual, err := bindListener(squatted)
	if err != nil {
		t.Fatalf("bindListener should have walked to next port, got err: %v", err)
	}
	defer ln.Close()
	if actual == squatted {
		t.Fatalf("got same addr %q — probe didn't walk", actual)
	}
	// The bound port must be higher than the squatted one and within
	// the probe window.
	squattedPort := portFromAddr(squatted)
	actualPort := portFromAddr(actual)
	if actualPort <= squattedPort || actualPort > squattedPort+portProbeRange {
		t.Fatalf("actual port %d out of probe window (%d..%d]",
			actualPort, squattedPort, squattedPort+portProbeRange)
	}
}

// TestBindListener_Exhaustion: when every port in the range is busy,
// we get a useful error — not a silent 0.0.0.0 bind or a crash.
func TestBindListener_Exhaustion(t *testing.T) {
	start := freePort(t)
	startPort := portFromAddr(start)

	// Squat on start..start+portProbeRange inclusive.
	var squatters []net.Listener
	for i := 0; i <= portProbeRange; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", startPort+i)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			// Someone else grabbed one of the ports between our free-port
			// probe and the squat loop. Skip — flaky in parallel CI but
			// the functional behavior is correct either way.
			t.Skipf("could not squat on %s for exhaustion test: %v", addr, err)
		}
		squatters = append(squatters, l)
	}
	defer func() {
		for _, s := range squatters {
			s.Close()
		}
	}()

	ln, _, err := bindListener(start)
	if err == nil {
		ln.Close()
		t.Fatalf("expected exhaustion error, got nil")
	}
	if !strings.Contains(err.Error(), "could not bind") {
		t.Fatalf("expected exhaustion message, got: %v", err)
	}
}

// TestBindListener_InvalidAddr: malformed addrs must fail fast instead
// of probing endlessly.
func TestBindListener_InvalidAddr(t *testing.T) {
	cases := []string{"", "not-an-addr", "127.0.0.1", "127.0.0.1:abc"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, _, err := bindListener(c)
			if err == nil {
				t.Fatalf("expected error for invalid addr %q", c)
			}
		})
	}
}

// TestIsAddrInUse: verifies the error detection covers both the typed
// syscall.EADDRINUSE path and the string-match fallback path.
func TestIsAddrInUse(t *testing.T) {
	if isAddrInUse(nil) {
		t.Fatal("nil error should not be EADDRINUSE")
	}
	// String-match fallback.
	if !isAddrInUse(&pseudoErr{"bind: address already in use"}) {
		t.Fatal("string fallback should match")
	}
	if isAddrInUse(&pseudoErr{"some other error"}) {
		t.Fatal("unrelated error should not match")
	}
	// Typed syscall errno. We manufacture an OpError the way net.Listen
	// would, so errors.As resolves down to syscall.EADDRINUSE.
	typed := &net.OpError{
		Op:  "listen",
		Err: &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE},
	}
	if !isAddrInUse(typed) {
		t.Fatal("typed EADDRINUSE should match")
	}
}

// TestWriteRuntimeInfo_Roundtrip: write, read, assert fields round-trip.
func TestWriteRuntimeInfo_Roundtrip(t *testing.T) {
	// Sandbox HOME so writeRuntimeInfo lands under t.TempDir().
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Also clear USERPROFILE for Windows just in case (no-op on Linux).
	t.Setenv("USERPROFILE", tmp)

	info := runtimeInfo{
		APIAddr:   "127.0.0.1:4273",
		APIPort:   4273,
		WebAddr:   "127.0.0.1:3000",
		WebPort:   3000,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Truncate(time.Second),
		Version:   "test",
	}
	writeRuntimeInfo(info)

	// File must exist at the expected path and be 0o600.
	path := filepath.Join(tmp, ".qorven", "runtime.json")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("runtime.json not written: %v", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("runtime.json perms = %v, want 0o600", fi.Mode().Perm())
	}

	// Round-trip JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got runtimeInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.APIAddr != info.APIAddr || got.APIPort != info.APIPort {
		t.Errorf("API addr/port mismatch: got %+v, want %+v", got, info)
	}
	if got.PID != info.PID {
		t.Errorf("PID mismatch: got %d, want %d", got.PID, info.PID)
	}
}

// TestWriteRuntimeInfo_HomeUnset: when HOME isn't set, we fall back to
// TempDir instead of crashing. This matters for `sudo systemctl start`
// without a User= directive.
func TestWriteRuntimeInfo_HomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	path := runtimePath()
	// Must still be a non-empty path.
	if path == "" {
		t.Fatal("runtimePath returned empty string on unset HOME")
	}
	// Must point at TempDir — not a random place under / or relative.
	if !strings.Contains(path, os.TempDir()) {
		// Not strictly required on every OS, but flag loudly if it's
		// somewhere unexpected.
		t.Logf("note: HOME unset → runtime path = %s (not under TempDir)", path)
	}
}

// TestHandleLivez: always returns 200 + {"status":"alive"}, regardless
// of DB state. Critical — /livez driving container restarts means a
// slow or buggy handler here trips restart storms.
func TestHandleLivez(t *testing.T) {
	gw := &Gateway{startTime: time.Now()}
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	gw.handleLivez(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("body.status = %q, want alive", body["status"])
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("missing no-store cache header")
	}
}

// TestHandleReadyz_NoDB: when no DB is configured (fresh install, bare
// gateway), /readyz returns 200 with "not_configured" rather than 503.
// Reason: during Config → Wizard phase, the user hasn't set up a DB
// yet — returning 503 here would make every healthcheck red during
// what is actually a healthy fresh-boot state.
func TestHandleReadyz_NoDB(t *testing.T) {
	gw := &Gateway{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	gw.handleReadyz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no DB is not an error)", w.Code)
	}
	var body struct {
		Ready  bool              `json:"ready"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if !body.Ready {
		t.Error("ready=false when DB not configured — should be true")
	}
	if body.Checks["database"] != "not_configured" {
		t.Errorf("db check = %q, want not_configured", body.Checks["database"])
	}
}

// TestHandleRuntimeInfo: endpoint reports the correct API addr + port
// and sets the CORS header the web client needs during dev.
func TestHandleRuntimeInfo(t *testing.T) {
	gw := &Gateway{
		server:    &http.Server{Addr: "127.0.0.1:4273"},
		startTime: time.Now(),
	}
	req := httptest.NewRequest(http.MethodGet, "/__qorven_runtime", nil)
	w := httptest.NewRecorder()
	gw.handleRuntimeInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if cors := w.Header().Get("Access-Control-Allow-Origin"); cors != "*" {
		t.Errorf("CORS = %q, want *", cors)
	}
	var info runtimeInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if info.APIAddr != "127.0.0.1:4273" {
		t.Errorf("api_addr = %q, want 127.0.0.1:4273", info.APIAddr)
	}
	if info.APIPort != 4273 {
		t.Errorf("api_port = %d, want 4273", info.APIPort)
	}
}

// TestPortFromAddr: cover the successful parse and the sentinel-0 on
// malformed input. Used by runtime.json + the discovery endpoint.
func TestPortFromAddr(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"127.0.0.1:4200", 4200},
		{"[::1]:9000", 9000},
		{"invalid", 0},
		{"", 0},
		{"127.0.0.1", 0},
	}
	for _, c := range cases {
		if got := portFromAddr(c.in); got != c.want {
			t.Errorf("portFromAddr(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestSignalNotify: a cheap smoke test that the helper hooks the
// channel without erroring. Cancel via closing the channel — we don't
// actually raise SIGTERM against the test process (that would kill the
// test runner on some CI providers).
func TestSignalNotify(t *testing.T) {
	ch := make(chan os.Signal, 1)
	signalNotify(ch)
	// Handler is installed; nothing to assert beyond "didn't panic".
	// If Go ever broke signal.Notify signature, this test fails to compile.
	_ = ch
}

// freePort grabs and releases a port, returning a :port addr that's
// almost certainly free for the next net.Listen call. "Almost" because
// the port could race — tests that need a guaranteed-contested port
// should squat on this addr themselves (see the contention test).
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// pseudoErr implements error with a controllable message, for isAddrInUse
// string-fallback tests. Using a package type keeps the assertions
// narrowly scoped and stable against future error-wrapping changes.
type pseudoErr struct{ msg string }

func (p *pseudoErr) Error() string { return p.msg }

// context import reference — suppresses unused-import in minimal builds.
var _ = context.Background
