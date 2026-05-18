// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	qorvtls "github.com/qorvenai/qorven/internal/tls"
)

var tlsCmd = &cobra.Command{
	Use:   "tls",
	Short: "Manage TLS certificates",
}

var tlsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate CA + server certificates for HTTPS",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".qorven", "tls")
		cert, key, err := qorvtls.EnsureCert(dir)
		if err != nil {
			return fmt.Errorf("generate certs: %w", err)
		}
		fmt.Printf("✅ Certificates generated:\n  Cert: %s\n  Key:  %s\n  CA:   %s\n", cert, key, filepath.Join(dir, "ca.pem"))
		return nil
	},
}

var tlsInstallCACmd = &cobra.Command{
	Use:   "install-ca",
	Short: "Install the Qorven local CA into the OS trust store (needs sudo)",
	Long: `Install the Qorven local CA certificate into the operating
system's trust store so browsers trust https://localhost (and the
other SANs on the server cert — your hostname + every LAN IP).

Linux   — copies to /usr/local/share/ca-certificates/ (Debian family)
          or /etc/pki/ca-trust/source/anchors/ (RHEL family) and
          runs update-ca-certificates / update-ca-trust.
macOS   — calls security(8) to add the CA to the System keychain
          as a trustRoot. Prompts for the admin password.
Windows — not installed by this command. In an elevated PowerShell:
            certutil -addstore -f "ROOT" %USERPROFILE%\.qorven\tls\ca.pem`,
	RunE: func(cmd *cobra.Command, args []string) error {
		caFile, _ := cmd.Flags().GetString("ca-cert")
		if caFile == "" {
			home, _ := os.UserHomeDir()
			caFile = filepath.Join(home, ".qorven", "tls", "ca.pem")
		}
		dest, err := qorvtls.InstallCA(caFile)
		if err != nil {
			return fmt.Errorf("install CA: %w", err)
		}
		fp, _ := qorvtls.CAFingerprint(caFile)
		fmt.Printf("✅ CA installed at: %s\n", dest)
		if fp != "" {
			fmt.Printf("   Fingerprint (SHA-256): %s\n", fp)
		}
		fmt.Println("   Restart your browser to pick up the new trust anchor.")
		return nil
	},
}

var tlsFingerprintCmd = &cobra.Command{
	Use:   "show-fingerprint",
	Short: "Print the SHA-256 fingerprint of the local CA certificate",
	RunE: func(cmd *cobra.Command, args []string) error {
		caFile, _ := cmd.Flags().GetString("ca-cert")
		if caFile == "" {
			home, _ := os.UserHomeDir()
			caFile = filepath.Join(home, ".qorven", "tls", "ca.pem")
		}
		fp, err := qorvtls.CAFingerprint(caFile)
		if err != nil {
			return err
		}
		fmt.Println(fp)
		return nil
	},
}

var tlsRegenerateCmd = &cobra.Command{
	Use:   "regenerate",
	Short: "Wipe and regenerate the local CA + server cert (e.g. after the host IP changed)",
	Long: `Deletes ~/.qorven/tls/{cert,key,ca,ca-key}.pem and recreates
them. The CA fingerprint changes, so any browser that trusts the old
one will need to re-install via qorven tls install-ca.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".qorven", "tls")
		cert, key, err := qorvtls.Regenerate(dir)
		if err != nil {
			return fmt.Errorf("regenerate: %w", err)
		}
		fp, _ := qorvtls.CAFingerprint(filepath.Join(dir, "ca.pem"))
		fmt.Printf("✅ Regenerated:\n  Cert: %s\n  Key:  %s\n  CA:   %s\n",
			cert, key, filepath.Join(dir, "ca.pem"))
		if fp != "" {
			fmt.Printf("  CA fingerprint (SHA-256): %s\n", fp)
		}
		fmt.Println("  Re-run: sudo qorven tls install-ca")
		return nil
	},
}

// tlsCarootCmd prints the CAROOT directory so it can be passed to the client-side
// `mkcert -install` command: CAROOT=$(qorven tls caroot) mkcert -install
var tlsCarootCmd = &cobra.Command{
	Use:   "caroot",
	Short: "Print the CA root directory (for mkcert client-side trust setup)",
	Long: `Prints the path that contains rootCA.pem (when mkcert was used) or
ca.pem (self-signed fallback). Use this on the client to trust the cert:

  On your Mac/Linux laptop (after copying the CA cert from the server):
    brew install mkcert
    curl -sfk https://<server-ip>/ca.pem -o /tmp/qorven-ca.pem
    CAROOT=$(mkcert -CAROOT) cp /tmp/qorven-ca.pem $(mkcert -CAROOT)/rootCA.pem
    mkcert -install`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".qorven", "tls")
		fmt.Println(dir)
		return nil
	},
}

func init() {
	tlsInstallCACmd.Flags().String("ca-cert", "", "Path to CA certificate (default: ~/.qorven/tls/ca.pem)")
	tlsFingerprintCmd.Flags().String("ca-cert", "", "Path to CA certificate (default: ~/.qorven/tls/ca.pem)")
	tlsCmd.AddCommand(tlsGenerateCmd, tlsInstallCACmd, tlsFingerprintCmd, tlsRegenerateCmd, tlsCarootCmd)
	rootCmd.AddCommand(tlsCmd)
}
