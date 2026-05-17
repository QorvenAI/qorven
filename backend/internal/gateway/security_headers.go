// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import "net/http"

// securityHeaders is a chi middleware that adds the OWASP-recommended
// defensive headers on every response. These are cheap (no state) and
// matter for any deployment that puts the web UI on a public hostname:
//
//   - X-Frame-Options: DENY
//       Prevents clickjacking by refusing to render Qorven in an iframe.
//       Some legitimate embedding scenarios (chatbot widget on a
//       partner site) will want to relax this — leave that to the
//       future embed story, default DENY for the admin UI.
//
//   - X-Content-Type-Options: nosniff
//       Stops IE/old-Edge MIME sniffing from reinterpreting a JSON
//       response as HTML and rendering attacker-controlled markup.
//
//   - Referrer-Policy: strict-origin-when-cross-origin
//       Don't leak full URLs (can contain session IDs in URL params)
//       to third-party sites the user clicks through to.
//
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//       Browsers deny these features by default for this origin so
//       a compromised iframe can't trigger device prompts.
//
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains
//       Set only on HTTPS responses (r.TLS != nil or X-Forwarded-Proto
//       is https). Tells browsers never to downgrade to HTTP for this
//       origin. Omitted on plain HTTP so developer laptops aren't
//       pinned accidentally.
//
//   - Content-Security-Policy
//       Restricts script / connect / frame sources. We use a relaxed
//       policy that still lets the Next.js dev server with its HMR
//       WebSocket work, and allows inline styles (Tailwind's JIT and
//       Next.js CSS modules need them). Tighten for production
//       hardened builds as a follow-up.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// HSTS only on HTTPS: avoids wedging a localhost dev loop into
		// HTTPS-only when the user runs plain http://localhost:4200.
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob: https:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss: https:; "+
				"frame-ancestors 'none'")

		next.ServeHTTP(w, r)
	})
}
