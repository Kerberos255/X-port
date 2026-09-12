# X-port

<p align="center">
  <strong>面向个人 Linux 代理服务器的小型、单节点 Xray 控制面板。</strong>
</p>

<p align="center">
  <a href="README.md">English</a> · <strong>简体中文</strong>
</p>

<p align="center">
  <a href="https://github.com/Kerberos255/X-port/releases/latest"><img src="https://img.shields.io/github/v/release/Kerberos255/X-port?label=release" alt="Release"></a>
  <a href="https://github.com/Kerberos255/X-port/actions/workflows/ci.yml"><img src="https://github.com/Kerberos255/X-port/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Kerberos255/X-port/actions/workflows/security.yml"><img src="https://github.com/Kerberos255/X-port/actions/workflows/security.yml/badge.svg" alt="Security"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="License: AGPL-3.0"></a>
</p>

X-port 的账号模型刻意保持简单：

> **1 个账号 = 1 个 inbound = 1 个独立端口 = 1 组凭据**

界面直接以“账号”为核心，而不是暴露 Xray 的 inbound / client 层级结构。

## 免责声明

X-port 仅用于合法的个人用途和技术学习。使用者有责任遵守所在地适用的法律法规。

请勿将 X-port 用于任何违法活动。使用者自行承担因使用本软件产生的一切风险与后果。

X-port 面向小型个人部署，不适用于多租户托管或关键业务基础设施。

本软件按 GNU Affero General Public License v3.0 的约定“按原样”提供，不附带任何形式的担保。

## 快速开始

X-port 当前支持 **systemd Linux**，架构支持 **amd64** 和 **arm64**。

安装最新稳定版：

```bash
curl -fsSL https://raw.githubusercontent.com/Kerberos255/X-port/main/install.sh | sudo bash
```

引导安装器会：

1. 检测服务器架构；
2. 获取最新稳定 GitHub Release；
3. 下载对应架构的 X-port 二进制文件和 `SHA256SUMS`；
4. 在执行前校验二进制文件的 SHA-256；
5. 从**同一个 Release tag** 下载安装脚本和迁移辅助脚本；
6. 进入正常的 X-port 安装流程。

如需安装指定版本，可设置 `XPORT_VERSION`：

```bash
curl -fsSL https://raw.githubusercontent.com/Kerberos255/X-port/main/install.sh | sudo XPORT_VERSION=v0.1.2 bash
```

安装器会要求设置管理员密码（至少 12 个字符），输入过程不会回显。系统只保存密码的 bcrypt 哈希。面板默认监听 `127.0.0.1:8080`；如需远程访问，建议放在 HTTPS 反向代理之后，或明确修改监听地址。

安装完成后，可通过以下命令打开终端管理器：

```bash
sudo xport
```

服务、更新、备份、防火墙和迁移相关命令请参阅 [X-port 终端管理器](docs/terminal-manager.md)。

## 为什么选择 X-port？

X-port 刻意保持比大型多用户代理平台更窄的功能边界：

- **可预测的账号模型** —— 一个账号对应一个 Xray inbound、一个端口和一组凭据。
- **运行时体积小** —— 面板是单个 Go 二进制文件，WebUI 已内嵌；服务器不需要 Node.js/npm。
- **默认保守暴露** —— 全新安装默认只监听本机，而不是自动把管理界面暴露到公网。
- **受保护的迁移流程** —— x-ui / 3x-ui 迁移使用 dry-run、快照、Xray 配置校验和回滚，不会盲目复制数据库。
- **事务式恢复** —— 备份恢复前会创建恢复点，应用或重启失败时自动恢复运行状态。
- **经过验证的更新** —— Xray 和 X-port 更新都会校验 Release 完整性、验证候选版本，并提供回滚保护。
- **防火墙职责明确** —— WebUI 不会静默改写防火墙规则；常见防火墙辅助操作保留在终端管理器中。

## 功能

- 账号新增 / 编辑 / 删除 / 克隆。
- 独立端口分配，包含数据库唯一性检查以及只读 TCP/UDP 端口占用检查。
- 支持 **VLESS、VMess、Trojan、Shadowsocks、SOCKS、HTTP** 协议编辑。
- 新建 VLESS 账号默认使用 **REALITY + Vision**。
- 对支持 URI 格式的协议提供单账号分享链接和二维码。
- 批量导出账号 ZIP，包含 `links.txt`、二维码 PNG，以及无法安全表示账号的 `warnings.txt`。
- 基于 Xray StatsService 的单账号实时流量统计。
- 流量额度和到期时间策略。
- 手动重置当前周期流量。
- 可选的**按账号每月重置流量**。进入新月份后，首次运行检查会对符合条件的账号只重置一次当前上传/下载，因此即使服务器在每月 1 日离线也不受影响；累计流量不会清零。
- 因额度耗尽自动停用的账号，会在月度重置后重新启用；手动停用或已到期账号不会自动启用。
- 从 journald 查看 Xray 服务日志，并提供受控服务重启。
- 本地压缩备份，支持下载 / 删除、导入和事务式恢复。
- 面板设置：监听地址、Base Path、TLS 证书/私钥路径、管理员凭据、Xray API 端口、账号自动端口范围和默认 REALITY 目标。
- Xray Core 稳定版更新：GitHub SHA-256 校验、配置验证和回滚。
- `geoip.dat` / `geosite.dat` 独立更新并成对回滚。
- X-port 稳定版自更新：校验候选版本和 SHA-256，并通过 systemd 重启看门狗在新服务无法稳定运行时恢复旧二进制文件。
- 响应式深色 / 浅色 WebUI，移动端使用底部导航布局。

X-port **不会通过 WebUI 修改防火墙规则**。账号端口需要使用你原本的防火墙工具自行放行或关闭。终端管理器提供常见防火墙环境下的受保护辅助操作。

## 运行结构

```text
browser
   │ HTTP / HTTPS
   ▼
xport (单个 Go 二进制)
   ├── 内嵌 WebUI
   ├── SQLite /etc/x-port/xport.db
   ├── 账号策略 + 流量采集器
   ├── 备份 / 更新 / 迁移逻辑
   └── 本地 Xray StatsService
             │ 默认 127.0.0.1:10085
             ▼
          xray-core
             │
             └── 每个账号一个 inbound / 独立端口
```

Linux 服务器运行时不依赖 Node.js/npm。构建后的 Go 二进制会内嵌 `web/dist`。

`scripts/install.sh` 的默认安装路径：

```text
/usr/local/bin/xport
/usr/local/x-port/bin/xray
/etc/x-port/xport.db
/etc/x-port/xray/config.json
/etc/x-port/backups/
/etc/x-port/xport.env       # 可选；X-port 不会在其中自动创建凭据
/etc/systemd/system/xport.service
/etc/systemd/system/xport-xray.service
```

## 从源码构建

开发环境要求：

- Go 1.27+
- Node.js 仅用于 JavaScript 语法检查；当前 WebUI 已作为静态文件提交到仓库。

```bash
./scripts/build-linux.sh
sudo ./scripts/install.sh
```

常用检查：

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

如果检测到兼容的现有 x-ui / 3x-ui，安装器会刻意避免让新 Xray 服务直接占用旧端口，请改用受保护的迁移流程。

## 从 x-ui / 3x-ui 迁移

迁移流程设计为受保护的切换，而不是盲目复制数据库。安装完成后先打开终端管理器：

```bash
sudo xport
```

选择 x-ui / 3x-ui 迁移项目。受保护流程会：

1. 读取旧 SQLite 数据库，并在任何修改前报告可兼容 / 被跳过的 inbounds。
2. 强制遵循 X-port 账号模型：可迁移 inbound 必须表示“一个账号对应一个端口”。多 client inbound 会被跳过，并阻止直接应用，直到人工确认。
3. 在兼容时保留旧面板管理员用户名和 bcrypt 密码哈希。
4. 保留面板监听地址/端口、Web Base Path、域名，以及完整的 TLS 证书/私钥组合。
5. 在实际切换前校验迁移后的 TLS 证书/私钥文件。
6. 从旧生成配置中保留安全的全局 Xray 配置段，包括 routing、DNS、outbounds、policy，而不是静默替换成默认值。
7. 记录相关旧服务和 timer，并在切换期间抑制它们，避免 watchdog 立即抢回旧代理端口。
8. 启动新 Xray 前检查是否仍有旧 x-ui/Xray 进程残留。如果非 systemd watchdog 已重新拉起旧栈，迁移会中止，而不是与旧进程抢端口。
9. 同时对现有 X-port 数据库/配置，以及旧面板数据库/配置创建快照。
10. 渲染候选 Xray 配置并先执行 `xray run -test`。
11. 切换后验证 `xport-xray.service` 和 `xport.service` 均处于 active。
12. 出错或中断时，恢复迁移前 X-port 快照，并恢复旧 systemd unit 的 enable / active 状态。
13. 旧面板文件仍保留在磁盘，并在 X-port 备份目录生成对应快照的 `rollback.sh`。

迁移成功后，相关旧 systemd unit 会继续保持被抑制状态，防止旧 watchdog 意外复活面板。切换前仍应检查服务器真实的 watchdog、进程管理器和 cron 配置；如果存在唤醒周期很长的自定义非 systemd watchdog，应提前明确识别。

## 流量统计

X-port 启用 Xray 本地 API/StatsService，并按 inbound tag 读取流量。这与“一账号一 inbound”的模型一致，不依赖特定协议的 client email 字段。

每次采集周期会：

1. 读取并重置 Xray inbound 计数器；
2. 把增量写入 SQLite 中账号的当前周期和累计计数；
3. 评估到期和额度策略；
4. 进入新月份时，只重置开启月度重置且记录月份早于当前月份的账号；
5. 通过与普通账号编辑相同的校验 / 重启 / 回滚路径应用 Xray 配置变更。

## 备份

系统页可以创建并下载压缩快照。快照包含恢复所需的账号记录、原始协议/stream 设置、面板/系统设置和管理员密码哈希。

导入的备份文件会先经过校验并加入备份列表，**不会立即修改当前运行配置**。选择恢复时会先额外创建一个恢复前快照；如果恢复后的账号集应用失败，运行状态会回滚。

## 更新

### Xray Core

X-port 从官方 `XTLS/Xray-core` 仓库读取稳定 Release。一键更新通道会忽略 draft 和 prerelease。Release asset 必须提供 SHA-256 digest；候选二进制必须能够正常执行，并通过当前 Xray 配置验证后才会替换。

### GeoData

`geoip.dat` 和 `geosite.dat` 与核心二进制分开更新。两个文件均来自同一个官方稳定 Xray Release 压缩包，使用 Release asset SHA-256 校验，并以成对方式替换；如果 Xray 无法重启，则成对回滚。

### X-port 自身

X-port 自更新通道要求 `Kerberos255/X-port` 的稳定 Release 提供以下架构资产：

```text
xport-linux-amd64
xport-linux-arm64
```

GitHub Release asset 必须提供 SHA-256 digest，同时候选二进制的 `xport version` 输出必须与 Release tag 匹配。

对于**公开仓库**，检查和下载稳定 Release 不需要 GitHub token。私有副本或 fork 可以通过 root-only 的 `/etc/x-port/xport.env` 文件可选提供 `XPORT_GITHUB_TOKEN`。该 token 不会保存到 SQLite，也不会通过 WebUI/API 返回。

实际执行自更新时，旧可执行文件会保留为 `.previous`。临时 systemd unit 会在面板自身 cgroup 之外重启 X-port，等待新服务保持 active；如果验证失败，则恢复并重新启动上一版二进制文件。

## 安全说明

- Session 使用随机 HttpOnly、SameSite=Strict Cookie；直接 TLS 或可信反向代理收到的 HTTPS 请求会设置 Secure。
- 已认证的状态变更 API 会拒绝跨源浏览器请求。
- 登录失败同时按 `IP + username` 和全 IP 内存桶限流。
- 账号列表响应不会返回凭据 / 私钥；敏感信息仅通过已认证的单账号编辑 / 分享接口返回。
- 分享 JSON/QR 与批量导出均要求认证，并在适用位置使用 no-store 语义。
- 导入备份具有大小限制，且会先解析校验后再接收。
- 面板 TLS 设置只在浏览器中保存文件系统路径，而不是证书 / 私钥内容。
- systemd 服务使用 `NoNewPrivileges`、`PrivateTmp` 和只读 home 可见性。保留只读可见性是为了兼容迁移后的证书路径。
- 防火墙管理保持显式、本地化；X-port 不会静默改写自定义规则集。

## 兼容性

X-port 对 VLESS、VMess、Trojan、Shadowsocks、SOCKS 和 HTTP 提供专用编辑器。迁移得到的其他协议 inbound 可以保留在存储的原始配置模型中，但 WebUI 会将其视为只读，而不会猜测如何重写未知的高级 JSON。

应用迁移前，请通过 dry-run 输出检查所有 warning 和被跳过的 inbound。

## 许可证

X-port 使用 **GNU Affero General Public License v3.0（AGPL-3.0-only）**。

如果你修改 X-port，并通过网络向用户提供修改后的版本，AGPL 要求你向这些用户提供对应源代码。

`v0.1.2` 之前发布的版本仍按其发布时适用的许可证条款提供，包括更早采用 MIT 许可证的版本。
