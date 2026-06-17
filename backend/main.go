// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package main

import (
	"github.com/qorvenai/qorven/cmd"
	"github.com/qorvenai/qorven/internal/gateway"
)

func main() {
	// Inject the embedded migrations FS so both the gateway (on first boot) and
	// the `qorven migrate` command can run schema migrations without an external
	// migrations/ directory — the installed binary is fully self-contained.
	gateway.SetEmbeddedMigrations(EmbeddedMigrations)
	cmd.SetEmbeddedMigrations(EmbeddedMigrations)
	cmd.Execute()
}
