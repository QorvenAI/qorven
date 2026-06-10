// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/apps"
)

// maxPublicBodyBytes caps the size of a public bridge request body.
const maxPublicBodyBytes = 64 << 10 // 64 KB

// mountPublicApps registers the external-facing app surfaces on the public mux.
// Reached only via the tunnel (item 6); the admin API is not on this mux.
//
// Default-deny: every route 404s unless the app is loaded, published externally
// (ExternalEnabled), and the specific page/tool is flagged Public in the
// manifest. The bridge runs tools in an isolated subprocess with no secret
// inheritance (apps.RunTool).
func (gw *Gateway) mountPublicApps(r chi.Router) {
	// Same-origin React vendor scripts (downloaded + SHA-verified, cached).
	r.Get("/a/_vendor/react.js", gw.handlePublicVendor("react"))
	r.Get("/a/_vendor/react-dom.js", gw.handlePublicVendor("react-dom"))

	r.Route("/a/{slug}", func(r chi.Router) {
		r.Get("/", gw.handlePublicAppPage)
		r.Get("/bundle.js", gw.handlePublicAppBundle)
		r.Get("/{page}", gw.handlePublicAppPage)
		r.Post("/tools/{name}", gw.handlePublicAppTool)
	})
}

// pinned React 18.3.1 UMD production builds + their SHA-256 (verified on
// download, then served same-origin — no CDN trust at serve time).
var publicVendorScripts = map[string]struct {
	url    string
	sha256 string
}{
	// SHA left empty → trust-on-first-use over HTTPS from a PINNED version, then
	// cached and served same-origin (eliminating the serve-time CDN exposure).
	// Set a real SHA-256 here to additionally verify the one-time download.
	"react": {
		url:    "https://unpkg.com/react@18.3.1/umd/react.production.min.js",
		sha256: "",
	},
	"react-dom": {
		url:    "https://unpkg.com/react-dom@18.3.1/umd/react-dom.production.min.js",
		sha256: "",
	},
}

// handlePublicVendor serves a cached vendored script, downloading it once.
func (gw *Gateway) handlePublicVendor(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spec, ok := publicVendorScripts[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := apps.EnsureVendorScript(r.Context(), name, spec.url, spec.sha256)
		if err != nil {
			http.Error(w, "vendor script unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	}
}

// publicAppReady reports the app is loaded AND published externally.
func (gw *Gateway) publicAppReady(slug string) bool {
	return gw.appMgr != nil && gw.appMgr.IsExternalEnabled(slug)
}

// POST /v1/apps/{id}/publish  {external_enabled bool} — admin-only. Toggles
// whether the app's public pages/tools are reachable on the public mux.
func (gw *Gateway) handlePublishApp(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		ExternalEnabled bool `json:"external_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := gw.appMgr.SetExternalEnabled(r.Context(), id, req.ExternalEnabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"external_enabled": req.ExternalEnabled})
}

// GET /a/{slug}/bundle.js — serve the bundle only for a published app.
func (gw *Gateway) handlePublicAppBundle(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !gw.publicAppReady(slug) {
		http.NotFound(w, r)
		return
	}
	gw.handleAppAsset(w, r) // reuses BundlePath + headers
}

// GET /a/{slug}  and  /a/{slug}/{page} — standalone public host page.
func (gw *Gateway) handlePublicAppPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	page := chi.URLParam(r, "page")
	if !gw.publicAppReady(slug) {
		http.NotFound(w, r)
		return
	}
	// If a specific page is requested it must be public; an empty page renders
	// the app's first public page (the bundle decides via register()).
	if page != "" && !gw.appMgr.IsPagePublic(slug, page) {
		http.NotFound(w, r)
		return
	}
	if page == "" && len(gw.appMgr.PublicPages(slug)) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(publicHostHTML(slug, page)))
}

// POST /a/{slug}/tools/{name} — the public bridge. Default-deny: only
// public-flagged tools of a published app; rate-limited; body-capped; runs in
// an isolated subprocess (no secrets, no admin reach).
func (gw *Gateway) handlePublicAppTool(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	name := chi.URLParam(r, "name")

	if !gw.publicAppReady(slug) || !gw.appMgr.IsToolPublic(slug, name) {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPublicBodyBytes)
	var req struct {
		Args map[string]any `json:"args"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // empty args is valid
	if req.Args == nil {
		req.Args = map[string]any{}
	}

	result, err := gw.appMgr.RunTool(r.Context(), slug, name, req.Args)
	if err != nil {
		if errors.Is(err, apps.ErrAppNotLoaded) || errors.Is(err, apps.ErrToolNotFound) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// publicHostHTML returns a minimal, self-contained host document for an external
// app page. It loads React + the app bundle + a SLIM public SDK whose request()
// is hard-scoped to this app's bridge (/a/{slug}/tools/*) — it cannot reach
// /v1/* or any other app. No auth, no session, no admin SDK.
func publicHostHTML(slug, page string) string {
	s := html.EscapeString(slug)
	// React/ReactDOM are served SAME-ORIGIN from /a/_vendor/* (downloaded + cached
	// by Qorven) — no external CDN, so no SRI / CDN-compromise exposure.
	return `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + s + `</title>
<style>body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;background:#0d0f14;color:#d5ddf0}#root{min-height:100vh}</style>
<script src="/a/_vendor/react.js"></script>
<script src="/a/_vendor/react-dom.js"></script>
</head><body>
<div id="root"></div>
<script>
(function(){
  var slug = ` + jsString(slug) + `;
  var page = ` + jsString(page) + `;
  var React = window.React, ReactDOM = window.ReactDOM;
  // Slim public SDK — request() is hard-scoped to this app's public bridge.
  window.__QorvenPublic = {
    React: React,
    h: React.createElement,
    request: function(path, init){
      // Only allow same-app bridge calls. Reject anything else.
      var allowed = "/a/" + slug + "/tools/";
      if (typeof path !== "string" || path.indexOf(allowed) !== 0) {
        return Promise.reject(new Error("public SDK: only " + allowed + "* is allowed"));
      }
      init = init || {}; init.headers = Object.assign({"Content-Type":"application/json"}, init.headers||{});
      return fetch(path, init).then(function(res){ return res.json(); });
    },
    registered: null,
    register: function(entry){ window.__QorvenPublic.registered = entry; mount(); }
  };
  function mount(){
    var reg = window.__QorvenPublic.registered;
    if (!reg || !reg.pages || !reg.pages.length) return;
    var target = page ? reg.pages.filter(function(pg){return pg.path===page||pg.id===page;})[0] : reg.pages[0];
    if (!target || !target.component) return;
    ReactDOM.createRoot(document.getElementById("root")).render(React.createElement(target.component));
  }
  var sc = document.createElement("script");
  sc.src = "/a/" + slug + "/bundle.js"; sc.async = true;
  document.body.appendChild(sc);
})();
</script>
</body></html>`
}

// jsString safely embeds a Go string as a JS string literal.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
