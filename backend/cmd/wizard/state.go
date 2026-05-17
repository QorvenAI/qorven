// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package wizard implements the Qorven setup wizard as a Bubbletea TUI.
// It uses github.com/charmbracelet/huh for form steps and
// github.com/charmbracelet/bubbletea for the root event loop.
package wizard

import tea "charm.land/bubbletea/v2"

// ── Wizard state — accumulated across all 9 steps ────────────────────────────

// State holds everything collected during the setup wizard.
// The root model passes a pointer to every step so each step can read
// prior decisions and write its own outputs.
type State struct {
	// Server connection
	BaseURL   string
	AuthToken string // JWT set after step 1 login

	// Step 1 — admin account
	Username    string
	Password    string
	DisplayName string // full name entered at account creation

	// Step 2 — workspace
	WorkspaceName string

	// Step 3 — Prime personality
	PrimeName     string
	PrimeIcon     string // emoji avatar chosen by user
	PrimeStyle    string // professional | casual | technical | creative
	PrimeLanguage string
	PrimeRoleDesc string

	// Step 4 — LLM providers
	AddedProviders []AddedProvider
	PrimaryModel   string
	FastModel      string
	CodingModel    string
	ProviderDBID   string // DB id of first/primary provider

	// Step 4→5 bridge
	PrimeID string // agent id of chief agent (set after persistPrime)

	// Step 5 — channels
	ConnectedChannels []string // types that were successfully connected
	TelegramToken     string
	DiscordToken      string

	// Step 6 — voice (optional)
	TTSDriver string
	TTSKey    string
	STTDriver string
	STTKey    string

	// Step 7 — security
	AccessMode string // "direct" | "domain" | "tailscale"
	TLSDomain  string
	WebPort    string

	// Step 8 — test chat
	TestChatPassed bool

	// Errors accumulated during async operations (non-fatal, shown inline)
	LastError string
}

// AddedProvider is a provider that was successfully tested and created.
type AddedProvider struct {
	ID           string // catalog id (e.g. "openai")
	DisplayName  string
	ProviderDBID string // UUID in database
}

// ── Messages shared across steps ─────────────────────────────────────────────

// NavMsg tells the root wizard to advance or retreat one step.
type NavMsg struct{ Dir int } // +1 = next, -1 = back

// ErrMsg carries a non-fatal error to display in the root error bar.
type ErrMsg struct{ Err error }

// ClearErrMsg clears the root error bar.
type ClearErrMsg struct{}

// Helpers so step code reads cleanly.
func Next() tea.Cmd { return func() tea.Msg { return NavMsg{Dir: 1} } }
func Back() tea.Cmd { return func() tea.Msg { return NavMsg{Dir: -1} } }
