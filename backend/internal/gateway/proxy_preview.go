// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// previewProxyHandler returns an http.Handler that WebSocket-aware reverse-proxies
// requests to the given project's dev server port.
//
// For WebSocket upgrade requests (HMR connections from Vite/Next.js), it hijacks
// the client connection and opens a raw TCP tunnel to the dev server — mirroring
// the pattern in sandbox/app_runner.go — because httputil.ReverseProxy drops
// Upgrade headers and cannot relay the WS handshake.
//
// For normal HTTP requests it uses httputil.NewSingleHostReverseProxy with:
//   - FlushInterval:-1 (streaming/SSE friendly)
//   - ModifyResponse stripping X-Frame-Options and frame-ancestors CSP so the
//     preview iframe is not blocked
//   - ErrorHandler that returns 502 instead of the default 500
//
// In both paths the /v1/preview/{projectID} prefix is stripped before forwarding
// so the dev server sees the paths it expects (e.g. "/" not "/v1/preview/abc/").
func previewProxyHandler(pm *PreviewManager, projectID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port := pm.GetPort(projectID)
		if port == 0 {
			http.Error(w, "dev server not running", http.StatusServiceUnavailable)
			return
		}

		// Strip the gateway prefix — identical for both WS and HTTP paths.
		strippedPath := strings.TrimPrefix(r.URL.Path, "/v1/preview/"+projectID)
		if strippedPath == "" {
			strippedPath = "/"
		}

		// WebSocket upgrade: tunnel raw TCP rather than using the reverse proxy.
		// Mirrors app_runner.go:457-488 exactly.
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
			strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {

			targetConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				http.Error(w, "bad gateway", http.StatusBadGateway)
				return
			}
			defer targetConn.Close()

			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "websocket not supported", http.StatusInternalServerError)
				return
			}
			clientConn, bufrw, err := hj.Hijack()
			if err != nil {
				// After Hijack fails we can no longer write to w.
				return
			}
			defer clientConn.Close()

			// Rewrite the path before forwarding the upgrade request to the backend.
			r.URL.Path = strippedPath
			r.URL.RawPath = ""

			// Flush any bytes the HTTP server already buffered from the client
			// before forwarding the upgrade request (mirrors app_runner.go:477-480).
			if bufrw.Reader.Buffered() > 0 {
				if _, copyErr := io.CopyN(targetConn, bufrw.Reader, int64(bufrw.Reader.Buffered())); copyErr != nil {
					return
				}
			}

			// Forward the original upgrade request to the backend verbatim.
			if err := r.Write(targetConn); err != nil {
				return
			}

			// Bidirectional copy — one goroutine each direction.
			go io.Copy(targetConn, clientConn) //nolint:errcheck
			io.Copy(clientConn, targetConn)    //nolint:errcheck
			return
		}

		// Normal HTTP: use a buffered reverse proxy.
		target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		proxy := httputil.NewSingleHostReverseProxy(target)

		// FlushInterval:-1 disables response buffering so SSE / chunked streams
		// reach the browser incrementally instead of being held until the response ends.
		proxy.FlushInterval = -1 * time.Millisecond

		proxy.ModifyResponse = func(resp *http.Response) error {
			// Remove headers that would prevent the preview from loading inside an iframe.
			resp.Header.Del("X-Frame-Options")

			// Strip frame-ancestors directives from Content-Security-Policy if present.
			csp := resp.Header.Get("Content-Security-Policy")
			if csp != "" {
				// Remove any "frame-ancestors ..." directive (ends at ; or end-of-string).
				parts := strings.Split(csp, ";")
				filtered := parts[:0]
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if !strings.HasPrefix(strings.ToLower(trimmed), "frame-ancestors") {
						filtered = append(filtered, part)
					}
				}
				if len(filtered) == 0 {
					resp.Header.Del("Content-Security-Policy")
				} else {
					resp.Header.Set("Content-Security-Policy", strings.Join(filtered, ";"))
				}
			}
			return nil
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "dev server unavailable", http.StatusBadGateway)
		}

		// Rewrite the request path before the proxy forwards it.
		r.URL.Path = strippedPath
		r.URL.RawPath = ""

		proxy.ServeHTTP(w, r)
	})
}
