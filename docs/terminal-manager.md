# X-port terminal manager

After X-port is installed, run:

```bash
sudo xport
```

with no arguments to open the interactive management menu.

The menu intentionally keeps server operations in the terminal while the WebUI stays focused on accounts and Xray configuration.

## Menu

```text
  1. 查看状态 / 面板地址
  2. 面板设置
  3. 修改管理员账号 / 密码

  4. 启动 X-port
  5. 停止 X-port
  6. 重启 X-port
  7. 重启 Xray
  8. 查看日志

  9. 更新 X-port
 10. 更新 Xray
 11. 更新 GeoData

 12. 备份与恢复
 13. x-ui / 3x-ui 迁移
 14. 开机自启设置
 15. 防火墙管理（仅终端）
 16. 修复 systemd 安装
 17. 卸载 X-port

  0. 退出
```

The original low-level commands (`serve`, `init`, `migrate`, `render`, `xray-check`, `xray-update`, `version`) remain available for scripts and automation.

Useful direct management commands include:

```bash
xport status
sudo xport start
sudo xport stop
sudo xport restart
sudo xport restart-xray
xport logs xport
xport logs xray
xport logs xray --follow
xport autostart status
sudo xport autostart on
sudo xport autostart off
sudo xport settings
sudo xport admin
sudo xport update
sudo xport update-xray
sudo xport update-geodata
sudo xport backup
sudo xport firewall
sudo xport repair
sudo xport uninstall
```

## Firewall policy

Firewall mutation is deliberately **terminal-only**. No firewall controls are added to the WebUI.

- `ufw`: view rules, allow a TCP/UDP/both port, remove an allow rule.
- active `firewalld`: view rules, add/remove runtime and permanent TCP/UDP/both ports.
- custom `nftables` / `iptables`: read-only rule display. X-port does not guess how a custom ruleset should be modified.

This keeps the common personal-server cases convenient without rewriting an administrator's custom firewall policy.

## Backup restore

The terminal restore path creates a pre-restore backup, validates the target panel settings, applies the target Xray configuration, replaces the database snapshot and restarts the panel. If the database update or panel restart fails, the previous snapshot/Xray configuration is restored.

## Migration

`scripts/install.sh` installs the guarded migration helper under `/usr/local/lib/xport/`.

The menu's x-ui / 3x-ui migration item runs that guarded script. It retains the existing dry-run, backup, watchdog suppression and rollback behavior rather than bypassing it with a simpler migration path.

## First installation

The interactive manager can only exist after the X-port binary has been installed. A fresh source checkout therefore still uses the bootstrap once:

```bash
./scripts/build-linux.sh
sudo ./scripts/install.sh
```

After that, routine administration should normally use `sudo xport`.