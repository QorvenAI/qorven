// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"os"

	"github.com/charmbracelet/colorprofile"
)

// TerminalCaps captures what the host terminal can render. Set once at startup
// by detectCapabilities() and consulted throughout rendering.
type TerminalCaps struct {
	Profile colorprofile.Profile
	IsTTY   bool
}

var termCaps TerminalCaps

// detectCapabilities probes the terminal for color support. Called once from
// Run() before the bubbletea program starts. Results are process-global —
// the TUI is a single-process tool, so this is acceptable.
func detectCapabilities() TerminalCaps {
	// Detect on stderr to avoid disturbing the alt-screen buffer on stdout.
	profile := colorprofile.Detect(os.Stderr, os.Environ())
	termCaps = TerminalCaps{
		Profile: profile,
		IsTTY:   profile != colorprofile.NoTTY,
	}
	return termCaps
}

// supportsRichColor returns true if the terminal can render 256+ colors.
// Renderers that rely on finely-tuned hex colors (progress gradients,
// glamour themes) can fall back to simpler output when this is false.
func supportsRichColor() bool {
	return termCaps.Profile >= colorprofile.ANSI256
}
