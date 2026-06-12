// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/drive"
)

// enqueueMirrorPush fires a detached, panic-isolated push of a freshly
// written/updated Drive file to every external mirror whose scope covers it.
// One-way OUT only. Never blocks the caller (agent write / human upload).
func (gw *Gateway) enqueueMirrorPush(file *drive.File) {
	if gw.mirrorStore == nil || gw.connExec == nil || file == nil || file.IsFolder {
		return
	}
	f := *file
	safeGo("drive.mirror.push", func() {
		gw.pushFileToMirrors(context.Background(), &f)
	})
}

func (gw *Gateway) pushFileToMirrors(ctx context.Context, f *drive.File) {
	owner := ""
	if f.AgentID != nil {
		owner = *f.AgentID
	}
	scopeID := ""
	if f.ScopeID != nil {
		scopeID = *f.ScopeID
	}
	mirrors, err := gw.mirrorStore.MirrorsForFile(ctx, defaultTenant, f.Scope, scopeID, owner)
	if err != nil || len(mirrors) == 0 {
		return
	}
	// Cap the in-memory read+base64: a mirror push reads the whole file into
	// memory (≈1.33x base64) in a background goroutine, per mirror. Skip
	// oversized files and record the reason rather than risk an OOM.
	const maxMirrorBytes = 50 * 1024 * 1024
	if f.SizeBytes > maxMirrorBytes {
		for _, m := range mirrors {
			_ = gw.mirrorStore.RecordPush(ctx, f.ID, m.ID, "", "error", "file too large to mirror")
		}
		slog.Warn("drive.mirror.too_large", "file", f.ID, "size", f.SizeBytes)
		return
	}
	content, rerr := os.ReadFile(f.Path)
	if rerr != nil {
		slog.Warn("drive.mirror.read_failed", "file", f.ID, "err", rerr)
		return
	}
	for _, m := range mirrors {
		params := map[string]any{
			"name":           f.Name,
			"content_base64": base64.StdEncoding.EncodeToString(content),
			"folder_id":      m.RemoteFolderID,
			"remote_file_id": gw.mirrorStore.RemoteFileID(ctx, f.ID, m.ID),
		}
		execCtx := ctx
		if owner != "" {
			execCtx = connectors.WithAgentID(ctx, owner)
		}
		out, perr := gw.connExec.Execute(execCtx, m.Provider, "upload_file", params)
		if perr != nil {
			slog.Warn("drive.mirror.push_failed", "file", f.ID, "provider", m.Provider, "err", perr)
			_ = gw.mirrorStore.RecordPush(ctx, f.ID, m.ID, "", "error", perr.Error())
			continue
		}
		_ = gw.mirrorStore.RecordPush(ctx, f.ID, m.ID, extractRemoteID(out), "ok", "")
		slog.Info("drive.mirror.pushed", "file", f.ID, "provider", m.Provider)
	}
}

// extractRemoteID best-effort pulls a top-level "id" from a provider upload
// response. A miss just means the next push re-creates rather than updates —
// acceptable for v1.
func extractRemoteID(resp string) string {
	var m map[string]any
	if json.Unmarshal([]byte(resp), &m) == nil {
		if v, ok := m["id"].(string); ok {
			return v
		}
	}
	return ""
}
