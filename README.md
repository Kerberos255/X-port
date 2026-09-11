# X-port

A private, Linux-first Xray control panel built around one simple rule:

> **One account = one inbound = one dedicated port.**

X-port deliberately removes the reseller/membership/points model found in many x-ui derivatives. It is designed for a small private server with a handful of independently managed accounts.

## Current features

- Server dashboard: CPU, memory, disk, uptime, network rate, Xray/X-port state
- Account create / edit / delete / clone
- One account per inbound and port
- VLESS + REALITY + Vision account creation
- Clone regenerates UUID and clears traffic counters
- VLESS share link and local QR generation
- Xray update check and one-click update
  - reads official `XTLS/Xray-core` GitHub Releases
  - verifies GitHub's SHA-256 asset digest
  - tests the new binary against the current config
  - automatically restores the previous binary if restart fails
- SQLite storage
- X-Panel / 3x-ui read-only migration with dry-run, watchdog suppression and transactional rollback
- Linux systemd deployment
- Single Go binary with embedded Web UI; no Node/npm runtime on the server

## Layout

- `cmd/xport` — CLI / HTTP server
- `internal/accountcfg` — account model ↔ Xray VLESS/REALITY config
- `internal/service` — safe account mutations and rollback
- `internal/xray` — config rendering, runtime apply, core updater
- `internal/migrate` — X-Panel/3x-ui SQLite migration
- `web/dist` — dependency-free embedded Web UI
- `scripts` — build, install and migration helpers

## Build

Go 1.27+:

```bash
./scripts/build-linux.sh        # current architecture
./scripts/build-linux.sh amd64
./scripts/build-linux.sh arm64
```

The binary is written to `dist/`.

## Fresh install

```bash
sudo ./scripts/install.sh
```

Defaults:

- data: `/etc/x-port`
- binary: `/usr/local/bin/xport`
- Xray: `/usr/local/x-port/bin/xray`
- panel: `127.0.0.1:8080`

The installer asks for the bootstrap admin password without echoing it. X-port stores only a bcrypt hash. During an X-Panel migration, compatible old panel credentials replace this bootstrap login.

For remote access on a fresh install, keep the panel bound to localhost and put it behind an HTTPS reverse proxy. A migration deliberately preserves the old X-Panel listen address/port so the cutover does not unexpectedly change how the existing panel is reached.

## Migrate from X-Panel / 3x-ui

The common X-Panel paths are detected by default:

- database: `/etc/x-ui/x-ui.db`
- Xray config backup source: `/usr/local/x-ui/bin/config.json`

After installing X-port:

```bash
sudo ./scripts/migrate-xpanel.sh
```

Migration workflow:

1. Read the old SQLite database in **read-only** mode and print a dry-run.
2. Detect compatible WebUI administrators and the old `webListen` / `webPort` settings. Password hashes are never printed.
3. Ask for confirmation.
4. Snapshot current X-port database/config and record the old X-Panel service/timer states.
5. Stop and mask the old X-Panel related systemd services/timers so watchdogs cannot restart them during cutover.
6. Abort safely if a non-systemd watchdog still revives an old `/usr/local/x-ui/` process.
7. Import compatible one-client inbounds.
8. Replace the bootstrap X-port WebUI administrators with compatible X-Panel usernames and bcrypt password hashes, preserving the same login passwords.
9. Persist the old panel listen address and port in X-port settings.
10. Render and validate the Xray config, then start X-port Xray and panel.
11. If any cutover step fails, stop X-port, restore its previous database/config, restore the old unit states, restart the old panel when it was previously active, and verify rollback.

Multi-client inbounds are never silently split. They are reported and block `--apply` until handled explicitly.

If an old administrator password is not a recognizable bcrypt hash, that record is not imported automatically; the dry-run warns about it and the bootstrap X-port login is retained unless another valid old administrator exists.

## Account behavior

Creating an account currently targets VLESS/REALITY. Imported non-VLESS accounts are preserved, but advanced editing/sharing is intentionally limited until that protocol has a first-class editor.

Cloning keeps the transport/REALITY configuration but allocates a new UUID, resets traffic, and uses a new port (automatic when left blank).

## Security notes

- Panel session uses HttpOnly + SameSite=Strict cookie.
- Fresh installs default to localhost-only HTTP.
- Source migration is read-only.
- X-Panel bcrypt password hashes are reused directly; plaintext passwords are never needed or logged.
- Xray mutations are validated before activation.
- Failed Xray config/service activation restores the previous config.
- Migration rollback restores the complete pre-cutover X-port database, including panel login/listen settings.
- Xray binary update verifies the release asset SHA-256 digest and keeps a previous binary for rollback.
- No telemetry, ads, licensing server or external QR service.
