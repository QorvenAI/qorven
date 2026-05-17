// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package installer provides a full-screen Bubbletea TUI for `qorven install`.
package installer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Palette ───────────────────────────────────────────────────────────────────

var (
	cPrimary   = lipgloss.Color("#7C3AED")
	cPrimaryLt = lipgloss.Color("#A78BFA") // lighter violet — readable on dark bg
	cGreen     = lipgloss.Color("#34D399")
	cAmber     = lipgloss.Color("#FBBF24")
	cRed       = lipgloss.Color("#F87171")
	cMuted     = lipgloss.Color("#9CA3AF") // was #6B7280 — bumped for readability
	cSubtle    = lipgloss.Color("#6B7280") // was #374151 — bumped for readability
	cBorder    = lipgloss.Color("#4C1D95") // sidebar box borders — deep purple, visible
	cFg        = lipgloss.Color("#F9FAFB")
	cFgDim     = lipgloss.Color("#D1D5DB") // was #9CA3AF — lighter for footer hints
	cHeaderBg  = lipgloss.Color("#4C1D95")
	cFooterBg  = lipgloss.Color("#111827")

	okSt    = lipgloss.NewStyle().Foreground(cGreen)
	warnSt  = lipgloss.NewStyle().Foreground(cAmber)
	failSt  = lipgloss.NewStyle().Foreground(cRed)
	fgSt    = lipgloss.NewStyle().Foreground(cFg)
	boldSt  = lipgloss.NewStyle().Bold(true).Foreground(cFg)
	mutedSt = lipgloss.NewStyle().Foreground(cMuted)
	dimSt   = lipgloss.NewStyle().Foreground(cSubtle)
	primSt  = lipgloss.NewStyle().Foreground(cPrimary).Bold(true)
	primLtSt = lipgloss.NewStyle().Foreground(cPrimaryLt)
)

// ── Screens / Steps ───────────────────────────────────────────────────────────

type screen int

const (
	screenWelcome screen = iota
	screenTailscaleChoice // ask user: use Tailscale, skip, or decide later
	screenInstall
	screenTailscale // waiting for user to authorize in browser
	screenConfig    // fallback: manual IP / URL pick (skip Tailscale)
	screenDone
	screenError
)

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepWarn
	stepFail
)

type installStep struct {
	label  string
	status stepStatus
	detail string
}

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	Version          string
	DataDir          string
	SkipDocker       bool
	SkipPG           bool
	TailscaleAuthKey string // optional pre-auth key for headless setup
	SkipTailscale    bool
}

// ── Messages ──────────────────────────────────────────────────────────────────

type stepResultMsg struct {
	idx    int
	detail string
	warn   bool
	err    error
}

type tickMsg time.Time

type healthCheckMsg struct {
	up  bool
	err string
}

// tailscaleAuthURLMsg is sent when step 10 produces a browser auth URL.
type tailscaleAuthURLMsg struct{ url string }

// tailscaleIPMsg is sent when the polling loop detects a 100.x.x.x IP.
type tailscaleIPMsg struct{ ip string }

type healthStatus int

const (
	healthChecking healthStatus = iota
	healthUp
	healthDown
)

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	cfg      Config
	screen   screen
	steps    []installStep
	spinner  spinner.Model
	errMsg   string
	width    int
	height   int
	quitting bool

	// install timing
	stepStarted time.Time
	elapsed     time.Duration

	// config screen
	ips       ipResult
	urlInput  string // editable URL the user confirms
	urlCursor int

	// done screen — health check
	health    healthStatus
	healthErr string

	// tailscale choice screen (0=yes recommended, 1=skip, 2=decide later)
	tsChoice int

	// tailscale screen
	tsAuthURL  string // browser URL to authorize
	tsIP       string // 100.x.x.x once connected
	tsWaitSecs int    // elapsed wait seconds shown on screen
}

func New(cfg Config) *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(cPrimary)

	return &Model{
		cfg:     cfg,
		screen:  screenWelcome,
		spinner: sp,
		steps: []installStep{
			{label: "Detect system"},
			{label: "Update package index"},
			{label: "Install system packages"},
			{label: "Install PostgreSQL"},
			{label: "Install Docker"},
			{label: "Create OS user  qorven"},
			{label: "Create data directories"},
			{label: "Setup database"},
			{label: "Install binary to /usr/local/bin"},
			{label: "Install & start systemd service"},
			{label: "Install Tailscale"},
		},
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, tickCmd()) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if m.screen == screenInstall && !m.stepStarted.IsZero() {
			m.elapsed = time.Since(m.stepStarted)
		}
		if m.screen == screenTailscale {
			m.tsWaitSecs++
		}
		return m, tickCmd()

	case tea.KeyMsg:
		switch m.screen {
		case screenWelcome:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "enter", " ":
				if m.cfg.SkipTailscale {
					// Flag already decided — go straight to install
					m.screen = screenInstall
					m.stepStarted = time.Now()
					return m, tea.Batch(m.spinner.Tick, tickCmd(), m.kickStep(0))
				}
				m.screen = screenTailscaleChoice
				return m, nil
			}

		case screenTailscaleChoice:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "up", "k":
				if m.tsChoice > 0 {
					m.tsChoice--
				}
			case "down", "j":
				if m.tsChoice < 2 {
					m.tsChoice++
				}
			case "enter", " ":
				switch m.tsChoice {
				case 0: // Yes — install Tailscale
					m.cfg.SkipTailscale = false
				case 1, 2: // Skip / Decide later
					m.cfg.SkipTailscale = true
				}
				m.screen = screenInstall
				m.stepStarted = time.Now()
				return m, tea.Batch(m.spinner.Tick, tickCmd(), m.kickStep(0))
			}

		case screenInstall:
			// Allow force-quit even during install
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}

		case screenTailscale:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "s", "S":
				// Skip Tailscale — fall back to manual IP config
				best := m.ips.publicURL
				if best == "" && len(m.ips.lanIPs) > 0 {
					best = m.ips.lanIPs[0]
				}
				m.urlInput = best
				m.urlCursor = len(m.urlInput)
				m.screen = screenConfig
				return m, nil
			}

		case screenConfig:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "enter":
				if err := m.writeMinimalConfig(); err != nil {
					m.screen = screenError
					m.errMsg = err.Error()
					return m, nil
				}
				m.screen = screenDone
				m.health = healthChecking
				return m, m.waitForHealth(12 * time.Second)
			case "backspace":
				if m.urlCursor > 0 {
					m.urlInput = m.urlInput[:m.urlCursor-1] + m.urlInput[m.urlCursor:]
					m.urlCursor--
				}
			case "left":
				if m.urlCursor > 0 {
					m.urlCursor--
				}
			case "right":
				if m.urlCursor < len(m.urlInput) {
					m.urlCursor++
				}
			default:
				ch := msg.String()
				if len(ch) == 1 && ch[0] >= 0x20 {
					m.urlInput = m.urlInput[:m.urlCursor] + ch + m.urlInput[m.urlCursor:]
					m.urlCursor++
				}
			}

		case screenDone, screenError:
			switch msg.String() {
			case "ctrl+c", "enter", " ":
				m.quitting = true
				return m, tea.Quit
			}
		}

	case tailscaleIPMsg:
		m.tsIP = msg.ip
		m.urlInput = msg.ip
		m.urlCursor = len(m.urlInput)
		if err := m.writeMinimalConfig(); err != nil {
			m.screen = screenError
			m.errMsg = err.Error()
			return m, nil
		}
		m.screen = screenDone
		m.health = healthChecking
		return m, m.waitForHealth(12 * time.Second)

	case healthCheckMsg:
		if msg.up {
			m.health = healthUp
		} else {
			m.health = healthDown
			m.healthErr = msg.err
		}
		return m, nil

	case stepResultMsg:
		s := &m.steps[msg.idx]
		s.detail = msg.detail
		if msg.err != nil {
			s.status = stepFail
			m.screen = screenError
			m.errMsg = msg.err.Error()
			return m, nil
		}
		if msg.warn {
			s.status = stepWarn
		} else {
			s.status = stepDone
		}
		next := msg.idx + 1
		for next < len(m.steps) && m.steps[next].status != stepPending {
			next++
		}
		if next < len(m.steps) {
			m.stepStarted = time.Now()
			m.elapsed = 0
			m.steps[next].status = stepRunning
			return m, m.kickStep(next)
		}
		// All steps done — check what Tailscale step returned.
		// steps[10].detail is either:
		//   "connected:<100.x.x.x>"  → already connected, skip auth screen
		//   "url:<https://...>"       → need browser auth
		//   "skipped"                 → --skip-tailscale or error, go to config
		m.ips = detectIPs()
		detail := m.steps[10].detail
		switch {
		case strings.HasPrefix(detail, "connected:"):
			ip := strings.TrimPrefix(detail, "connected:")
			m.tsIP = ip
			m.urlInput = ip
			m.urlCursor = len(m.urlInput)
			if err := m.writeMinimalConfig(); err != nil {
				m.screen = screenError
				m.errMsg = err.Error()
				return m, nil
			}
			m.screen = screenDone
			m.health = healthChecking
			return m, m.waitForHealth(12 * time.Second)
		case strings.HasPrefix(detail, "url:"):
			m.tsAuthURL = strings.TrimPrefix(detail, "url:")
			m.screen = screenTailscale
			return m, m.pollTailscaleIP()
		default:
			// Tailscale skipped / not a VPS — go to manual config
			best := m.ips.publicURL
			if best == "" && len(m.ips.lanIPs) > 0 {
				best = m.ips.lanIPs[0]
			}
			m.urlInput = best
			m.urlCursor = len(m.urlInput)
			m.screen = screenConfig
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m *Model) kickStep(idx int) tea.Cmd {
	m.steps[idx].status = stepRunning
	m.stepStarted = time.Now()
	m.elapsed = 0
	cfg := m.cfg
	return func() tea.Msg {
		detail, warn, err := executeStep(idx, cfg)
		return stepResultMsg{idx: idx, detail: detail, warn: warn, err: err}
	}
}

func (m *Model) writeMinimalConfig() error {
	etcDir := "/etc/qorven"
	os.MkdirAll(etcDir, 0755)

	baseURL := m.urlInput
	if baseURL == "" {
		baseURL = m.ips.publicURL
	}
	if baseURL == "" {
		baseURL = "localhost"
	}
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}

	cfgContent := fmt.Sprintf(`# Qorven Configuration — generated by qorven install
[server]
api_listen = "127.0.0.1:4200"
web_listen = "0.0.0.0:80"
base_url = "%s"

[server.tls]
mode = "disabled"

[database]
# DSN is in .env
`, baseURL)
	if err := os.WriteFile(filepath.Join(etcDir, "config.toml"), []byte(cfgContent), 0644); err != nil {
		return fmt.Errorf("write config.toml: %w", err)
	}
	env := strings.Join([]string{
		"# Qorven secrets — keep private",
		"QORVEN_POSTGRES_DSN=" + probeSocketDSN(),
		"QORVEN_GATEWAY_TOKEN=" + randHex(16),
		"QORVEN_ENCRYPTION_KEY=" + randHex(32),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(etcDir, ".env"), []byte(env), 0600); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	// Run migrations synchronously before starting the service.
	// Without this, the wizard's /auth/setup hits the DB before the users
	// table exists and returns "relation does not exist".
	migrateCmd := exec.Command("sudo", "-u", "qorven",
		"env",
		"QORVEN_CONFIG="+filepath.Join(etcDir, "config.toml"),
		"QORVEN_POSTGRES_DSN="+probeSocketDSN(),
		"/usr/local/bin/qorven", "migrate", "up",
	)
	// Retry up to 3 times — pg socket may need a moment to become ready
	for i := 0; i < 3; i++ {
		if err := migrateCmd.Run(); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
		migrateCmd = exec.Command("sudo", "-u", "qorven",
			"env",
			"QORVEN_CONFIG="+filepath.Join(etcDir, "config.toml"),
			"QORVEN_POSTGRES_DSN="+probeSocketDSN(),
			"/usr/local/bin/qorven", "migrate", "up",
		)
	}

	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "restart", "qorven").Run()
	return nil
}

// ── Layout primitives ─────────────────────────────────────────────────────────

// leftW / rightW — consistent split across every screen
func (m *Model) leftW() int  { return m.width * 3 / 5 }
func (m *Model) rightW() int { return m.width - m.leftW() }

// leftPanel wraps content in the left column style (right border, fills height)
func (m *Model) leftPanel(content string, h int) string {
	return lipgloss.NewStyle().
		Width(m.leftW()-1). // -1 for the border char
		Height(h).
		Padding(2, 3).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(cSubtle).
		Render(content)
}

// rightPanel wraps content in the right column style (fills height, no border)
func (m *Model) rightPanel(content string, h int) string {
	return lipgloss.NewStyle().
		Width(m.rightW()).
		Height(h).
		Padding(2, 3).
		Render(content)
}

// sectionTitle renders a small section label
func sectionTitle(s string) string {
	return lipgloss.NewStyle().
		Foreground(cPrimary).
		Bold(true).
		Render(s)
}

// infoBox renders a labelled box on the right panel
func (m *Model) infoBox(title, body string) string {
	inner := sectionTitle(title) + "\n\n" + body
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cBorder).
		Padding(1, 2).
		Width(m.rightW() - 8).
		Render(inner)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m *Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	contentH := m.height - headerH - footerH
	if contentH < 1 {
		contentH = 1
	}

	var left, right string
	switch m.screen {
	case screenWelcome:
		left, right = m.viewWelcomeLeft(), m.viewWelcomeRight()
	case screenTailscaleChoice:
		left, right = m.viewTailscaleChoiceLeft(), m.viewTailscaleChoiceRight()
	case screenInstall:
		left, right = m.viewInstallLeft(), m.viewInstallRight()
	case screenTailscale:
		left, right = m.viewTailscaleLeft(), m.viewTailscaleRight()
	case screenConfig:
		left, right = m.viewConfigLeft(), m.viewConfigRight()
	case screenDone:
		left, right = m.viewDoneLeft(), m.viewDoneRight()
	case screenError:
		left, right = m.viewErrorLeft(), m.viewErrorRight()
	}

	inner := lipgloss.JoinHorizontal(lipgloss.Top,
		m.leftPanel(left, contentH-2),
		m.rightPanel(right, contentH-2),
	)
	// 4-sided border around the entire content area
	content := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cSubtle).
		Width(m.width - 2).
		Render(inner)

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, content, footer))
	v.AltScreen = true
	return v
}

// renderHeader — full-width purple bar, consistent on every screen
func (m *Model) renderHeader() string {
	wh := lipgloss.Color("#FFFFFF")
	lavender := lipgloss.Color("#C4B5FD")

	// Brand badge: "⚡ Qorven" in white bold
	brand := lipgloss.NewStyle().Bold(true).Foreground(wh).Render("⚡ Qorven")
	// Version pill
	ver := lipgloss.NewStyle().Foreground(lavender).Render("  " + m.cfg.Version)
	// Separator + context
	sep := lipgloss.NewStyle().Foreground(lavender).Render("  │  ")
	ctx := lipgloss.NewStyle().Foreground(lavender).Render("Server Installer")
	left := brand + ver + sep + ctx

	var right string
	switch m.screen {
	case screenInstall:
		done := m.countDone()
		right = lipgloss.NewStyle().Foreground(lavender).
			Render(fmt.Sprintf("Step %d / %d", done, len(m.steps)))
	case screenTailscale:
		right = lipgloss.NewStyle().Foreground(cGreen).Render("✓ Installed") +
			lipgloss.NewStyle().Foreground(lavender).Render("  │  Authorizing…")
	case screenConfig:
		right = lipgloss.NewStyle().Foreground(cGreen).Render("✓ Installed")
	case screenDone:
		right = lipgloss.NewStyle().Bold(true).Foreground(cGreen).Render("✓ Complete")
	case screenError:
		right = lipgloss.NewStyle().Bold(true).Foreground(cRed).Render("✗ Failed")
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 0 {
		gap = 0
	}

	return lipgloss.NewStyle().
		Background(cPrimary).
		Foreground(wh).
		Width(m.width).
		Padding(0, 2).
		Render(left + strings.Repeat(" ", gap) + right)
}

// renderFooter — full-width dark hint bar, consistent on every screen
func (m *Model) renderFooter() string {
	hints := map[screen]string{
		screenWelcome:         "Enter  agree & install  ·  Ctrl+C  cancel",
		screenTailscaleChoice: "↑ ↓ / J K  navigate  ·  Enter  confirm  ·  Ctrl+C  cancel",
		screenInstall:         "Installing…  ·  Ctrl+C  force quit (re-run to resume)",
		screenTailscale: "Waiting for Tailscale authorization  ·  S  skip (use IP manually)  ·  Ctrl+C  cancel",
		screenConfig:    "Edit URL if needed  ·  Enter  confirm & finish  ·  Ctrl+C  cancel",
		screenDone:    "Enter  exit to shell",
		screenError:   "Enter  exit  ·  sudo qorven install  to retry",
	}
	return lipgloss.NewStyle().
		Background(cFooterBg).
		Foreground(lipgloss.Color("#D1D5DB")).
		Width(m.width).
		Padding(0, 2).
		Render(hints[m.screen])
}

// ── Welcome ───────────────────────────────────────────────────────────────────

func (m *Model) viewWelcomeLeft() string {
	// Available inner width: leftW minus panel padding (3 each side = 6) minus border (1)
	innerW := m.leftW() - 7
	if innerW < 20 {
		innerW = 20
	}
	divider := lipgloss.NewStyle().Foreground(cSubtle).Render(strings.Repeat("─", innerW))

	var b strings.Builder

	// ── Brand mark box ───────────────────────────────────────────────────────
	brandInner := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("⚡ Qorven"),
		lipgloss.NewStyle().Foreground(cPrimaryLt).Render("Self-Hosted AI Agent Platform"),
		lipgloss.NewStyle().Foreground(cMuted).Render("One binary. No cloud lock-in."),
	)
	brandBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cPrimary).
		Padding(0, 2).
		Render(brandInner)
	b.WriteString(brandBox + "\n\n")

	// ── Features ─────────────────────────────────────────────────────────────
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cPrimaryLt).Render("What you're installing") + "\n")
	b.WriteString(divider + "\n")
	features := []struct{ icon, text string }{
		{"🤖", "Autonomous AI agents that work around the clock"},
		{"🌐", "Web browsing, code execution & file access"},
		{"💬", "Chat, email, Slack, WhatsApp & Telegram"},
		{"🗓", "Scheduled tasks, briefings & cron workflows"},
		{"🔒", "Fully self-hosted — data never leaves this server"},
	}
	for _, f := range features {
		b.WriteString("  " + f.icon + "  " + fgSt.Render(f.text) + "\n")
	}

	b.WriteString("\n")

	// ── What the installer does ───────────────────────────────────────────────
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cPrimaryLt).Render("This wizard will") + "\n")
	b.WriteString(divider + "\n")
	for _, line := range []string{
		"Install PostgreSQL & pgvector",
		"Create OS user and database",
		"Install binary & register systemd service",
		"Open browser — set up provider & admin account",
	} {
		b.WriteString(okSt.Render("✓") + "  " + fgSt.Render(line) + "\n")
	}

	b.WriteString("\n")

	// ── What agents can do — notice ──────────────────────────────────────────
	noticeSt := lipgloss.NewStyle().Foreground(cAmber)
	noticeInner := noticeSt.Bold(true).Render("⚡ Your agent's capabilities") + "\n" +
		noticeSt.Render("Browse the web  ·  run code  ·  send emails") + "\n" +
		noticeSt.Render("call APIs  ·  manage files on this server") + "\n" +
		mutedSt.Render("You are responsible for every agent action.")
	noticeBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cAmber).
		Padding(0, 2).
		Width(innerW - 2).
		Render(noticeInner)
	b.WriteString(noticeBox + "\n\n")

	// ── CTA ──────────────────────────────────────────────────────────────────
	cta := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(cPrimary).
		Padding(0, 3).
		Render("▶  I understand — Install Qorven")
	b.WriteString(cta)
	return b.String()
}

func (m *Model) viewWelcomeRight() string {
	req := func(icon, label, detail string) string {
		return icon + "  " + fgSt.Render(label) + "\n" +
			"   " + mutedSt.Render(detail)
	}
	reqs := req("🐧", "Ubuntu 20.04+ / Debian 11+", "or any systemd-based Linux") + "\n" +
		req("🔑", "root or sudo access", "to install packages & services") + "\n" +
		req("💾", "2 GB RAM  ·  10 GB disk", "minimum recommended") + "\n" +
		req("🌐", "Internet access", "to pull packages on first install")

	optional := fgSt.Render("AI provider API key") + "\n" +
		mutedSt.Render("OpenAI, Anthropic, Gemini, or Ollama") + "\n\n" +
		okSt.Render("✓") + "  " + primLtSt.Render("Add later in the web UI") + "\n" +
		mutedSt.Render("  Not required to complete installation")

	trusted := fgSt.Render("⭐  Open Source") + "\n" +
		mutedSt.Render("    FSL-1.1-ALv2 — source-available,") + "\n" +
		mutedSt.Render("    commercial use allowed") + "\n\n" +
		fgSt.Render("🔗  github.com/qorvenai/qorven") + "\n" +
		primLtSt.Render("    qorven.ai")

	return m.infoBox("System requirements", reqs) +
		"\n" +
		m.infoBox("You will need", optional) +
		"\n" +
		m.infoBox("About Qorven", trusted)
}

// ── Tailscale choice ─────────────────────────────────────────────────────────

func (m *Model) viewTailscaleChoiceLeft() string {
	var b strings.Builder
	b.WriteString(boldSt.Render("Secure remote access") + "\n")
	b.WriteString(mutedSt.Render("How would you like to reach Qorven from other devices?") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", 44)) + "\n\n")

	options := []struct {
		label    string
		sublabel string
		desc     string
	}{
		{
			label:    "Install Tailscale",
			sublabel: "Recommended",
			desc:     "Private encrypted network — no firewall rules, no port forwarding. Works from anywhere.",
		},
		{
			label:    "Use public IP / domain",
			sublabel: "Advanced",
			desc:     "Expose Qorven on a public IP or your own domain. You manage firewall and TLS.",
		},
		{
			label:    "Decide later",
			sublabel: "Skip for now",
			desc:     "Continue without network setup. Configure access from Settings after install.",
		},
	}

	for i, opt := range options {
		selected := m.tsChoice == i
		var cursor, labelRender, subRender, descRender string
		if selected {
			cursor = primSt.Render("▶  ")
			labelRender = boldSt.Copy().Foreground(cPrimaryLt).Render(opt.label)
			subRender = " " + primLtSt.Render(opt.sublabel)
			descRender = "   " + fgSt.Render(opt.desc)
		} else {
			cursor = dimSt.Render("   ")
			labelRender = fgSt.Render(opt.label)
			subRender = " " + mutedSt.Render(opt.sublabel)
			descRender = "   " + mutedSt.Render(opt.desc)
		}
		b.WriteString(cursor + labelRender + subRender + "\n")
		b.WriteString(descRender + "\n\n")
	}
	return b.String()
}

func (m *Model) viewTailscaleChoiceRight() string {
	whatIs := fgSt.Render("  Tailscale is a free private mesh network") + "\n" +
		mutedSt.Render("  built on WireGuard encryption.") + "\n\n" +
		okSt.Render("  ✓  No port forwarding required") + "\n" +
		okSt.Render("  ✓  Works behind NAT and firewalls") + "\n" +
		okSt.Render("  ✓  Peer-to-peer — fast & private") + "\n" +
		okSt.Render("  ✓  Free for personal use") + "\n\n" +
		dimSt.Render("  tailscale.com")

	howIt := dimSt.Render("  1.  Install here → get a 100.x.x.x IP") + "\n" +
		dimSt.Render("  2.  Install Tailscale on your laptop/phone") + "\n" +
		dimSt.Render("  3.  Reach Qorven at your 100.x.x.x IP") + "\n\n" +
		mutedSt.Render("  Your data never leaves your devices.") + "\n" +
		mutedSt.Render("  No Qorven cloud relay ever involved.")

	return m.infoBox("What is Tailscale?", whatIs) +
		"\n\n" +
		m.infoBox("How it works", howIt)
}

// ── Install ───────────────────────────────────────────────────────────────────

func (m *Model) viewInstallLeft() string {
	var b strings.Builder
	b.WriteString(boldSt.Render("Installing Qorven…") + "\n")
	b.WriteString(mutedSt.Render("Please wait — this takes about 2 minutes.") + "\n\n")
	for _, s := range m.steps {
		icon, label, detail := dimSt.Render(" ○"), dimSt.Render(s.label), ""
		switch s.status {
		case stepRunning:
			icon = " " + m.spinner.View()
			label = boldSt.Render(s.label)
		case stepDone:
			icon = okSt.Render(" ✓")
			label = fgSt.Render(s.label)
			if s.detail != "" {
				detail = "  " + mutedSt.Render(s.detail)
			}
		case stepWarn:
			icon = warnSt.Render(" !")
			label = warnSt.Render(s.label)
			if s.detail != "" {
				detail = "  " + warnSt.Render(s.detail)
			}
		case stepFail:
			icon = failSt.Render(" ✗")
			label = failSt.Render(s.label)
		}
		b.WriteString(icon + "  " + label + detail + "\n")
	}
	return b.String()
}

func (m *Model) viewInstallRight() string {
	done := m.countDone()
	total := len(m.steps)
	barW := m.rightW() - 8
	if barW < 4 {
		barW = 4
	}
	filled := 0
	if total > 0 {
		filled = barW * done / total
	}
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}

	bar := okSt.Render(strings.Repeat("█", filled)) +
		dimSt.Render(strings.Repeat("░", barW-filled))

	var b strings.Builder
	b.WriteString(sectionTitle("Progress") + "\n\n")
	b.WriteString(boldSt.Render(fmt.Sprintf("%d%%", pct)) + "  " +
		mutedSt.Render(fmt.Sprintf("%d of %d steps", done, total)) + "\n\n")
	b.WriteString(bar + "\n\n")

	// Current running step + elapsed time
	for i := len(m.steps) - 1; i >= 0; i-- {
		if m.steps[i].status == stepRunning {
			elapsed := ""
			if m.elapsed >= time.Second {
				elapsed = "  " + mutedSt.Render(fmt.Sprintf("%ds", int(m.elapsed.Seconds())))
			}
			b.WriteString(dimSt.Render("Running:") + "\n")
			b.WriteString(fgSt.Render(m.steps[i].label) + elapsed + "\n\n")
			break
		}
	}
	// Last completed detail
	for i := done - 1; i >= 0; i-- {
		if m.steps[i].detail != "" {
			b.WriteString(dimSt.Render("Last output:") + "\n")
			b.WriteString(mutedSt.Render(m.steps[i].detail) + "\n")
			break
		}
	}
	return b.String()
}

// ── Tailscale screen ──────────────────────────────────────────────────────────

// pollTailscaleIP polls `tailscale ip -4` every 2s until a 100.x.x.x appears.
func (m *Model) pollTailscaleIP() tea.Cmd {
	return func() tea.Msg {
		for {
			time.Sleep(2 * time.Second)
			out, err := exec.Command("tailscale", "ip", "-4").Output()
			if err != nil {
				continue
			}
			ip := strings.TrimSpace(string(out))
			if strings.HasPrefix(ip, "100.") {
				return tailscaleIPMsg{ip: ip}
			}
		}
	}
}

func (m *Model) viewTailscaleLeft() string {
	var b strings.Builder
	b.WriteString(okSt.Bold(true).Render("✓  Tailscale installed") + "\n")
	b.WriteString(mutedSt.Render("One quick step — authorize this server in your browser.") + "\n\n")
	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")

	b.WriteString(sectionTitle("Authorize this server") + "\n\n")
	b.WriteString(mutedSt.Render("  Open this URL on any device:") + "\n\n")

	// Big highlighted auth URL box
	urlBox := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(cPrimary).
		Padding(1, 3).
		Width(m.leftW() - 12).
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Render(m.tsAuthURL)
	b.WriteString(urlBox + "\n\n")

	b.WriteString(fgSt.Render("  1.  Open that URL on your phone, laptop, or any device") + "\n")
	b.WriteString(fgSt.Render("  2.  Log in or sign up — it's free") + "\n")
	b.WriteString(fgSt.Render("  3.  Click Connect — this screen updates automatically") + "\n\n")
	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")

	if m.tsIP != "" {
		b.WriteString(okSt.Bold(true).Render("✓  Connected: ") + primSt.Render(m.tsIP) + "\n")
	} else {
		wait := ""
		if m.tsWaitSecs > 0 {
			wait = fmt.Sprintf("  %ds", m.tsWaitSecs)
		}
		b.WriteString(m.spinner.View() + " " + mutedSt.Render("Waiting for authorization…"+wait) + "\n")
	}
	return b.String()
}

func (m *Model) viewTailscaleRight() string {
	what := dimSt.Render("  Tailscale is a free private network") + "\n" +
		dimSt.Render("  that connects your devices securely.") + "\n\n" +
		dimSt.Render("  No port forwarding. No firewall rules.") + "\n" +
		dimSt.Render("  Works from VPS, home, office, anywhere.") + "\n\n" +
		okSt.Render("  ✓  Encrypted peer-to-peer") + "\n" +
		okSt.Render("  ✓  Works behind NAT") + "\n" +
		okSt.Render("  ✓  Free for personal use")

	var ipBox string
	if m.tsIP != "" {
		ipBox = m.infoBox("Your Tailscale IP",
			okSt.Bold(true).Render("  "+m.tsIP)+"\n\n"+
				dimSt.Render("  Share this IP with anyone on your")+"\n"+
				dimSt.Render("  Tailscale network to access Qorven."))
	} else {
		ipBox = m.infoBox("Your Tailscale IP",
			mutedSt.Render("  Waiting for authorization…")+"\n\n"+
				dimSt.Render("  Will appear here once connected."))
	}

	return m.infoBox("What is Tailscale?", what) +
		"\n\n" +
		ipBox
}

// ── Config (IP / URL) ─────────────────────────────────────────────────────────

func (m *Model) viewConfigLeft() string {
	var b strings.Builder
	b.WriteString(okSt.Bold(true).Render("✓  Installation complete!") + "\n")
	b.WriteString(mutedSt.Render("All packages installed. Confirm how Qorven will be reached.") + "\n\n")
	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")

	b.WriteString(sectionTitle("Detected IP addresses") + "\n\n")

	if m.ips.publicURL != "" && !m.ips.behindNAT {
		// VPS / cloud — public IP directly on this machine
		b.WriteString(okSt.Render("  ● ") + fgSt.Render("Public:  ") + primSt.Render(m.ips.publicURL) + "\n")
	}
	if m.ips.wanIP != "" && m.ips.behindNAT {
		// Broadband — router's WAN IP, not directly reachable without port forward
		b.WriteString(warnSt.Render("  ⚠ ") + fgSt.Render("Router WAN:  ") + warnSt.Render(m.ips.wanIP) +
			mutedSt.Render("  (needs port 80 forward)") + "\n")
	}
	if m.ips.publicURL != "" && m.ips.behindNAT {
		b.WriteString(okSt.Render("  ● ") + fgSt.Render("LAN (pre-filled):  ") + primSt.Render(m.ips.publicURL) + "\n")
	}
	for _, ip := range m.ips.lanIPs {
		b.WriteString(dimSt.Render("  ○ LAN:  ") + mutedSt.Render(ip) + "\n")
	}
	if m.ips.publicURL == "" && len(m.ips.lanIPs) == 0 {
		b.WriteString(warnSt.Render("  Could not detect any IP — using localhost") + "\n")
	}

	b.WriteString("\n" + dimSt.Render(strings.Repeat("─", 44)) + "\n\n")
	// Public IP warning — if user is on VPS with direct public IP and NOT using Tailscale
	if m.ips.publicURL != "" && !m.ips.behindNAT && m.tsIP == "" {
		b.WriteString(warnSt.Bold(true).Render("⚠  Security notice") + "\n")
		b.WriteString(warnSt.Render("   Exposing Qorven directly on a public IP means") + "\n")
		b.WriteString(warnSt.Render("   anyone who finds this URL can reach the login page.") + "\n")
		b.WriteString(mutedSt.Render("   Consider using Tailscale for private access instead.") + "\n")
		b.WriteString(mutedSt.Render("   Re-run: sudo qorven install  (without --skip-tailscale)") + "\n\n")
	}

	b.WriteString(boldSt.Render("Public URL for Qorven (edit if needed):") + "\n\n")

	// Editable URL input
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cPrimary).
		Padding(0, 1).
		Width(m.leftW() - 12)

	// Build cursor-annotated display
	disp := m.urlInput
	cur := m.urlCursor
	if cur > len(disp) {
		cur = len(disp)
	}
	rendered := disp[:cur] +
		lipgloss.NewStyle().Background(cPrimary).Foreground(cFg).Render(" ") +
		disp[cur:]

	b.WriteString(inputStyle.Render(rendered) + "\n\n")
	b.WriteString(mutedSt.Render("Press Enter to confirm."))
	return b.String()
}

func (m *Model) viewConfigRight() string {
	tips := dimSt.Render("  Use your server's public IP") + "\n" +
		dimSt.Render("  or a domain name pointing to it.") + "\n\n" +
		dimSt.Render("  For Tailscale, enter your") + "\n" +
		dimSt.Render("  100.x.x.x Tailscale IP here.") + "\n\n" +
		dimSt.Render("  For local-only access, keep") + "\n" +
		dimSt.Render("  the LAN IP or use localhost.")

	scenarios := dimSt.Render("  VPS / cloud server") + "\n" +
		mutedSt.Render("    → public IP auto-detected") + "\n\n" +
		dimSt.Render("  Behind NAT / broadband") + "\n" +
		mutedSt.Render("    → enter WAN IP or domain") + "\n\n" +
		dimSt.Render("  Tailscale") + "\n" +
		mutedSt.Render("    → enter 100.x.x.x IP") + "\n\n" +
		dimSt.Render("  Local machine") + "\n" +
		mutedSt.Render("    → keep LAN IP or localhost")

	return m.infoBox("URL tips", tips) +
		"\n\n" +
		m.infoBox("Scenarios", scenarios)
}

// ── Done ─────────────────────────────────────────────────────────────────────

func (m *Model) viewDoneLeft() string {
	url := m.urlInput
	if url == "" {
		url = "localhost"
	}
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}

	mode := detectMode(m.urlInput, m.ips, m.tsIP)

	var b strings.Builder
	b.WriteString(okSt.Bold(true).Render("✓  Qorven is ready!") + "\n")

	// Mode-specific subtitle
	switch mode {
	case "tailscale":
		b.WriteString(mutedSt.Render("Running on your Tailscale network.") + "\n")
	case "nat":
		b.WriteString(mutedSt.Render("Running on your local network (behind NAT).") + "\n")
	case "local":
		b.WriteString(mutedSt.Render("Running locally on this machine.") + "\n")
	default:
		b.WriteString(mutedSt.Render("Running and reachable from the internet.") + "\n")
	}

	b.WriteString("\n")

	// Health check status
	switch m.health {
	case healthChecking:
		b.WriteString(m.spinner.View() + " " + mutedSt.Render("Verifying service is up…") + "\n\n")
	case healthUp:
		b.WriteString(okSt.Bold(true).Render("✓  Service is responding") + "\n\n")
	case healthDown:
		b.WriteString(warnSt.Render("⚠  Service not yet responding") + "\n")
		b.WriteString(dimSt.Render("   Check: journalctl -u qorven -f") + "\n\n")
	}

	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")
	b.WriteString(boldSt.Render("Open Qorven in your browser:") + "\n\n")
	b.WriteString("  " + primSt.Render(url+"/") + "\n")

	// Mode-specific access notes
	switch mode {
	case "nat":
		if m.ips.wanIP != "" {
			b.WriteString("\n" + warnSt.Render("  Router WAN: "+m.ips.wanIP) + "\n")
			b.WriteString(mutedSt.Render("  Forward port 80 on your router to reach") + "\n")
			b.WriteString(mutedSt.Render("  this server from outside your network.") + "\n")
		}
		if len(m.ips.lanIPs) > 0 {
			b.WriteString("\n" + dimSt.Render("  Other LAN addresses:") + "\n")
			for _, ip := range m.ips.lanIPs {
				b.WriteString(dimSt.Render("    http://"+ip+"/") + "\n")
			}
		}
	case "tailscale":
		b.WriteString("\n" + dimSt.Render("  Only devices on your Tailscale network") + "\n")
		b.WriteString(dimSt.Render("  can reach this URL.") + "\n")
	}

	b.WriteString("\n" + dimSt.Render(strings.Repeat("─", 44)) + "\n\n")
	b.WriteString(fgSt.Render("Create your admin account and add an AI") + "\n")
	b.WriteString(fgSt.Render("provider from the web UI. No terminal config needed.") + "\n")
	return b.String()
}

func (m *Model) viewDoneRight() string {
	mode := detectMode(m.urlInput, m.ips, m.tsIP)

	steps := dimSt.Render("  1.  Open the URL on the left") + "\n" +
		dimSt.Render("  2.  Create your admin account") + "\n" +
		dimSt.Render("  3.  Add an AI provider API key") + "\n" +
		dimSt.Render("  4.  Create your first agent") + "\n" +
		dimSt.Render("  5.  Start chatting")

	service := mutedSt.Render("  systemctl status qorven") + "\n" +
		mutedSt.Render("  journalctl -u qorven -f") + "\n" +
		mutedSt.Render("  sudo qorven migrate up")

	var modeNote string
	switch mode {
	case "nat":
		modeNote = m.infoBox("Behind NAT",
			warnSt.Render("  Your WAN IP is "+m.ips.wanIP) + "\n\n" +
				dimSt.Render("  To reach Qorven from outside:") + "\n" +
				dimSt.Render("  • Forward port 80 on router") + "\n" +
				dimSt.Render("  • Or use Tailscale (tailscale.com)") + "\n" +
				dimSt.Render("  • Or use Cloudflare Tunnel"))
	case "tailscale":
		modeNote = m.infoBox("Tailscale mode",
			okSt.Render("  Secure private network access") + "\n\n" +
				dimSt.Render("  Add devices via tailscale.com/admin") + "\n" +
				dimSt.Render("  to let others join your Qorven."))
	case "local":
		modeNote = m.infoBox("Local access only",
			dimSt.Render("  Reachable on this machine only.") + "\n\n" +
				dimSt.Render("  To share with others:") + "\n" +
				dimSt.Render("  • Install Tailscale") + "\n" +
				dimSt.Render("  • Or use your LAN IP"))
	default:
		modeNote = m.infoBox("Publicly reachable",
			okSt.Render("  Your server has a public IP.") + "\n\n" +
				dimSt.Render("  Add a domain for HTTPS:") + "\n" +
				dimSt.Render("  Point DNS → "+m.ips.publicURL) + "\n" +
				dimSt.Render("  Then: sudo qorven tls enable"))
	}

	return m.infoBox("Next steps", steps) +
		"\n\n" +
		modeNote +
		"\n\n" +
		m.infoBox("Service commands", service)
}

// ── Error ─────────────────────────────────────────────────────────────────────

func (m *Model) viewErrorLeft() string {
	var b strings.Builder
	b.WriteString(failSt.Bold(true).Render("✗  Installation failed") + "\n\n")

	errBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cRed).
		Padding(1, 2).
		Width(m.leftW() - 14).
		Render(failSt.Render(m.errMsg))
	b.WriteString(errBox + "\n\n")

	b.WriteString(mutedSt.Render("Fix the issue above, then re-run:") + "\n")
	b.WriteString(primSt.Render("  sudo qorven install"))
	return b.String()
}

func (m *Model) viewErrorRight() string {
	common := dimSt.Render("  No internet — check curl / DNS") + "\n" +
		dimSt.Render("  Port 443 already in use") + "\n" +
		dimSt.Render("  postgresql service not starting") + "\n" +
		dimSt.Render("  Disk full — needs 10 GB free") + "\n" +
		dimSt.Render("  Not running as root (use sudo)")

	logs := mutedSt.Render("  journalctl -xe") + "\n" +
		mutedSt.Render("  apt-get install -f")

	return m.infoBox("Common causes", common) +
		"\n\n" +
		m.infoBox("Diagnose", logs)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) countDone() int {
	n := 0
	for _, s := range m.steps {
		if s.status == stepDone || s.status == stepWarn {
			n++
		}
	}
	return n
}

// ── Step execution ────────────────────────────────────────────────────────────

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	switch idx {
	case 0:
		out, _ := runSilent("bash", "-c", `source /etc/os-release 2>/dev/null && echo "$PRETTY_NAME"`)
		arch, _ := runSilent("uname", "-m")
		return strings.TrimSpace(out) + "  " + strings.TrimSpace(arch), false, nil

	case 1:
		if err = runQuiet("apt-get", "update", "-qq"); err != nil {
			return "skipped", true, nil
		}
		return "", false, nil

	case 2:
		if err = runQuiet("apt-get", "install", "-y", "-qq",
			"curl", "ca-certificates", "gnupg", "lsb-release", "openssl"); err != nil {
			return err.Error(), true, nil
		}
		return "", false, nil

	case 3: // PostgreSQL
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		if !commandExists("psql") {
			if err = runQuiet("apt-get", "install", "-y", "-qq", "postgresql", "postgresql-contrib"); err != nil {
				return "", false, fmt.Errorf("apt install postgresql: %w", err)
			}
			runQuiet("systemctl", "enable", "--now", "postgresql")
			time.Sleep(2 * time.Second)
		}
		// Install pgvector for the installed PostgreSQL major version.
		// The versioned package (postgresql-<maj>-pgvector) lives in the PGDG
		// APT repo.  Add it if missing — idempotent, safe to re-run.
		v, _ := runSilent("psql", "--version")
		pgMaj := ""
		if parts := strings.Fields(strings.TrimSpace(v)); len(parts) >= 3 {
			pgMaj = strings.Split(parts[2], ".")[0]
		}
		installPgvector(pgMaj)
		return "installed — " + strings.TrimSpace(v), false, nil

	case 4: // Docker
		if cfg.SkipDocker {
			return "skipped (--skip-docker)", true, nil
		}
		if commandExists("docker") {
			v, _ := runSilent("docker", "--version")
			return strings.TrimSpace(v), false, nil
		}
		scriptPath := "/tmp/get-docker.sh"
		if _, err = runSilent("curl", "-fsSL", "https://get.docker.com", "-o", scriptPath); err != nil {
			return "", false, fmt.Errorf("download docker script: %w", err)
		}
		os.Chmod(scriptPath, 0755)
		if err = runQuiet("sh", scriptPath); err != nil {
			return "", false, fmt.Errorf("docker install: %w", err)
		}
		runQuiet("systemctl", "enable", "--now", "docker")
		v, _ := runSilent("docker", "--version")
		return "installed — " + strings.TrimSpace(v), false, nil

	case 5: // OS user
		if _, err = runSilent("id", "qorven"); err != nil {
			if err = runQuiet("useradd", "--system", "--no-create-home",
				"--shell", "/usr/sbin/nologin", "qorven"); err != nil {
				return "", false, fmt.Errorf("useradd: %w", err)
			}
			if commandExists("docker") {
				runSilent("usermod", "-aG", "docker", "qorven")
			}
			return "created", false, nil
		}
		return "already exists", false, nil

	case 6: // Data dirs
		dirs := []string{cfg.DataDir, cfg.DataDir + "/logs", cfg.DataDir + "/workspaces", cfg.DataDir + "/tls", "/etc/qorven"}
		for _, d := range dirs {
			os.MkdirAll(d, 0755)
			runSilent("chown", "qorven:qorven", d)
		}
		return cfg.DataDir, false, nil

	case 7: // DB
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		out, _ := runSilent("sudo", "-u", "postgres", "psql", "-tAc",
			"SELECT 1 FROM pg_database WHERE datname='qorven'")
		if strings.TrimSpace(out) != "1" {
			if _, err = runSilent("sudo", "-u", "postgres", "createdb", "qorven"); err != nil {
				return "", false, fmt.Errorf("createdb: %w", err)
			}
		}
		out, _ = runSilent("sudo", "-u", "postgres", "psql", "-tAc",
			"SELECT 1 FROM pg_roles WHERE rolname='qorven'")
		if strings.TrimSpace(out) != "1" {
			if _, err = runSilent("sudo", "-u", "postgres", "createuser",
				"--no-superuser", "--no-createdb", "--no-createrole", "qorven"); err != nil {
				return "", false, fmt.Errorf("createuser: %w", err)
			}
		}
		runSilent("sudo", "-u", "postgres", "psql", "-c",
			"GRANT ALL PRIVILEGES ON DATABASE qorven TO qorven;")
		runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-c",
			"GRANT ALL ON SCHEMA public TO qorven;")
		// pgvector extension — must exist before migrations run
		runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-c",
			"CREATE EXTENSION IF NOT EXISTS vector;")
		return "ready", false, nil

	case 8: // Binary — stop service first, use atomic rename to avoid "text file busy"
		target := "/usr/local/bin/qorven"
		self, _ := os.Executable()
		selfReal, _ := filepath.EvalSymlinks(self)
		targetReal, evalErr := filepath.EvalSymlinks(target)
		if evalErr == nil && selfReal == targetReal {
			return "already in place", false, nil
		}
		// Stop the running service so the binary file is not in use
		runQuiet("systemctl", "stop", "qorven")
		data, readErr := os.ReadFile(self)
		if readErr != nil {
			return "", false, fmt.Errorf("read binary: %w", readErr)
		}
		// Write to temp path then rename — atomic, avoids ETXTBSY
		tmp := target + ".installing"
		if err = os.WriteFile(tmp, data, 0755); err != nil {
			return "", false, fmt.Errorf("write temp binary: %w", err)
		}
		if err = os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return "", false, fmt.Errorf("rename binary: %w", err)
		}
		return target, false, nil

	case 9: // systemd
		if !commandExists("systemctl") {
			return "not available — skipped", true, nil
		}
		unit := `[Unit]
Description=Qorven AI Gateway
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=qorven
Group=qorven
Environment=QORVEN_CONFIG=/etc/qorven/config.toml
EnvironmentFile=-/etc/qorven/.env
ExecStart=/usr/local/bin/qorven start
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
WorkingDirectory=/var/lib/qorven
NoNewPrivileges=yes
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=/var/lib/qorven /etc/qorven
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`
		if err = os.WriteFile("/etc/systemd/system/qorven.service", []byte(unit), 0644); err != nil {
			return "", false, fmt.Errorf("write unit: %w", err)
		}
		runQuiet("systemctl", "daemon-reload")
		runQuiet("systemctl", "enable", "qorven")
		runQuiet("systemctl", "start", "qorven")
		return "enabled", false, nil

	case 10: // Tailscale
		if cfg.SkipTailscale {
			return "skipped (--skip-tailscale)", true, nil
		}
		// Stop the service before doing anything with Tailscale — the running
		// service holds the binary open (ETXTBSY on re-run).
		// writeMinimalConfig() restarts it once config is written.
		runQuiet("systemctl", "stop", "qorven")

		// If already installed and connected, return the IP directly.
		if commandExists("tailscale") {
			if out, err := exec.Command("tailscale", "ip", "-4").Output(); err == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
		}

		// Install Tailscale if missing.
		if !commandExists("tailscale") {
			script := "/tmp/tailscale-install.sh"
			if _, err = runSilent("curl", "-fsSL", "https://tailscale.com/install.sh", "-o", script); err != nil {
				return "skipped (download failed)", true, nil
			}
			os.Chmod(script, 0755)
			if err = runQuiet("sh", script); err != nil {
				return "skipped (install failed)", true, nil
			}
		}

		// Headless pre-auth key path — no interactive screen needed.
		if cfg.TailscaleAuthKey != "" {
			if err = runQuiet("tailscale", "up",
				"--auth-key", cfg.TailscaleAuthKey,
				"--ssh", "--accept-routes", "--accept-dns"); err != nil {
				return "skipped (auth-key rejected)", true, nil
			}
			// Poll for IP up to 15s
			for i := 0; i < 15; i++ {
				time.Sleep(time.Second)
				if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
					ip := strings.TrimSpace(string(out))
					if strings.HasPrefix(ip, "100.") {
						return "connected:" + ip, false, nil
					}
				}
			}
			return "skipped (IP not assigned)", true, nil
		}

		// Interactive path — start tailscale up and read output line-by-line.
		// tailscale up blocks waiting for browser auth; we just need the URL it
		// prints, then we kill the process and let the polling loop handle the rest.
		tsCmd := exec.Command("tailscale", "up", "--ssh", "--accept-routes", "--accept-dns")
		pr, pw, pipeErr := os.Pipe()
		if pipeErr != nil {
			return "skipped (pipe error)", true, nil
		}
		tsCmd.Stdout = pw
		tsCmd.Stderr = pw
		if err = tsCmd.Start(); err != nil {
			pw.Close()
			pr.Close()
			return "skipped (start failed)", true, nil
		}
		pw.Close() // writer end closed in parent; reader will EOF when cmd exits

		authURL := ""
		buf := make([]byte, 4096)
		accumulated := ""
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) && authURL == "" {
			pr.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, _ := pr.Read(buf)
			if n > 0 {
				accumulated += string(buf[:n])
				for _, line := range strings.Split(accumulated, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "https://login.tailscale.com/") {
						authURL = line
						break
					}
				}
			}
			// Also check if already connected (tailscale up exited cleanly)
			if ip, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ipStr := strings.TrimSpace(string(ip))
				if strings.HasPrefix(ipStr, "100.") {
					tsCmd.Process.Kill()
					pr.Close()
					return "connected:" + ipStr, false, nil
				}
			}
		}
		pr.Close()
		// Don't kill the process — tailscale up needs to stay alive to complete
		// the auth handshake after the user clicks the URL.
		if authURL != "" {
			return "url:" + authURL, false, nil
		}
		// Timed out finding URL — kill and fall back
		tsCmd.Process.Kill()
		return "skipped (no auth URL found)", true, nil
	}
	return "", false, nil
}

// ── Health check ─────────────────────────────────────────────────────────────

// waitForHealth polls http://127.0.0.1:4200/health until it responds 200 or
// the deadline is reached.
func (m *Model) waitForHealth(timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 2 * time.Second}
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			resp, err := client.Get("http://127.0.0.1:4200/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return healthCheckMsg{up: true}
				}
			}
			time.Sleep(1 * time.Second)
		}
		return healthCheckMsg{up: false, err: "service did not respond within 12s"}
	}
}

// detectMode returns a label for the access mode based on URL, IP result and
// whether Tailscale was used (tsIP non-empty).
func detectMode(urlInput string, ips ipResult, tsIP string) string {
	if tsIP != "" {
		return "tailscale"
	}
	host := urlInput
	if strings.Contains(host, "://") {
		host = strings.SplitN(host, "://", 2)[1]
	}
	host = strings.SplitN(host, "/", 2)[0]
	if strings.HasPrefix(host, "100.") {
		return "tailscale"
	}
	if ips.behindNAT {
		return "nat"
	}
	if host == "localhost" || strings.HasPrefix(host, "127.") ||
		strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") ||
		strings.HasPrefix(host, "100.64.") || isPrivateHostIP(host) {
		return "local"
	}
	return "public"
}

// isPrivateHostIP checks whether the string host extracted from a URL is a
// private/CGNAT IP (covers 172.16-31.x which detectMode can't do with HasPrefix).
func isPrivateHostIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return isPrivateIP(ip)
}

// ── IP detection ──────────────────────────────────────────────────────────────

type ipResult struct {
	publicURL string
	wanIP     string
	lanIPs    []string
	behindNAT bool
}

func detectIPs() ipResult {
	var publicIface, privateIface []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			if isPrivateIP(ip) {
				privateIface = append(privateIface, ip.String())
			} else {
				publicIface = append(publicIface, ip.String())
			}
		}
	}
	if len(publicIface) > 0 {
		return ipResult{publicURL: publicIface[0], lanIPs: append(publicIface[1:], privateIface...)}
	}
	r := ipResult{behindNAT: true, lanIPs: privateIface}

	// Cloud metadata (AWS/GCP/Hetzner use 169.254.169.254).
	// If this succeeds the cloud provider routes external traffic to this server
	// via NAT/EIP — the metadata IP IS directly reachable, safe to pre-fill.
	if ip, err := runSilent("curl", "-sf", "--connect-timeout", "1",
		"http://169.254.169.254/latest/meta-data/public-ipv4"); err == nil && isValidIP(ip) {
		r.publicURL = strings.TrimSpace(ip)
		r.wanIP = r.publicURL
		r.behindNAT = false // cloud NAT is transparent; EIP works directly
		r.lanIPs = privateIface
		return r
	}

	// ipify gives the broadband router's WAN IP. That IP only reaches THIS server
	// if the user has port-forwarded 80/443 on their router — we cannot assume
	// that. Store it as wanIP (informational) but pre-fill with the LAN IP instead.
	if ip, err := runSilent("curl", "-sf", "--connect-timeout", "3",
		"https://api.ipify.org"); err == nil && isValidIP(ip) {
		r.wanIP = strings.TrimSpace(ip)
	}

	// Pre-fill with LAN IP — the only address guaranteed to reach this machine.
	if len(privateIface) > 0 {
		r.publicURL = privateIface[0]
		r.lanIPs = privateIface[1:]
	}
	return r
}

func isPrivateIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	// RFC-1918
	if ip[0] == 10 {
		return true
	}
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	// IANA Shared Address Space / CGNAT — used by Tailscale (100.64.0.0/10)
	if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
		return true
	}
	return false
}

func isValidIP(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && net.ParseIP(s) != nil
}

// ── Shell helpers ─────────────────────────────────────────────────────────────

func randHex(bytes int) string {
	b := make([]byte, bytes)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// installPgvector installs the pgvector shared library for the given PG major
// version. It tries the PGDG-style versioned package first
// (postgresql-<maj>-pgvector), adds the PGDG repo if that fails, and falls
// back to the distro-shipped "postgresql-pgvector" name used on Ubuntu 24.04.
func installPgvector(pgMaj string) {
	if pgMaj == "" {
		return
	}
	versioned := "postgresql-" + pgMaj + "-pgvector"
	if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
		return // already available (PGDG repo present or distro ships it)
	}
	// Add the PGDG APT repo so the versioned package becomes available.
	// Steps mirror https://www.postgresql.org/download/linux/debian/ and ubuntu/.
	distro, _ := runSilent("bash", "-c",
		`source /etc/os-release 2>/dev/null && echo "${ID}"`)
	distro = strings.TrimSpace(distro)
	codename, _ := runSilent("lsb_release", "-cs")
	codename = strings.TrimSpace(codename)
	if (distro == "ubuntu" || distro == "debian") && codename != "" {
		keyPath := "/usr/share/keyrings/postgresql.asc"
		runSilent("curl", "-fsSL",
			"https://www.postgresql.org/media/keys/accc4cf8.asc",
			"-o", keyPath)
		repo := fmt.Sprintf(
			"deb [signed-by=%s] https://apt.postgresql.org/pub/repos/apt %s-pgdg main",
			keyPath, codename)
		os.WriteFile("/etc/apt/sources.list.d/pgdg.list", []byte(repo+"\n"), 0644)
		runQuiet("apt-get", "update", "-qq")
		if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
			return
		}
	}
	// Last-resort: unversioned package name (Ubuntu universe / some distros)
	runQuiet("apt-get", "install", "-y", "-qq", "postgresql-pgvector")
}

// probeSocketDSN constructs a PostgreSQL socket DSN. It detects the actual
// socket port by scanning /var/run/postgresql for .s.PGSQL.<port> files so
// it works regardless of whether PG runs on the default 5432 or a custom port.
func probeSocketDSN() string {
	port := probePGPort()
	base := "postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable"
	if port != "" && port != "5432" {
		base += "&port=" + port
	}
	return base
}

func probePGPort() string {
	// Look for /var/run/postgresql/.s.PGSQL.<port>
	entries, err := os.ReadDir("/var/run/postgresql")
	if err != nil {
		return "5432"
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".s.PGSQL.") {
			p := strings.TrimPrefix(name, ".s.PGSQL.")
			if _, err := strconv.Atoi(p); err == nil {
				return p
			}
		}
	}
	return "5432"
}

func runQuiet(name string, args ...string) error {
	c := exec.Command(name, args...)
	return c.Run()
}

func runSilent(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// ── Entry point ───────────────────────────────────────────────────────────────

func Run(cfg Config) (bool, error) {
	m := New(cfg)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	fm, ok := final.(*Model)
	if !ok {
		return false, nil
	}
	if fm.quitting && fm.screen == screenDone {
		return true, nil
	}
	if fm.quitting {
		return false, fmt.Errorf("installation cancelled")
	}
	return fm.screen == screenDone, nil
}
