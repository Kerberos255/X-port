# X-port

<p align="center">
  <strong>A small, single-node Xray control panel for personal Linux proxy servers.</strong>
</p>

<p align="center">
  <a href="https://github.com/Kerberos255/X-port/releases/latest"><img src="https://img.shields.io/github/v/release/Kerberos255/X-port?label=release" alt="Release"></a>
  <a href="https://github.com/Kerberos255/X-port/actions/workflows/ci.yml"><img src="https://github.com/Kerberos255/X-port/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Kerberos255/X-port/actions/workflows/security.yml"><img src="https://github.com/Kerberos255/X-port/actions/workflows/security.yml/badge.svg" alt="Security"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="License: AGPL-3.0"></a>
</p>

Its account model is intentionally simple:

> **1 account = 1 inbound = 1 dedicated port = 1 credential set**

The UI presents accounts directly rather than exposing Xray's inbound/client hierarchy.

## Disclaimer

X-port is intended for lawful personal use and technical learning. Users are responsible for complying with all applicable local laws and regulations.

Do not use X-port for unlawful activities. Users assume all risks and consequences arising from its use.

X-port is designed for small personal deployments and is not intended for multi-tenant hosting or mission-critical infrastructure.

This software is provided "as is", without warranty of any kind, as described in the GNU Affero General Public License v3.0.

## Quick start

X-port currently supports **systemd Linux** on **amd64** and **arm64**.

Install the latest stable release:

```bash
curl -fsSL https://raw.githubusercontent.com/Kerberos255/X-port/main/install.sh | sudo bash
```

The bootstrap installer:

1. detects the server architecture;
2. resolves the latest stable GitHub Release;
3. downloads the matching X-port binary and `SHA256SUMS`;
4. verifies the binary's SHA-256 checksum before executing it;
5. downloads the installer and migration helper from the **same release tag**;
6. runs the normal X-port installation flow.

To install a specific release, set `XPORT_VERSION`:

```bash
curl -fsSL https://raw.githubusercontent.com/Kerberos255/X-port/main/install.sh | sudo XPORT_VERSION=v0.1.2 bash
```

The installer asks for an administrator password (minimum 12 characters) without echoing it. Only its bcrypt hash is stored. The panel listens on `127.0.0.1:8080` by default; for remote access, place it behind an HTTPS reverse proxy or intentionally change the listen address.

After installation, open the terminal manager with:

```bash
sudo xport
```

See [X-port terminal manager](docs/terminal-manager.md) for service, update, backup, firewall and migration commands.

## Why X-port?

X-port deliberately keeps a narrower scope than large multi-user proxy platforms:

- **Predictable account model** — one account maps to one Xray inbound, one port and one credential set.
- **Small runtime footprint** — the panel is a single Go binary with its WebUI embedded; Node.js/npm is not required on the server.
- **Conservative exposure by default** — a fresh installation binds the panel to localhost rather than publishing the admin UI to the Internet automatically.
- **Guarded migration** — x-ui / 3x-ui migration uses dry-run checks, snapshots, Xray config validation and rollback instead of blindly copying a database.
- **Transactional recovery** — backup restore creates a pre-restore snapshot and rolls back the running state when apply/restart fails.
- **Verified updates** — Xray and X-port update flows verify release integrity and validate candidates before replacement, with rollback protection.
- **Explicit firewall ownership** — the WebUI never silently rewrites firewall rules; common firewall helpers stay in the terminal manager.

## What it provides

- Account create / edit / delete / clone.
- Dedicated-port allocation with database uniqueness checks and read-only TCP/UDP port occupancy checks.
- Protocol editors for **VLESS, VMess, Trojan, Shadowsocks, SOCKS and HTTP**.
- New VLESS accounts default to **REALITY + Vision**.
- Per-account share links and QR codes where the protocol has a supported URI format.
- Bulk account export as a ZIP containing `links.txt`, QR PNG files and `warnings.txt` for accounts that cannot be represented safely.
- Live per-account traffic accounting from Xray StatsService.
- Traffic quota and expiry enforcement.
- Manual current-period traffic reset.
- Optional **per-account monthly reset**. After entering a new month, the first runtime check resets current upload/download exactly once, so it still works if the server was offline on the first day. Lifetime traffic is preserved.
- Accounts disabled automatically because of quota are re-enabled after their monthly reset; manually disabled or expired accounts are not.
- Xray service logs from journald and controlled service restart.
- Local compressed backups with download/delete, import and transactional restore.
- Panel settings for listen address, Base Path, TLS certificate/key paths, admin credentials, Xray API port, automatic account-port range and default REALITY target.
- Stable-channel Xray core update with GitHub SHA-256 verification, config validation and rollback.
- Separate `geoip.dat` / `geosite.dat` update with pairwise rollback.
- X-port self-update from stable GitHub Releases with candidate version + SHA-256 verification and a systemd restart watchdog that restores the previous binary if the new service cannot stay active.
- Responsive dark/light WebUI with a mobile bottom navigation layout.

X-port **does not modify firewall rules from the WebUI**. Open or close account ports yourself with the firewall tooling you already use. The terminal manager provides guarded helpers for common firewall setups.

## Runtime layout

```text
browser
   │ HTTP / HTTPS
   ▼
xport (single Go binary)
   ├── embedded WebUI
   ├── SQLite /etc/x-port/xport.db
   ├── account policy + traffic collector
   ├── backup / update / migration logic
   └── local Xray StatsService
             │ 127.0.0.1:10085 by default
             ▼
          xray-core
             │
             └── one inbound / dedicated port per account
```

There is no Node.js/npm runtime dependency on the Linux server. The built Go binary embeds `web/dist`.

Default paths installed by `scripts/install.sh`:

```text
/usr/local/bin/xport
/usr/local/x-port/bin/xray
/etc/x-port/xport.db
/etc/x-port/xray/config.json
/etc/x-port/backups/
/etc/x-port/xport.env       # optional; not created with secrets by X-port
/etc/systemd/system/xport.service
/etc/systemd/system/xport-xray.service
```

## Build from source

Requirements for development:

- Go 1.27+
- Node.js only for JavaScript syntax checking; the current WebUI is already committed as static files.

```bash
./scripts/build-linux.sh
sudo ./scripts/install.sh
```

Useful checks:

```bash
node --check web/dist/app.js
node --check web/dist/extra.js
node --check web/dist/v3.js
bash -n install.sh scripts/build-linux.sh scripts/install.sh scripts/migrate-*.sh
go mod tidy
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/xport ./cmd/xport
```

If an existing compatible x-ui / 3x-ui installation is detected, the installer deliberately does **not** start the new Xray service on the old ports. Use the guarded migration flow instead.

## Migration from x-ui / 3x-ui

The migration is designed as a guarded cutover, not a blind database copy. After installation, open the terminal manager:

```bash
sudo xport
```

Choose the x-ui / 3x-ui migration item. The guarded flow:

1. Reads the old SQLite database and reports compatible/skipped inbounds before changing anything.
2. Requires the X-port account model: supported migrated inbounds must represent one account on one port. Multi-client inbounds are skipped and block apply until reviewed.
3. Preserves the old panel admin username and bcrypt password hash when compatible.
4. Preserves the panel listen address/port, Web Base Path, domain and a complete TLS certificate/key pair.
5. Validates migrated TLS certificate/key files **before** applying the cutover.
6. Preserves safe global Xray configuration sections from the old generated config, including routing, DNS, outbounds and policy instead of silently replacing them with defaults.
7. Records relevant legacy services and timers, then suppresses them during cutover so a watchdog cannot immediately reclaim the old proxy ports.
8. Checks for surviving legacy x-ui/Xray processes before starting the new Xray. If a non-systemd watchdog has already respawned the old stack, migration aborts rather than racing for ports.
9. Snapshots the existing X-port database/config as well as the legacy panel database/config.
10. Renders the candidate Xray config and runs `xray run -test` before starting the new service.
11. Verifies `xport-xray.service` and `xport.service` are active after cutover.
12. On an error or interruption, restores the pre-migration X-port snapshot and the old systemd unit enable/active states.
13. Leaves the legacy panel files on disk and writes a snapshot-specific `rollback.sh` under the X-port backup directory.

The old related systemd units remain suppressed after a successful migration so their watchdogs cannot revive the old panel unexpectedly. Before cutover, inspect the server's actual watchdog, process-manager and cron setup; a custom non-systemd watchdog with a long wake-up interval should be identified explicitly.

## Traffic accounting

X-port enables Xray's local API/StatsService and reads traffic by inbound tag. This fits the one-account/one-inbound model and does not depend on a protocol-specific client email field.

Every collection cycle:

1. read and reset Xray's inbound counters;
2. add the delta to the account's current-period and lifetime counters in SQLite;
3. evaluate expiry and quota policy;
4. when entering a new month, reset only accounts whose monthly-reset option is enabled and whose recorded reset month is older than the current month;
5. apply Xray config changes through the same validate/restart/rollback path used by normal account edits.

## Backups

The system page can create and download compressed snapshots. A snapshot contains the account records, raw protocol/stream settings, panel/system settings and admin password hashes needed for restoration.

Imported backup files are validated and added to the backup list **without immediately changing the running configuration**. Choosing Restore creates an additional pre-restore snapshot first; if applying the restored account set fails, the running state is rolled back.

## Updates

### Xray core

X-port reads stable releases from the official `XTLS/Xray-core` repository. Drafts and prereleases are ignored by the one-click channel. The release asset must expose a SHA-256 digest. The candidate binary must execute successfully and accept the current Xray config before replacement.

### GeoData

`geoip.dat` and `geosite.dat` are updated separately from the core binary. Both are taken from the same stable official Xray release archive, verified by the release asset SHA-256, replaced as a pair and rolled back as a pair if Xray cannot restart.

### X-port itself

The self-update channel expects stable releases in `Kerberos255/X-port` with architecture assets named:

```text
xport-linux-amd64
xport-linux-arm64
```

The GitHub release asset must provide a SHA-256 digest and the candidate binary's `xport version` output must match the release tag.

For a **public repository**, no GitHub token is required to check or download stable Releases. Private copies or forks can optionally provide `XPORT_GITHUB_TOKEN` through the root-only `/etc/x-port/xport.env` file. The token is never saved to SQLite or returned by the WebUI/API.

During an actual self-update, the old executable is kept as `.previous`. A transient systemd unit restarts X-port outside the panel's own cgroup, waits for the new service to remain active, and restores/restarts the previous binary if verification fails.

## Security notes

- Sessions use random HttpOnly, SameSite=Strict cookies; Secure is set for direct TLS or HTTPS requests received through a trusted reverse proxy.
- State-changing authenticated API calls reject cross-origin browser requests.
- Login failures are rate-limited by both `IP + username` and IP-wide buckets in memory.
- Account list responses omit credentials/private keys; sensitive material is returned only by authenticated single-account edit/share operations.
- Share JSON/QR and bulk export are authenticated and sent with no-store semantics where applicable.
- Imported backups are size-limited and parsed before being accepted.
- Panel TLS settings store filesystem paths, not certificate/private-key contents in the browser.
- systemd services use `NoNewPrivileges`, `PrivateTmp` and read-only home visibility. Read-only visibility is intentional so migrated certificate paths can still be used.
- Firewall management remains explicit and local; X-port does not silently rewrite custom rulesets.

## Compatibility

X-port has dedicated editors for VLESS, VMess, Trojan, Shadowsocks, SOCKS and HTTP. A migrated inbound using another protocol can remain in the stored/raw configuration model, but the WebUI treats it as read-only rather than guessing how to rewrite unknown advanced JSON.

Use migration dry-run output to review every warning or skipped inbound before applying a cutover.


## License

X-port is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0-only)**. If you modify X-port and make that modified version available to users over a network, the AGPL requires those users to be offered the corresponding source code.

Releases published before `v0.1.2` remain available under the license terms that applied to those releases, including the earlier MIT-licensed versions.
