# Qorven Roadmap

Path to v1.0 stable. Priorities are adjusted based on community feedback — [open an issue](https://github.com/QorvenAI/qorven/issues/new) or [start a Discussion](https://github.com/QorvenAI/qorven/discussions) to weigh in.

---

## Now — v0.1.x

Ongoing fixes and improvements:

- [ ] Channel setup in-app guides (per-channel credential walkthroughs)
- [ ] Self-update polish — rollback on failure, progress feedback
- [ ] Memory tab: tagging and manual editing of stored memories
- [ ] Docker Compose one-liner for non-root / non-systemd installs
- [ ] Complete Matrix, Signal, and Mattermost channel wiring

---

## Next — v0.2

*Target: ~30 days*

- [ ] **Agent-generated connectors** — natural language → working Go connector scaffolded, built, and installed at runtime; no developer required
- [ ] **Dashboard live tiles** — pin any connector output to your home screen (stat cards, tables, feeds)
- [ ] **Connector scope model** — workspace / agent / team visibility for installed connectors and credentials
- [ ] **Data source scheduling** — connectors can declare a cron schedule; results injected into daily briefings
- [ ] Guided first-run: channel setup wizard, model selection helper
- [ ] Connector credential vault — per-connector encrypted key storage

---

## v0.3 — Integrations

*Target: 30–60 days*

- [ ] Connector marketplace — share and install community connectors
- [ ] Deeper GitHub integration (PR review, issue triage, CI summaries)
- [ ] Google Calendar and Outlook scheduling agents
- [ ] HubSpot / Pipedrive CRM connector
- [ ] Notion and Linear workspace integration

---

## v0.4 — Teams & Access Control

*Target: 60–90 days*

- [ ] Multi-user teams with role-based access control
- [ ] Per-agent and per-team scoped connectors and credentials
- [ ] Audit log UI improvements (filtering, CSV export, webhook push)
- [ ] LDAP / SSO authentication option
- [ ] Agent performance analytics dashboard

---

## v1.0 — Production Stable

*Target: 90+ days*

- [ ] High-availability multi-node deployment
- [ ] Backup and restore with point-in-time recovery
- [ ] Full test coverage on all critical paths
- [ ] Comprehensive operations and upgrade documentation
- [ ] Third-party security audit
- [ ] Stable plugin/extension API for community connectors

---

## Continuous

- Binary size and cold-start optimisation
- Agent reasoning reliability and transparency
- Model provider coverage
- Documentation depth and contributor onboarding
- Web UI accessibility

---

> To sponsor a specific roadmap item, [contact us](mailto:hello@qorven.ai).
