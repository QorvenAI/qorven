# TLS — how the green lock works

Qorven ships its own local certificate authority so every self-hosted install gets HTTPS with a trusted cert, no domain required. On first run the binary generates:

- A CA cert (`~/.qorven/tls/ca.pem`, valid 10 years)
- A server cert signed by that CA (`~/.qorven/tls/cert.pem` + `key.pem`)
- The server cert includes SANs for `localhost`, the host's short name, and every LAN IP on the box

The CA is self-generated, not shared across installs — each Qorven machine has its own trust root. Installing that root into your OS is a one-time step per machine you visit the UI from.

## CLI

```bash
qorven tls generate            # regenerates only if files are missing
qorven tls install-ca          # add the local CA to the OS trust store (needs sudo)
qorven tls show-fingerprint    # SHA-256 of the CA (colon-separated hex)
qorven tls regenerate          # wipe + regenerate (use after IP / hostname change)
```

## Linux

`install-ca` writes to `/usr/local/share/ca-certificates/` (Debian family) or `/etc/pki/ca-trust/source/anchors/` (RHEL family) and runs `update-ca-certificates` / `update-ca-trust`. Firefox has its own trust store — add the CA manually once under Preferences → Certificates.

## macOS

`install-ca` calls `security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain` and prompts for the admin password. Safari and Chrome pick it up immediately; Firefox needs a manual add.

## Windows

There's no elevation path in the CLI. Copy `~/.qorven/tls/ca.pem` to the machine and run, in an **elevated PowerShell**:

```powershell
certutil -addstore -f "ROOT" C:\path\to\ca.pem
```

## Let's Encrypt / public domain

If the host has a real public domain pointed at it, set the TLS mode in `config.toml`:

```toml
[tls]
mode   = "acme"
domain = "qorven.example.com"
email  = "ops@example.com"
```

The gateway negotiates a Let's Encrypt cert via HTTP-01. The local CA stays generated (useful on the LAN side); externally the browser sees the Let's Encrypt chain.

## Bring-your-own cert

```toml
[tls]
mode    = "custom"
tls_cert = "/path/to/fullchain.pem"
tls_key  = "/path/to/key.pem"
```

`install-ca` is a no-op in this mode — the provided chain is whatever the host's OS already trusts.

## Rotating after IP change

The server cert's SANs are pinned to the IPs present at generation time. If the host moves, re-run:

```bash
sudo qorven tls regenerate
sudo qorven tls install-ca          # on this machine
sudo systemctl restart qorven
```

The CA fingerprint **changes** after `regenerate`; every machine that trusted the old CA needs to re-run `install-ca`.
