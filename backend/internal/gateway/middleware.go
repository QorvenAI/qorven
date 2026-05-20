// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net"
	"net/http"
	"context"
	"time"
	"path/filepath"
	"strings"
	"sync"
)

// wsAuth wraps a WebSocket handler with token validation via ?token= query param.
func (gw *Gateway) wsAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if token == "" {
			if c, err := r.Cookie("qorven_token"); err == nil {
				token = c.Value
			}
		}

		// Accept static gateway token (fast path)
		if gw.cfg.Auth.Token != "" && token == gw.cfg.Auth.Token {
			handler(w, r)
			return
		}

		// Accept valid JWT tokens (issued by auth service)
		if gw.authSvc != nil && token != "" {
			if _, err := gw.authSvc.ValidateToken(token); err == nil {
				handler(w, r)
				return
			}
		}

		// Accept DB API keys
		if gw.db != nil && token != "" {
			var keyID string
			if err := gw.db.Pool.QueryRow(r.Context(),
				`SELECT id FROM api_keys WHERE key_hash = encode(sha256($1::bytea), 'hex') AND revoked_at IS NULL`,
				token).Scan(&keyID); err == nil {
				handler(w, r)
				return
			}
		}

		// No auth configured — allow (dev mode)
		if gw.cfg.Auth.Token == "" && gw.authSvc == nil {
			handler(w, r)
			return
		}

		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}
}

// AuthMiddleware validates the Authorization header token.
func (gw *Gateway) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gw.cfg.Auth.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		// Check config token first (fast path)
		if token == gw.cfg.Auth.Token {
			next.ServeHTTP(w, r)
			return
		}
		// Check DB API keys (slow path)
		if gw.db != nil {
			var keyID string
			err := gw.db.Pool.QueryRow(r.Context(),
				`SELECT id FROM api_keys WHERE key_hash = encode(sha256($1::bytea), 'hex') AND revoked_at IS NULL`,
				token).Scan(&keyID)
			if err == nil {
				// Valid API key — update last_used
				gw.db.Pool.Exec(r.Context(),
					`UPDATE api_keys SET last_used_at = now() WHERE id = $1`, keyID)
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
}

// apiPrefixAlias rewrites /api/{v1,auth,ws,ws/*} → /{v1,auth,ws,ws/*}
// before any other middleware runs. The web UI uses the /api prefix
// in dev (via Next.js rewrites) and the same call sites must work
// when the UI is served as a static export by this binary. The
// existing Core1 routes mounted under /api (e.g. /api/memory,
// /api/teams, /api/plugins) are explicitly preserved — the alias
// only strips /api when followed by a subpath this function is
// responsible for.
func apiPrefixAlias(next http.Handler) http.Handler {
	// We only strip for paths the frontend expects to hit through
	// Next rewrites. Keep the list explicit so Core1's /api/memory,
	// /api/teams, /api/plugins routes don't get eaten.
	aliased := []string{"/api/v1/", "/api/v1", "/api/auth/", "/api/auth", "/api/ws/", "/api/ws"}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range aliased {
			if r.URL.Path == p || strings.HasPrefix(r.URL.Path, p+"/") || (strings.HasSuffix(p, "/") && strings.HasPrefix(r.URL.Path, p)) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				r.RequestURI = r.URL.RequestURI()
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

// PathTraversalGuard blocks path traversal attempts (CVE-2026-25253).
//
// /_next/ is explicitly whitelisted: Next.js chunk filenames legitimately
// contain ".." (e.g. "0~z6blmwdgh5..js") — these are NOT traversal
// sequences, they are part of the filename. filepath.Clean does not
// collapse them, so the naive strings.Contains("..") check produces false
// positives and returns 403 for every JS chunk. Traversal is only
// meaningful relative to a directory, so we check whether the cleaned path
// resolves higher than "/" which filepath.Clean already prevents.
func PathTraversalGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /_next/ assets are build artifacts — filenames may contain ".."
		// as part of the chunk hash. Skip the guard for this well-known prefix.
		if strings.HasPrefix(r.URL.Path, "/_next/") {
			next.ServeHTTP(w, r)
			return
		}
		clean := filepath.Clean(r.URL.Path)
		if strings.Contains(clean, "..") {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SSRFGuard blocks requests to private/internal IPs.
func IsPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving hostname
		addrs, err := net.LookupHost(host)
		if err != nil || len(addrs) == 0 {
			return false
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return false
		}
	}
	privateRanges := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16", "::1/128", "fc00::/7",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	// Block cloud metadata endpoints
	if ip.String() == "169.254.169.254" {
		return true
	}
	return false
}

// ShellDenyPatterns blocks dangerous shell patterns in tool execution.
var ShellDenyPatterns = []string{
	"rm -rf /", "mkfs", "dd if=", "> /dev/sd",
	":(){ :|:& };:", "chmod 777 /", "curl|sh", "wget|sh",
}

func ContainsDangerousPattern(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, p := range ShellDenyPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// SSRFAllowlist holds hosts that bypass private IP checks (for self-hosted K8s services).
var SSRFAllowlist []string

func SetSSRFAllowlist(hosts []string) { SSRFAllowlist = hosts }

func IsSSRFAllowed(host string) bool {
	for _, pattern := range SSRFAllowlist {
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) || host == pattern[2:] { return true }
		} else if host == pattern { return true }
	}
	return false
}

// DangerousEnvKeys blocks proxy-style and TLS override env vars from tool/plugin execution.
var DangerousEnvKeys = map[string]bool{
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true,
	"http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true,
	"GIT_SSL_NO_VERIFY": true, "NODE_TLS_REJECT_UNAUTHORIZED": true,
	"CURL_CA_BUNDLE": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	"REQUESTS_CA_BUNDLE": true, "GIT_TERMINAL_PROMPT": true,
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "DYLD_INSERT_LIBRARIES": true,
}

func FilterDangerousEnv(env map[string]string) map[string]string {
	safe := make(map[string]string, len(env))
	for k, v := range env {
		if !DangerousEnvKeys[k] { safe[k] = v }
	}
	return safe
}

// MaxBodySize limits request body size to prevent memory exhaustion.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// RequestTimeout adds a timeout to all requests.
func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CORS adds Cross-Origin Resource Sharing headers for web UI access.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := len(allowedOrigins) == 0 // empty = allow all
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
			if allowed && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPRateLimit limits requests per IP address using a simple token bucket.
type IPRateLimit struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rate    int // requests per second
	burst   int
}

type ipBucket struct {
	tokens    float64
	lastCheck time.Time
}

func NewIPRateLimit(ratePerSec, burst int) *IPRateLimit {
	return &IPRateLimit{
		buckets: make(map[string]*ipBucket),
		rate:    ratePerSec,
		burst:   burst,
	}
}

func (rl *IPRateLimit) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: float64(rl.burst), lastCheck: time.Now()}
		rl.buckets[ip] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now
	b.tokens += elapsed * float64(rl.rate)
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// versionHeaderMiddleware stamps X-Qorven-Version on every response so the
// frontend can detect a backend upgrade and reload its stale JS bundle.
func versionHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Qorven-Version", buildInfo.Version)
		next.ServeHTTP(w, r)
	})
}

func (rl *IPRateLimit) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.Split(fwd, ",")[0]
			}
			if !rl.Allow(strings.TrimSpace(ip)) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
