// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package cmd

import _ "embed"

// changelogEmbedded holds the CHANGELOG.md content baked into the binary at
// build time. The file is copied from the repo root into cmd/ by the release
// workflow before go build runs. In dev builds (file absent) the var is empty
// and start.go falls back to reading CHANGELOG.md from disk.
//
//go:embed CHANGELOG.md
var changelogEmbedded string
