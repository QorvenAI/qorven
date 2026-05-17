# Security Policy

## Supported versions

| Version | Security updates |
|---|---|
| `0.1.x` (current) | ✅ Yes |
| Earlier | ❌ No |

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.** Public disclosure before a fix is ready puts every Qorven installation at risk.

Instead, report via one of these channels:

- **GitHub private advisory:** [Report a vulnerability](https://github.com/QorvenAI/qorven/security/advisories/new) ← preferred
- **Email:** security@qorven.ai

Please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce (proof-of-concept is welcome, exploitation not required)
- Affected versions, if known
- Any suggested mitigations you've identified

## What to expect

| Step | Timeline |
|---|---|
| Acknowledgement | Within 48 hours |
| Initial assessment | Within 5 business days |
| Fix or mitigation | Target: 30 days for critical, 90 days for others |
| Public disclosure | Coordinated with reporter after fix is released |

We follow responsible disclosure: we will notify you before public release, credit you in the advisory (unless you prefer to remain anonymous), and work with you on the disclosure timeline.

## Scope

In scope:
- Authentication bypass or privilege escalation
- Remote code execution
- Data exfiltration across tenant boundaries
- Prompt injection that bypasses safety controls
- Cryptographic weaknesses in key storage or transport

Out of scope:
- Issues in third-party dependencies (report upstream; notify us separately if critical)
- Self-inflicted issues from misconfiguration (e.g. exposing Qorven without authentication on a public IP)
- Social engineering

## Bug bounty

There is no paid bug bounty programme at this time. We will acknowledge your contribution publicly and can provide Qorven swag or a sponsor credit on request.
