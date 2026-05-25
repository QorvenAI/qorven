// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/tools"
)

// handleServeFile serves a file that an agent staged via the send_file tool.
// GET /v1/files/download/{token}
func (gw *Gateway) handleServeFile(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	info, ok := tools.GetServedFile(token)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "file not found or expired"})
		return
	}

	f, err := os.Open(info.Path)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not open file"})
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not stat file"})
		return
	}

	w.Header().Set("Content-Type", info.MIME)
	w.Header().Set("Content-Disposition", `attachment; filename="`+info.Filename+`"`)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, info.Filename, stat.ModTime(), f)
}
