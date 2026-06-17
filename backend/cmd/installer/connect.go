// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// connChoice is the result of asking the user how Qorven should be reached.
type connChoice struct {
	// useTailscale is true when the user chose to install + connect Tailscale.
	useTailscale bool
	// overrideURL, when non-empty, is the exact base address the user picked
	// (a detected IP or a custom URL). It takes precedence over auto-detection.
	overrideURL string
}

// chooseConnection asks the user how they want to reach this Qorven server,
// reading the answer from the controlling terminal (/dev/tty) so it works even
// under `curl | sudo bash`, where stdin is the piped script. It recommends
// Tailscale for private, encrypted access and otherwise offers this server's
// detected address or a custom URL.
//
// Flags win over the prompt: an explicit --tailscale-auth-key runs the headless
// Tailscale flow with no prompt, and --skip-tailscale skips it. When there is no
// terminal (cloud-init, CI, a bare pipe) the install stays fully unattended —
// it uses the auto-detected address and skips Tailscale.
func chooseConnection(cfg Config) connChoice {
	// A pre-supplied auth key means "use Tailscale, headless" — no prompt.
	if cfg.TailscaleAuthKey != "" {
		return connChoice{useTailscale: true}
	}
	// An explicit skip flag means "don't ask, don't use Tailscale".
	if cfg.SkipTailscale {
		return connChoice{useTailscale: false}
	}

	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		// No controlling terminal — stay unattended: auto-detect + skip Tailscale.
		fmt.Println("  No terminal detected — using this server's address and skipping Tailscale.")
		fmt.Println("  (pass --tailscale-auth-key=… to connect Tailscale unattended)")
		return connChoice{useTailscale: false}
	}
	defer tty.Close()

	ips := detectIPs()
	port := effectivePort(cfg)
	detected := ips.publicURL
	if detected == "" && len(ips.lanIPs) > 0 {
		detected = ips.lanIPs[0]
	}
	detectedURL := "http://localhost"
	if detected != "" {
		detectedURL = fmt.Sprintf("http://%s:%d", detected, port)
	}

	fmt.Fprintln(tty)
	fmt.Fprintln(tty, "  How will you reach this Qorven server?")
	fmt.Fprintln(tty)
	fmt.Fprintln(tty, "    1) Tailscale — private, encrypted access from your own devices  (recommended)")
	fmt.Fprintf(tty, "    2) This server's address — %s\n", detectedURL)
	fmt.Fprintln(tty, "    3) A custom domain or URL")
	fmt.Fprintln(tty)
	fmt.Fprint(tty, "  Choose [1]: ")

	reader := bufio.NewReader(tty)
	line, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(line)

	switch choice {
	case "", "1":
		return connChoice{useTailscale: true}
	case "2":
		return connChoice{useTailscale: false, overrideURL: detected}
	case "3":
		fmt.Fprint(tty, "  Enter the domain or URL (e.g. qorven.example.com): ")
		urlLine, _ := reader.ReadString('\n')
		custom := strings.TrimSpace(urlLine)
		if custom == "" {
			fmt.Fprintln(tty, "  Nothing entered — using this server's address.")
			return connChoice{useTailscale: false, overrideURL: detected}
		}
		return connChoice{useTailscale: false, overrideURL: custom}
	default:
		fmt.Fprintf(tty, "  '%s' is not an option — defaulting to Tailscale.\n", choice)
		return connChoice{useTailscale: true}
	}
}

// awaitTailscaleAuth prints the browser authorization URL to the terminal and
// waits, polling for a Tailscale IP, until the user authorizes the machine or
// the timeout elapses. It returns the assigned 100.x address, or "" if the user
// did not authorize in time (the install still completes; they can authorize
// later). It reads/writes the controlling terminal so the prompt is visible
// under `curl | sudo bash`.
func awaitTailscaleAuth(authURL string, timeout time.Duration) string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	out := os.Stdout
	if err == nil {
		out = tty
		defer tty.Close()
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Authorize this machine on your tailnet — open this URL in a browser:")
	fmt.Fprintf(out, "\n    %s\n\n", authURL)
	fmt.Fprint(out, "  Waiting for authorization")

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ip := tailscaleIP(); ip != "" {
			fmt.Fprintf(out, "\n  Connected — Tailscale address %s\n", ip)
			return ip
		}
		fmt.Fprint(out, ".")
		time.Sleep(2 * time.Second)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Not authorized yet — continuing. Once you authorize, reach Qorven at your")
	fmt.Fprintln(out, "  Tailscale address (run: tailscale ip -4).")
	return ""
}

// tailscaleIP returns the machine's current Tailscale IPv4 (100.x) or "".
func tailscaleIP() string {
	out, err := exec.Command("tailscale", "ip", "-4").Output()
	if err != nil {
		return ""
	}
	ip := strings.TrimSpace(string(out))
	if strings.HasPrefix(ip, "100.") {
		return ip
	}
	return ""
}
