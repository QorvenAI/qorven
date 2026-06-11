// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// stubBackend starts a TCP listener that accepts one connection, reads whatever
// the client sends, and echoes it back verbatim.  It returns the listener (so
// the caller can derive the port) and a WaitGroup that is Done once the echo
// goroutine exits.
func stubBackend(t *testing.T) (net.Listener, *sync.WaitGroup) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("stubBackend listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		defer conn.Close()
		io.Copy(conn, conn) //nolint:errcheck // echo
	}()
	return ln, &wg
}

// fakePreviewManager is a minimal PreviewManager stand-in that always returns
// the configured port (or 0 if portOverride is 0).
type fakePreviewManager struct {
	portOverride int
}

func (f *fakePreviewManager) GetPort(_ string) int { return f.portOverride }

// proxyHandlerForTest wires previewProxyHandler against a fakePreviewManager.
// We cannot call pm.GetPort() on a *PreviewManager whose internal map is empty,
// so we reach into previewProxyHandler with a duck-typed helper.
//
// previewProxyHandler accepts a *PreviewManager; we expose a thin wrapper that
// creates a real *PreviewManager and pre-populates its servers map so GetPort
// returns the right value.
func makeHandler(t *testing.T, port int, projectID string) http.Handler {
	t.Helper()
	pm := NewPreviewManager()
	if port != 0 {
		pm.servers[projectID] = &DevServer{ProjectID: projectID, Port: port}
	}
	return previewProxyHandler(pm, projectID)
}

// TestPreviewProxy_NoServer asserts that a request for a project with no
// running dev server returns 503.
func TestPreviewProxy_NoServer(t *testing.T) {
	h := makeHandler(t, 0, "proj-none")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/preview/proj-none/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

// TestPreviewProxy_HTTP_DeadPort asserts that a normal HTTP request to a port
// with nothing listening returns 502.
func TestPreviewProxy_HTTP_DeadPort(t *testing.T) {
	// Allocate a port then immediately close it so it is definitely not listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	h := makeHandler(t, port, "proj-dead")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/preview/proj-dead/index.html"), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

// TestPreviewProxy_HTTP_XFrameOptions asserts that a real HTTP backend response
// that includes X-Frame-Options has that header stripped by ModifyResponse.
func TestPreviewProxy_HTTP_XFrameOptions(t *testing.T) {
	// Start a tiny HTTP server that replies with X-Frame-Options: DENY.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port := backend.Listener.Addr().(*net.TCPAddr).Port
	h := makeHandler(t, port, "proj-xfo")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/preview/proj-xfo/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if v := rec.Header().Get("X-Frame-Options"); v != "" {
		t.Errorf("expected X-Frame-Options to be stripped, got %q", v)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if strings.Contains(strings.ToLower(csp), "frame-ancestors") {
		t.Errorf("expected frame-ancestors to be stripped from CSP, got %q", csp)
	}
}

// TestPreviewProxy_HTTP_PathStrip asserts that the /v1/preview/{id} prefix is
// stripped before the request reaches the backend.
func TestPreviewProxy_HTTP_PathStrip(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	port := backend.Listener.Addr().(*net.TCPAddr).Port
	h := makeHandler(t, port, "proj-strip")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/preview/proj-strip/some/page.html", nil)
	h.ServeHTTP(rec, req)

	if receivedPath != "/some/page.html" {
		t.Errorf("expected backend to receive /some/page.html, got %q", receivedPath)
	}
}

// TestPreviewProxy_WebSocket_Tunnel asserts that a WebSocket upgrade request is
// tunnelled through the raw TCP hijack path.  The stub backend simply echoes
// whatever it receives; we assert that our request bytes arrive there.
//
// Note: httptest.NewRecorder does NOT implement http.Hijacker; we need a real
// httptest.Server with the handler to exercise the hijack path end-to-end.
func TestPreviewProxy_WebSocket_Tunnel(t *testing.T) {
	// Stub backend: accepts one connection, reads the upgrade request line,
	// sends back a minimal 101 response, then echoes subsequent bytes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	backendPort := ln.Addr().(*net.TCPAddr).Port
	defer ln.Close()

	var backendGot []byte
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(done)
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		// Read until the blank line that ends HTTP headers.
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				break
			}
			mu.Lock()
			backendGot = append(backendGot, []byte(line)...)
			mu.Unlock()
			if line == "\r\n" {
				break
			}
		}
		// Reply with a 101 Switching Protocols.
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")) //nolint:errcheck
		close(done)
	}()

	// Wrap the handler in a real httptest.Server (supports Hijack).
	h := makeHandler(t, backendPort, "proj-ws")
	ts := httptest.NewServer(h)
	defer ts.Close()

	// Dial the test server directly as TCP and send an HTTP upgrade request.
	tsPort := ts.Listener.Addr().(*net.TCPAddr).Port
	client, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", tsPort))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	wsReq := "GET /v1/preview/proj-ws/ HTTP/1.1\r\n" +
		fmt.Sprintf("Host: 127.0.0.1:%d\r\n", tsPort) +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"
	if _, err := client.Write([]byte(wsReq)); err != nil {
		t.Fatal(err)
	}

	// Wait for backend to finish reading the upgrade request.
	<-done

	mu.Lock()
	got := string(backendGot)
	mu.Unlock()

	if !strings.Contains(got, "Upgrade: websocket") {
		t.Errorf("backend did not receive Upgrade header; got:\n%s", got)
	}
	// The path should be stripped to "/" before being forwarded.
	if !strings.Contains(got, "GET / HTTP") {
		t.Errorf("expected backend to receive 'GET / HTTP', got:\n%s", got)
	}
}
