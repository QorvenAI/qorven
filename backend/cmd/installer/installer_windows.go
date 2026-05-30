//go:build windows

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import "fmt"

// Windows installation is handled by the PowerShell script (install.ps1).
// These stubs satisfy the build — the TUI installer never runs on Windows.

func platformSteps() []installStep { return nil }

func platformConfigDir() string { return `C:\ProgramData\Qorven` }

func platformBinPath() string { return `C:\Program Files\Qorven\qorven.exe` }

func platformRestartService(configPath string) {}

func platformMigrate(configPath, dsn string) error { return nil }

func probeSocketDSN() string {
	return "postgres://qorven@localhost:5432/qorven?sslmode=disable"
}

func platformRequirementsText() string {
	return fgSt.Render("Windows not supported via this wizard") + "\n" +
		mutedSt.Render("Use the PowerShell installer instead:")  + "\n" +
		dimSt.Render("  iwr https://get.qorven.ai/win | iex")
}

func platformServiceCommands() string {
	return mutedSt.Render("  Get-Service QorvenAI") + "\n" +
		mutedSt.Render("  qorven migrate up")
}

func platformErrorHints() (common, logs string) {
	common = dimSt.Render("  Use the PowerShell installer:") + "\n" +
		dimSt.Render("  iwr https://get.qorven.ai/win | iex")
	logs = mutedSt.Render("  See C:\\ProgramData\\Qorven\\logs\\qorven.log")
	return
}

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	return "", false, fmt.Errorf("use the PowerShell installer: iwr https://get.qorven.ai/win | iex")
}
