// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package installer provides a full-screen Bubbletea TUI for `qorven install`.
// Platform-specific install steps live in installer_linux.go and installer_darwin.go.
package installer

import (
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
	screenPortPicker      // ask user which port to bind (default 8486)
	screenNginxChoice     // ask user whether to install nginx (default: no)
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
	Port             int  // chosen port; 0 means use DefaultPort (8486)
	SkipNginx        bool // true = do not install/configure nginx
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

	// port picker screen
	portInput       string // raw text the user is typing
	portErr         string // validation / availability message
	portBusyWarned  bool   // true after first "port busy" warning; next Enter overrides

	// nginx choice screen
	nginxChoice int // 0=no (default), 1=yes

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
		steps:   platformSteps(),
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
					// Flag already decided — skip Tailscale choice, go straight to install
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
				// Go straight to install — no port/nginx questions
				m.screen = screenInstall
				m.stepStarted = time.Now()
				return m, tea.Batch(m.spinner.Tick, tickCmd(), m.kickStep(0))
			}

		case screenPortPicker:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "enter":
				port := 8486
				if m.portInput != "" {
					n, convErr := strconv.Atoi(strings.TrimSpace(m.portInput))
					if convErr != nil || n < 1 || n > 65535 {
						m.portErr = fmt.Sprintf("'%s' is not a valid port number (1–65535). Try again: ", m.portInput)
						m.portBusyWarned = false
						return m, nil
					}
					port = n
				}
				// If user already saw "port busy" warning and hits Enter again → override
				if m.portBusyWarned {
					m.cfg.Port = port
					m.nginxChoice = 0
					m.screen = screenNginxChoice
					return m, nil
				}
				// Probe availability
				ln, listenErr := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
				if listenErr != nil {
					// Port busy — warn; next Enter will override
					m.portErr = fmt.Sprintf("Port %d is already in use. Press Enter to use it anyway, or type a different port: ", port)
					m.portBusyWarned = true
					return m, nil
				}
				ln.Close()
				m.cfg.Port = port
				// Move to nginx choice screen
				m.nginxChoice = 0 // default: no
				m.screen = screenNginxChoice
				return m, nil
			case "backspace":
				if len(m.portInput) > 0 {
					m.portInput = m.portInput[:len(m.portInput)-1]
					m.portErr = ""
					m.portBusyWarned = false
				}
			default:
				ch := msg.String()
				if len(ch) == 1 && ch[0] >= '0' && ch[0] <= '9' {
					m.portInput += ch
					m.portErr = ""
					m.portBusyWarned = false
				}
			}

		case screenNginxChoice:
			switch msg.String() {
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "up", "k":
				if m.nginxChoice > 0 {
					m.nginxChoice--
				}
			case "down", "j":
				if m.nginxChoice < 1 {
					m.nginxChoice++
				}
			case "enter", " ":
				m.cfg.SkipNginx = (m.nginxChoice == 0) // 0=No, 1=Yes
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
		// steps[11].detail is either:
		//   "connected:<100.x.x.x>"  → already connected, skip auth screen
		//   "url:<https://...>"       → need browser auth
		//   "skipped"                 → --skip-tailscale or error, go to config
		m.ips = detectIPs()
		detail := m.steps[11].detail
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
	etcDir := platformConfigDir()
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

	port := m.effectivePort()
	configPath := filepath.Join(etcDir, "config.toml")
	cfgContent := fmt.Sprintf(`# Qorven Configuration — generated by qorven install
[server]
listen = "0.0.0.0:%d"
base_url = "%s"

[server.tls]
mode = "disabled"

[database]
# DSN is in .env
`, port, baseURL)
	if err := os.WriteFile(configPath, []byte(cfgContent), 0644); err != nil {
		return fmt.Errorf("write config.toml: %w", err)
	}

	dsn := probeSocketDSN()
	envPath := filepath.Join(etcDir, ".env")

	// Preserve the existing encryption_key and gateway_token on re-run.
	// Regenerating these would render all stored API keys and secrets unreadable.
	existingKey := ""
	existingToken := ""
	if existing, readErr := os.ReadFile(envPath); readErr == nil {
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.HasPrefix(line, "QORVEN_ENCRYPTION_KEY=") {
				existingKey = strings.TrimPrefix(line, "QORVEN_ENCRYPTION_KEY=")
			}
			if strings.HasPrefix(line, "QORVEN_GATEWAY_TOKEN=") {
				existingToken = strings.TrimPrefix(line, "QORVEN_GATEWAY_TOKEN=")
			}
		}
	}
	if existingKey == "" {
		existingKey = randHex(32)
	}
	if existingToken == "" {
		existingToken = randHex(16)
	}

	env := strings.Join([]string{
		"# Qorven secrets — keep private",
		"QORVEN_POSTGRES_DSN=" + dsn,
		"QORVEN_GATEWAY_TOKEN=" + existingToken,
		"QORVEN_ENCRYPTION_KEY=" + existingKey,
		"",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(env), 0600); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}

	// Run migrations before starting the service (retry 3x — socket needs a moment)
	for i := 0; i < 3; i++ {
		if err := platformMigrate(configPath, dsn); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	platformRestartService(configPath)
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
	case screenPortPicker:
		left, right = m.viewPortPickerLeft(), m.viewPortPickerRight()
	case screenNginxChoice:
		left, right = m.viewNginxChoiceLeft(), m.viewNginxChoiceRight()
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
	reqs := platformRequirementsText()

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

// ── Port picker ───────────────────────────────────────────────────────────────

func (m *Model) viewPortPickerLeft() string {
	var b strings.Builder
	b.WriteString(boldSt.Render("Choose a port for Qorven") + "\n")
	b.WriteString(mutedSt.Render("This is the port Qorven will listen on for all traffic.") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", 44)) + "\n\n")

	b.WriteString(fgSt.Render("Default: ") + primSt.Render("8486") + "\n")
	b.WriteString(mutedSt.Render("Press Enter to accept the default, or type a port number.") + "\n\n")

	prompt := "Port: "
	input := m.portInput
	if input == "" {
		input = dimSt.Render("8486  (default)")
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(cPrimary).
		Padding(0, 1).
		Width(m.leftW() - 12).
		Render(boldSt.Render(prompt) + fgSt.Render(input))
	b.WriteString(inputBox + "\n\n")

	if m.portErr != "" {
		errBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cAmber).
			Padding(0, 1).
			Width(m.leftW() - 12).
			Render(warnSt.Render("⚠  " + m.portErr))
		b.WriteString(errBox + "\n")
	}
	return b.String()
}

func (m *Model) viewPortPickerRight() string {
	info := fgSt.Render("  Qorven serves its web UI, API, and") + "\n" +
		fgSt.Render("  WebSocket connections on a single port.") + "\n\n" +
		dimSt.Render("  Port 8486 is Qorven's default.") + "\n" +
		dimSt.Render("  It is not registered with IANA and is") + "\n" +
		dimSt.Render("  unlikely to conflict with other services.") + "\n\n" +
		okSt.Render("  ✓  1–65535 are valid port numbers") + "\n" +
		okSt.Render("  ✓  Ports < 1024 require root access") + "\n" +
		mutedSt.Render("  ✓  You can change this later in config.toml")

	nginx := dimSt.Render("  If you install nginx (next step),") + "\n" +
		dimSt.Render("  nginx listens on port 80 and forwards") + "\n" +
		dimSt.Render("  all traffic to the port you choose here.") + "\n\n" +
		mutedSt.Render("  Users always reach Qorven on port 80;") + "\n" +
		mutedSt.Render("  this port stays internal.")

	return m.infoBox("About the port", info) +
		"\n\n" +
		m.infoBox("nginx & port 80", nginx)
}

// ── Nginx choice ──────────────────────────────────────────────────────────────

func (m *Model) viewNginxChoiceLeft() string {
	var b strings.Builder
	b.WriteString(boldSt.Render("Set up nginx as a reverse proxy?") + "\n")
	b.WriteString(mutedSt.Render("nginx listens on port 80 and forwards traffic to Qorven.") + "\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(cBorder).Render(strings.Repeat("─", 44)) + "\n\n")

	options := []struct {
		label    string
		sublabel string
		desc     string
	}{
		{
			label:    "No — skip nginx",
			sublabel: "Default",
			desc:     fmt.Sprintf("Qorven is reachable directly on port %d. No nginx needed.", m.effectivePort()),
		},
		{
			label:    "Yes — install nginx",
			sublabel: "Recommended for port 80",
			desc:     "nginx proxies port 80 → Qorven, adds WebSocket support, and makes HTTPS easy to add later.",
		},
	}

	for i, opt := range options {
		selected := m.nginxChoice == i
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

func (m *Model) viewNginxChoiceRight() string {
	whenSkip := dimSt.Render("  Access Qorven at:") + "\n" +
		primSt.Render(fmt.Sprintf("  http://<your-ip>:%d/", m.effectivePort())) + "\n\n" +
		mutedSt.Render("  Good for Tailscale, LAN, or when you") + "\n" +
		mutedSt.Render("  already have a reverse proxy set up.")

	whenNginx := dimSt.Render("  Access Qorven at:") + "\n" +
		primSt.Render("  http://<your-ip>/") + "\n\n" +
		mutedSt.Render("  nginx handles port 80 and later HTTPS.") + "\n" +
		mutedSt.Render("  Installs nginx via apt if not present.")

	return m.infoBox("Without nginx", whenSkip) +
		"\n\n" +
		m.infoBox("With nginx", whenNginx)
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
	var b strings.Builder
	b.WriteString(okSt.Bold(true).Render("✓  Qorven is installed!") + "\n\n")

	// Health check status
	switch m.health {
	case healthChecking:
		b.WriteString(m.spinner.View() + " " + mutedSt.Render("Verifying service is up…") + "\n\n")
	case healthUp:
		b.WriteString(okSt.Bold(true).Render("✓  Service is running") + "\n\n")
	case healthDown:
		b.WriteString(warnSt.Render("⚠  Service not yet responding") + "\n")
		b.WriteString(dimSt.Render("   Check: journalctl -u qorven -f") + "\n\n")
	}

	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")
	b.WriteString(boldSt.Render("Access Qorven at:") + "\n\n")

	port := m.effectivePort()

	// Always show local/public IP
	localURL := ""
	if m.ips.publicURL != "" {
		localURL = fmt.Sprintf("http://%s:%d/", m.ips.publicURL, port)
	} else if len(m.ips.lanIPs) > 0 {
		localURL = fmt.Sprintf("http://%s:%d/", m.ips.lanIPs[0], port)
	} else {
		localURL = fmt.Sprintf("http://localhost:%d/", port)
	}
	b.WriteString("  " + fgSt.Render("Local / Public IP") + "\n")
	b.WriteString("  " + primSt.Render(localURL) + "\n\n")

	// Tailscale IP (if connected)
	if m.tsIP != "" {
		tsURL := fmt.Sprintf("http://%s:%d/", m.tsIP, port)
		b.WriteString("  " + fgSt.Render("Tailscale (private network)") + "\n")
		b.WriteString("  " + primSt.Render(tsURL) + "\n")
		b.WriteString("  " + mutedSt.Render("Reachable from any device on your Tailscale network.") + "\n\n")
	}

	b.WriteString(dimSt.Render(strings.Repeat("─", 44)) + "\n\n")
	b.WriteString(fgSt.Render("Open either URL to complete setup in your browser.") + "\n")
	return b.String()
}

func (m *Model) viewDoneRight() string {
	steps := dimSt.Render("  1.  Open the URL on the left") + "\n" +
		dimSt.Render("  2.  Create your admin account") + "\n" +
		dimSt.Render("  3.  Add an AI provider API key") + "\n" +
		dimSt.Render("  4.  Start chatting")

	service := platformServiceCommands()

	var accessNote string
	if m.tsIP != "" {
		accessNote = m.infoBox("Tailscale connected",
			okSt.Render("  ✓  Private network access active") + "\n\n" +
				dimSt.Render("  Add devices: tailscale.com/admin") + "\n" +
				dimSt.Render("  Port & TLS: Settings → Access in the UI"))
	} else if m.ips.publicURL != "" {
		accessNote = m.infoBox("Public IP",
			okSt.Render("  ✓  Publicly reachable") + "\n\n" +
				dimSt.Render("  Add a domain and HTTPS in") + "\n" +
				dimSt.Render("  Settings → Access after install."))
	} else {
		accessNote = m.infoBox("Private network",
			dimSt.Render("  Reachable on your LAN.") + "\n\n" +
				dimSt.Render("  For remote access install Tailscale:") + "\n" +
				dimSt.Render("  tailscale.com/download"))
	}

	return m.infoBox("Next steps", steps) +
		"\n\n" +
		accessNote +
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
	common, logs := platformErrorHints()
	return m.infoBox("Common causes", common) +
		"\n\n" +
		m.infoBox("Diagnose", logs)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// effectivePort returns the port chosen by the user, falling back to 8486.
func (m *Model) effectivePort() int {
	if m.cfg.Port > 0 {
		return m.cfg.Port
	}
	return 8486
}

func (m *Model) countDone() int {
	n := 0
	for _, s := range m.steps {
		if s.status == stepDone || s.status == stepWarn {
			n++
		}
	}
	return n
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

// ── Health check ──────────────────────────────────────────────────────────────

func (m *Model) waitForHealth(timeout time.Duration) tea.Cmd {
	port := m.effectivePort()
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	return func() tea.Msg {
		client := &http.Client{Timeout: 2 * time.Second}
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			resp, err := client.Get(url)
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
