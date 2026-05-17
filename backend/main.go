// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package main

import (
	"github.com/qorvenai/qorven/cmd"
	"github.com/qorvenai/qorven/internal/gateway"
)

func main() {
	// Inject the embedded migrations FS so the gateway can run schema
	// migrations on first boot without an external migrations/ directory.
	gateway.SetEmbeddedMigrations(EmbeddedMigrations)
	cmd.Execute()
}
