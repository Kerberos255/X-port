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
- X-Panel / 3x-ui read-only migration with dry-run and backup
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

The installer asks for the admin password without echoing it. X-port stores only a bcrypt hash.

For remote access, keep the panel bound to localhost and put it behind an HTTPS reverse proxy.

## Migrate from X-Panel / 3x-ui

The common X-Panel paths are detected by default:

- database: `/etc/x-ui/x-ui.db`
- Xray config backup source: `/usr/local/x-ui/bin/config.json`

After installing X-port:

```bash
sudo ./scripts/migrate-xpanel.sh
```

Migration workflow:

1. Read old SQLite database in **read-only** mode and print a dry-run.
2. Ask for confirmation.
3. Stop the old service.
4. Back up old database/config and current X-port state.
5. Import only compatible one-client inbounds.
6. Render and validate the Xray config.
7. Start X-port Xray and panel.
8. If any cutover step fails, restart the old panel automatically.

Multi-client inbounds are never silently split. They are reported and block `--apply` until handled explicitly.

## Account behavior

Creating an account currently targets VLESS/REALITY. Imported non-VLESS accounts are preserved, but advanced editing/sharing is intentionally limited until that protocol has a first-class editor.

Cloning keeps the transport/REALITY configuration but allocates a new UUID, resets traffic, and uses a new port (automatic when left blank).

## Security notes

- Panel session uses HttpOnly + SameSite=Strict cookie.
- Panel defaults to localhost-only HTTP.
- Source migration is read-only.
- Xray mutations are validated before activation.
- Failed Xray config/service activation restores the previous config.
- Xray binary update verifies the release asset SHA-256 digest and keeps a previous binary for rollback.
- No telemetry, ads, licensing server or external QR service.
