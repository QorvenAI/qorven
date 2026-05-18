// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package webui ships the compiled Next.js static export inside the
// Go binary. The embed uses a sentinel file (.embedded) so go:embed
// always has at least one file to match — an empty dir would fail
// to build. Release builds overwrite the whole dist tree with the
// real web/out output before `go build`.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var raw embed.FS

// FS returns the embedded web UI rooted at the dist/ subdirectory so
// callers see index.html at the root, not dist/index.html.
func FS() (fs.FS, error) {
	return fs.Sub(raw, "dist")
}

// HasIndex reports whether the embed contains a real web UI (an
// index.html). Release pipelines populate it; bare `go build` ships
// with only the sentinel file, so the gateway falls back to the
// external web/ directory.
func HasIndex() bool {
	sub, err := FS()
	if err != nil {
		return false
	}
	_, err = fs.Stat(sub, "index.html")
	return err == nil
}
