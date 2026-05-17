// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnsureCert generates CA + server cert if they don't exist. Returns cert/key paths.
// When mkcert is available on PATH it is used; this allows clients to gain browser
// trust by running a single `mkcert -install` on their device with the same CAROOT.
// Falls back to a pure-Go self-signed CA when mkcert is absent.
func EnsureCert(dir string) (certFile, keyFile string, err error) {
	os.MkdirAll(dir, 0700)
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	if fileExists(certFile) && fileExists(keyFile) {
		return certFile, keyFile, nil
	}

	slog.Info("tls: generating certificates", "dir", dir)

	if mkcertPath, e := osexec.LookPath("mkcert"); e == nil {
		if cf, kf, e2 := ensureCertMkcert(mkcertPath, dir, certFile, keyFile); e2 == nil {
			return cf, kf, nil
		} else {
			slog.Warn("tls: mkcert failed, falling back to self-signed", "err", e2)
		}
	}

	return ensureCertSelfSigned(dir, certFile, keyFile)
}

// ensureCertMkcert uses the mkcert binary (with CAROOT=dir) to generate a
// locally-trusted cert. The rootCA.pem written by mkcert is aliased to ca.pem
// so the rest of the codebase can find it at the canonical path.
func ensureCertMkcert(mkcertPath, dir, certFile, keyFile string) (string, string, error) {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if h, _ := os.Hostname(); h != "" && h != "localhost" {
		hosts = append(hosts, h)
	}
	for _, ip := range lanIPs() {
		hosts = append(hosts, ip.String())
	}

	args := append([]string{"-cert-file", certFile, "-key-file", keyFile}, hosts...)
	cmd := osexec.Command(mkcertPath, args...)
	cmd.Env = append(os.Environ(), "CAROOT="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("mkcert: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Alias rootCA.pem → ca.pem for compatibility with the rest of the codebase.
	caFile := filepath.Join(dir, "ca.pem")
	if !fileExists(caFile) {
		if data, err := os.ReadFile(filepath.Join(dir, "rootCA.pem")); err == nil {
			os.WriteFile(caFile, data, 0600) //nolint:errcheck
		}
	}

	slog.Info("tls: mkcert certificates generated", "cert", certFile, "caroot", dir)
	return certFile, keyFile, nil
}

// ensureCertSelfSigned generates a pure-Go CA + server cert when mkcert is absent.
func ensureCertSelfSigned(dir, certFile, keyFile string) (string, string, error) {
	caFile := filepath.Join(dir, "ca.pem")
	caKeyFile := filepath.Join(dir, "ca-key.pem")

	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Qorven Local CA"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create CA: %w", err)
	}
	caCert, _ := x509.ParseCertificate(caCertDER)

	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sans := []string{"localhost"}
	if h, _ := os.Hostname(); h != "" {
		sans = append(sans, h)
	}
	ips := []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	ips = append(ips, lanIPs()...)

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{Organization: []string{"Qorven"}},
		DNSNames:     sans,
		IPAddresses:  ips,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("create server cert: %w", err)
	}

	writePEM(caFile, "CERTIFICATE", caCertDER)
	writeKeyPEM(caKeyFile, caKey)
	writePEM(certFile, "CERTIFICATE", serverCertDER)
	writeKeyPEM(keyFile, serverKey)

	slog.Info("tls: self-signed certificates generated", "cert", certFile, "ca", caFile, "sans", sans, "ips", ips)
	return certFile, keyFile, nil
}

// InstallCA copies the Qorven local CA into every trust store that
// will suppress browser warnings without any manual user action.
//
// When mkcert generated the certs (rootCA.pem is present in the same
// directory as caFile), this calls `mkcert -install` which handles
// system store + NSS (Chrome/Chromium) + Firefox in one shot.
//
// Without mkcert it falls back to the manual approach:
//
// Linux — system store (/usr/local/share/ca-certificates or /etc/pki) +
// NSS database (~/.pki/nssdb) + Firefox profiles.
// macOS — System Keychain covers Safari and Chrome; Firefox NSS too.
func InstallCA(caFile string) (string, error) {
	dir := filepath.Dir(caFile)

	// Prefer mkcert -install: handles all stores (system + NSS + Firefox) atomically.
	if mkcertPath, err := osexec.LookPath("mkcert"); err == nil {
		if fileExists(filepath.Join(dir, "rootCA.pem")) {
			cmd := osexec.Command(mkcertPath, "-install")
			cmd.Env = append(os.Environ(), "CAROOT="+dir)
			out, err := cmd.CombinedOutput()
			if err == nil {
				slog.Info("tls: mkcert CA installed", "caroot", dir)
				return "mkcert: system CA store + NSS (Chrome/Firefox)", nil
			}
			slog.Warn("tls: mkcert -install failed, trying manual install",
				"error", strings.TrimSpace(string(out)))
		}
	}

	// Manual fallback.
	data, err := os.ReadFile(caFile)
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "linux":
		results := []string{}
		errs := []string{}

		// ── 1. System CA store (curl, wget, Go TLS) ──────────────────
		if _, err := os.Stat("/usr/local/share/ca-certificates"); err == nil {
			dest := "/usr/local/share/ca-certificates/qorven-local-ca.crt"
			if err := os.WriteFile(dest, data, 0644); err != nil {
				errs = append(errs, "system-store: "+err.Error())
			} else if _, err := runTool("update-ca-certificates").Output(); err != nil {
				errs = append(errs, "update-ca-certificates: "+err.Error())
			} else {
				results = append(results, dest)
			}
		} else if _, err := os.Stat("/etc/pki/ca-trust/source/anchors"); err == nil {
			dest := "/etc/pki/ca-trust/source/anchors/qorven-local-ca.crt"
			if err := os.WriteFile(dest, data, 0644); err != nil {
				errs = append(errs, "system-store: "+err.Error())
			} else if _, err := runTool("update-ca-trust").Output(); err != nil {
				errs = append(errs, "update-ca-trust: "+err.Error())
			} else {
				results = append(results, dest)
			}
		}

		// ── 2. NSS databases — Chrome/Chromium + Firefox ─────────────
		if _, err := osexec.LookPath("certutil"); err == nil {
			nssDirs := findNSSDirs()
			for _, nssDir := range nssDirs {
				out, err := runTool("certutil", "-d", "sql:"+nssDir,
					"-A", "-t", "C,,", "-n", "Qorven Local CA", "-i", caFile).CombinedOutput()
				if err != nil {
					runTool("mkdir", "-p", nssDir).Run()                                  //nolint:errcheck
					runTool("certutil", "-d", "sql:"+nssDir, "-N", "--empty-password").Run() //nolint:errcheck
					out2, err2 := runTool("certutil", "-d", "sql:"+nssDir,
						"-A", "-t", "C,,", "-n", "Qorven Local CA", "-i", caFile).CombinedOutput()
					if err2 != nil {
						errs = append(errs, fmt.Sprintf("nss(%s): %s", nssDir, strings.TrimSpace(string(out2))))
					} else {
						results = append(results, "nss:"+nssDir)
					}
				} else {
					_ = out
					results = append(results, "nss:"+nssDir)
				}
			}
		} else {
			installCertutil()
			if _, err := osexec.LookPath("certutil"); err == nil {
				for _, nssDir := range findNSSDirs() {
					runTool("certutil", "-d", "sql:"+nssDir,
						"-A", "-t", "C,,", "-n", "Qorven Local CA", "-i", caFile).Run() //nolint:errcheck
					results = append(results, "nss:"+nssDir)
				}
			}
		}

		if len(results) == 0 && len(errs) > 0 {
			return "", fmt.Errorf("CA install failed: %s", strings.Join(errs, "; "))
		}
		return strings.Join(results, ", "), nil

	case "darwin":
		keychain := "/Library/Keychains/System.keychain"
		cmd := runTool("security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", keychain, caFile)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("security add-trusted-cert failed (need sudo?): %s: %w",
				strings.TrimSpace(string(out)), err)
		}
		results := []string{keychain}
		if _, err := osexec.LookPath("certutil"); err == nil {
			for _, nssDir := range findNSSDirs() {
				runTool("certutil", "-d", "sql:"+nssDir,
					"-A", "-t", "C,,", "-n", "Qorven Local CA", "-i", caFile).Run() //nolint:errcheck
				results = append(results, "firefox:"+nssDir)
			}
		}
		return strings.Join(results, ", "), nil

	case "windows":
		return "", fmt.Errorf("on Windows run in an elevated PowerShell:\n  certutil -addstore -f \"ROOT\" %s", caFile)
	default:
		return "", fmt.Errorf("unsupported OS: %s — install %s into your browser/OS trust store manually",
			runtime.GOOS, caFile)
	}
}

// findNSSDirs returns all NSS database directories for the current user.
// Covers Chrome (%HOME/.pki/nssdb), Chromium, and Firefox profiles.
func findNSSDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string

	dirs = append(dirs, filepath.Join(home, ".pki", "nssdb"))

	firefoxBase := filepath.Join(home, ".mozilla", "firefox")
	if entries, err := os.ReadDir(firefoxBase); err == nil {
		for _, e := range entries {
			if e.IsDir() && strings.Contains(e.Name(), ".default") {
				dirs = append(dirs, filepath.Join(firefoxBase, e.Name()))
			}
		}
	}
	macFFBase := filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")
	if entries, err := os.ReadDir(macFFBase); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(macFFBase, e.Name()))
			}
		}
	}
	return dirs
}

// installCertutil installs the libnss3-tools package which provides the
// certutil binary needed to add CAs to Chrome's NSS database.
func installCertutil() {
	for _, args := range [][]string{
		{"apt-get", "install", "-y", "-qq", "libnss3-tools"},
		{"dnf", "install", "-y", "-q", "nss-tools"},
		{"yum", "install", "-y", "-q", "nss-tools"},
	} {
		if path, err := osexec.LookPath(args[0]); err == nil {
			osexec.Command(path, args[1:]...).Run() //nolint:errcheck
			return
		}
	}
}

// runTool is a thin wrapper over os/exec Command so we can stub in tests and so
// the hard-coded argument lists are easy to audit. Every arg comes from
// compile-time constants or file paths we own — no user input reaches the shell.
func runTool(name string, args ...string) *osexec.Cmd { return osexec.Command(name, args...) }

// CAFingerprint returns the SHA-256 fingerprint of the CA cert.
func CAFingerprint(caFile string) (string, error) {
	data, err := os.ReadFile(caFile)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("not a PEM file: %s", caFile)
	}
	sum := sha256.Sum256(block.Bytes)
	hexStr := hex.EncodeToString(sum[:])
	var b strings.Builder
	for i := 0; i < len(hexStr); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(strings.ToUpper(hexStr[i : i+2]))
	}
	return b.String(), nil
}

// Regenerate wipes existing certs (including mkcert rootCA files) and re-runs EnsureCert.
func Regenerate(dir string) (certFile, keyFile string, err error) {
	for _, name := range []string{
		"cert.pem", "key.pem", "ca.pem", "ca-key.pem",
		"rootCA.pem", "rootCA-key.pem",
	} {
		os.Remove(filepath.Join(dir, name))
	}
	return EnsureCert(dir)
}

func lanIPs() []net.IP {
	var ips []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP)
		}
	}
	return ips
}

func writePEM(path, typ string, der []byte) {
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: typ, Bytes: der}) //nolint:errcheck
}

func writeKeyPEM(path string, key *ecdsa.PrivateKey) {
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	defer f.Close()
	der, _ := x509.MarshalECPrivateKey(key)
	pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}) //nolint:errcheck
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }
