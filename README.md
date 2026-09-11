# X-port

X-port is a small, single-node Xray control panel for a personal Linux proxy server.

Its account model is intentionally simple:

> **1 account = 1 inbound = 1 dedicated port = 1 credential set**

The UI talks about accounts rather than exposing Xray's inbound/client hierarchy. X-port is currently **alpha** and is designed for a small personal deployment rather than multi-tenant hosting.

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
- Optional **per-account monthly reset**: after entering a new month, the first runtime check resets current upload/download exactly once. It therefore still works if the server was offline on the first day. Lifetime traffic is preserved.
- Accounts disabled automatically because of quota are re-enabled after their monthly reset; manually disabled or expired accounts are not.
- Xray service logs from journald and controlled service restart.
- Local compressed backups, download/delete, backup import and transactional restore.
- Panel settings for listen address, Base Path, TLS certificate/key paths, admin credentials, Xray API port, automatic account-port range and default REALITY target.
- Stable-channel Xray core update with GitHub SHA-256 verification, config validation and rollback.
- Separate `geoip.dat` / `geosite.dat` update with pairwise rollback.
- X-port self-update support from stable GitHub Releases with candidate version + SHA-256 verification and a systemd restart watchdog that restores the previous binary if the new service cannot stay active.
- Responsive dark WebUI with a mobile bottom navigation layout.

X-port **does not modify firewall rules**. Open/close account ports yourself with the firewall tooling you already use.

## Runtime layout

The deployed runtime is deliberately small:

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

## Build

Requirements for development:

- Go 1.27+
- Node.js only for JavaScript syntax checking; the current WebUI is already committed as static files.

```bash
./scripts/build-linux.sh
```

Useful checks:

```bash
node --check web/dist/app.js
node --check web/dist/extra.js
bash -n scripts/build-linux.sh scripts/install.sh scripts/migrate-xpanel.sh
go mod tidy
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/xport ./cmd/xport
```

## Fresh install

Run as root on a systemd Linux host:

```bash
./scripts/build-linux.sh
sudo ./scripts/install.sh
```

The installer asks for an admin password without echoing it. Only its bcrypt hash is stored. The initial panel listen address defaults to `127.0.0.1:8080` unless `XPORT_LISTEN` is set.

If an existing x-ui/X-Panel database or service is detected, the installer deliberately does **not** start the new Xray service on the old ports. Use the guarded migration flow instead.

## X-Panel / 3x-ui migration

The migration is designed as a guarded cutover, not a blind database copy.

```bash
sudo ./scripts/migrate-xpanel.sh
```

The migration flow:

1. Reads the old SQLite database and reports compatible/skipped inbounds before changing anything.
2. Requires the X-port account model: supported migrated inbounds must represent one account on one port. Multi-client inbounds are skipped and block apply until reviewed.
3. Preserves the old panel admin username and bcrypt password hash when compatible.
4. Preserves the panel listen address/port, Web Base Path, domain and a complete TLS certificate/key pair.
5. Validates migrated TLS certificate/key files **before** applying the cutover.
6. Preserves safe global Xray configuration sections from the old generated config, including routing, DNS, outbounds and policy instead of silently replacing them with defaults.
7. Records relevant old X-Panel/x-ui services and timers, then `mask --now`s them so a systemd watchdog cannot immediately reclaim the old proxy ports.
8. Checks for surviving `/usr/local/x-ui/...` processes before and immediately before starting the new Xray. If a non-systemd watchdog has already respawned the old stack, migration aborts rather than racing for ports.
9. Snapshots the existing X-port database/config as well as the old panel database/config.
10. Renders the candidate Xray config and runs `xray run -test` before starting the new service.
11. Verifies `xport-xray.service` and `xport.service` are active after cutover.
12. On an error or interruption, restores the pre-migration X-port snapshot and the old systemd unit enable/active states.
13. Leaves the old X-Panel files on disk and writes a snapshot-specific `rollback.sh` under `/etc/x-port/backups/xpanel-.../`.

The old related systemd units remain masked after a successful migration so their watchdogs cannot revive the old panel unexpectedly.

**Production preflight still matters:** before migrating a real server, inspect its actual watchdog/process manager/cron setup. The generic process guard catches a respawn that is already running, but a custom non-systemd watchdog with a long wake-up interval should be identified explicitly before cutover.

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

Because the repository is currently private, a deployed server needs GitHub read access to check/download private releases. X-port only reads the optional environment variable `XPORT_GITHUB_TOKEN`; it is never saved to SQLite or returned by the WebUI/API.

A convenient root-only setup is:

```bash
sudo sh -c 'umask 077; printf "%s\n" "XPORT_GITHUB_TOKEN=YOUR_TOKEN" > /etc/x-port/xport.env'
sudo systemctl restart xport.service
```

`xport.service` loads `/etc/x-port/xport.env` only if the file exists. If there is no token or no suitable Release yet, the WebUI reports the update channel as unavailable instead of pretending the current build is latest.

During an actual self-update, the old executable is kept as `.previous`. A transient systemd unit restarts X-port outside the panel's own cgroup, waits for the new service to remain active, and restores/restarts the previous binary if verification fails.

## Security notes

- Sessions use random HttpOnly, SameSite=Strict cookies; Secure is set for direct TLS or HTTPS reverse-proxy requests.
- State-changing authenticated API calls reject cross-origin browser requests.
- Login failures are rate-limited by both `IP + username` and IP-wide buckets in memory.
- Account list responses omit credentials/private keys; sensitive material is returned only by authenticated single-account edit/share operations.
- Share JSON/QR and bulk export are authenticated and sent with no-store semantics where applicable.
- Imported backups are size-limited and parsed before being accepted.
- Panel TLS settings store filesystem paths, not certificate/private-key contents in the browser.
- systemd services use `NoNewPrivileges`, `PrivateTmp` and read-only home visibility. Read-only visibility is intentional so migrated certificate paths such as `/root/cert/...` can still be used.
- Firewall management remains explicitly manual.

## Current compatibility boundary

X-port has dedicated editors for VLESS, VMess, Trojan, Shadowsocks, SOCKS and HTTP. A migrated inbound using another protocol can remain in the stored/raw configuration model, but the WebUI treats it as read-only rather than guessing how to rewrite unknown advanced JSON.

The project is still alpha. Before a real X-Panel cutover, use the migration dry-run, inspect every warning/skipped inbound and verify the exact old watchdog mechanism on that server.
