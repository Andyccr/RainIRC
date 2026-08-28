# Changelog

All notable changes to P2P-IRC are documented here.
本文件记录 P2P-IRC 的用户可见变更。

The format follows [Keep a Changelog](https://keepachangelog.com/).
Versions match `internal/version`.

## [0.5.1] - 2026-08-28

### Deploy / 一次性部署

- `scripts/install.sh` prefers the GitHub Release binary and checks SHA-256
  before replacing `p2pirc`. `go install` is the fallback.
  安装脚本优先下 Release 并校验校验和，失败再 `go install`。
- Install is atomic (`p2pirc.new` then rename).
  安装用临时文件再改名，避免写到一半。
- CI disables the Go module cache (this module has no `go.sum`) and Release
  notes stay short instead of dumping every historical PR.
  CI 关掉无效的模块缓存；Release 说明不再罗列全部历史 PR。
- Quote the Release workflow `run` as a block so a colon in `--notes` cannot
  break YAML parsing (Actions was creating empty failed runs).
  Release 工作流的 `run` 改成块标量，避免 YAML 把说明里的冒号当成映射。

## [0.5.0] - 2026-08-28

### Deploy / 一次性部署

- `go install github.com/Andyccr/RainIRC/cmd/p2pirc@latest` and
  `scripts/install.sh` (default `~/.local/bin`, no root, no Docker).
  一条命令安装；默认装到 `~/.local/bin`。
- `make install` / `make dist` (linux/darwin/windows amd64+arm64) and a
  GitHub Release workflow on `v*` tags.
  交叉编译与打标签发 Release。
- `--lan` turns on auto-connect + reconnect for a LAN mesh in one flag.
  `--lan` 一键打开局域网自动连接与重连。
- Optional `~/.p2pirc/config` (CLI wins). Write once, then run `p2pirc`.
  可选本地配置文件，命令行优先。
- Optional systemd user unit in `contrib/p2pirc.service`.
  可选 systemd 用户服务。

### Architecture / 架构

- Version and commit can be set at link time (`-X ...Version=`).
  版本号可用 ldflags 写入。
- Relays are still not in the tree. 0.5.0 is deploy convenience, not a
  rendezvous network.
  0.5.0 不做中继，只把部署变简单。

## [0.4.3] - 2026-08-23

### Architecture / 架构

- Per-peer reconnect backoff (5s → 60s) so a down neighbor is not redialed
  every tick. Success or a clean disconnect resets the delay.
  按节点指数退避重连（5 秒到 60 秒）；连上或正常断开后清零。
- One in-flight dial per address and a process-wide cap of 4 outbound
  handshakes. Auto-connect no longer blocks the 10s ticker.
  同一地址不叠拨；出站握手最多 4 路并行。
- CI runs on `main` and pull requests only (no duplicate branch-push jobs).
  CI 只跑 `main` 与 PR，避免同一提交跑两次。

### Fixed / 修复

- Handshake comment said protocol v1; the wire is v2.
  握手注释误写 v1，实际是协议 v2。
- Reject negative `--max-peers`.
  拒绝负数 `--max-peers`。

## [0.4.2] - 2026-08-23

### Architecture / 架构

- Split the node into lifecycle (`node.go`), commands (`commands.go`),
  gossip hooks (`handlers.go`), dial/reconnect (`connect.go`), and NAT
  (`nat.go`). Commands are a table, not a growing switch.
  节点按生命周期 / 命令 / gossip / 拨号 / NAT 拆文件；命令用表注册。
- Parallel dial of saved addresses (first TCP handshake wins, cap 4).
  多地址并行拨号，先握手成功者胜出。
- `--reconnect` keeps retrying known peers every 5s, not only at startup.
  `--reconnect` 持续重试已知节点，而不是只在启动时试一次。

### Added / 新增

- `/names` (`/who`): list channel members already tracked locally.
  `/names` 列出本机已知的频道成员。
- Nickname changes gossip through existing signed `join` frames (no protocol bump).
  改昵称走现有签名 `join`，不升握手版本。
- Per-sender gossip rate limit (30 frames/s, local drop, not forwarded).
  按发送者限制 gossip（每秒 30 帧，本机丢弃不转发）。
- Repeat `join` for an existing member is no longer printed as a second join
  (covers nick-change and connection resync).
  已在成员表中的重复 `join` 不再刷第二条「加入」通知。

## [0.4.1] - 2026-08-22

### Fixed / 修复

- Re-send channel `join`s when a duplicate TCP session is replaced.
  重复连接被替换时补发频道 `join`，避免成员表脱节。
- Serialize and debounce `peers.json` writes; do not persist unsigned hello `addrs`.
  串行并节流写入 `peers.json`；不再把未签名的 hello `addrs` 当作重连目标。
- Drop signed frames outside a ± clock window (2 minutes future / 10 minutes past).
  超出时钟窗口的签名帧直接丢弃，降低 seen 缓存过期后的重放。
- Cap inbound handshakes and live peers; disconnect after repeated malformed frames.
  限制同时握手与连接数；连续畸形帧断开。
- Discovery read errors no longer kill `/discover`; auto-connect no longer blocks the UDP reader.
  发现读错误不再停掉 `/discover`；自动连接不再堵住 UDP 读取。
- UPnP mappings use a 1-hour lease, renew while running, and unmap if Close races the mapper.
  UPnP 使用 1 小时租约、运行中续租，并处理 Close 竞态。
- Clear channel membership on peer disconnect; copy channel history on trim.
  对端断开时清频道成员；历史裁剪时拷贝切片以免泄漏。

### Performance / 性能

- Seen-ID eviction is insertion-order O(k), not O(n²).
  已见 ID 按插入顺序淘汰。
- TLS Ed25519 certificates are cached per identity.
  TLS 证书按身份缓存。
- Skip keepalive pings while a session is already receiving traffic; rate-limit pongs.
  有流量时跳过 ping；pong 限速。
- Gossip broadcast snapshots the peer list before sending.
  Gossip 发送前先拷贝连接表。

### Added / 新增

- `/stats`, `--max-peers` (default 64).
  `/stats` 与 `--max-peers`（默认 64）。

## [0.4.0] - 2026-08-22

### Added / 新增

- STUN Binding client (RFC 5389, IPv4) and `/addr` address candidates.
  STUN Binding 客户端与 `/addr` 地址候选。
- Optional `--upnp` IGD TCP port mapping (off by default, fail-soft).
  可选 `--upnp` 网关 TCP 端口映射（默认关闭，失败不影响启动）。
- Unsigned `addrs` on `hello`/`welcome`; `/connect` retries saved addresses.
  握手里未签名的 `addrs`；`/connect` 会依次尝试已保存地址。
- `/version`, `p2pirc --version`, `Makefile`, GitHub Actions CI, this changelog.
  `/version`、`--version`、Makefile、CI 与本变更日志。

### Changed / 变更

- Local LAN IPv4 listen addresses are advertised to peers as TCP hints.
  本机局域网 IPv4 监听地址会作为 TCP 提示发给对端。

### Security / 安全（如实）

- STUN reports a **UDP** mapped address. It is **not** a TCP hole punch and
  does not make the listen port reachable from the public Internet.
  STUN 返回的是 **UDP** 映射，不是 TCP 打洞，也不能让公网连上监听端口。
- Inbound from the Internet still needs a forwarded TCP port, a working UPnP
  mapping, or a future 0.5 relay.
  公网入站仍然需要 TCP 端口转发、成功的 UPnP，或以后的 0.5 中继。

## [0.3.0] - 2026-08-22

Signed LAN discovery, `--auto-connect`, `--reconnect`, local aliases, `peers.json`.
签名局域网发现、自动连接、重连、本地别名。

## [0.2.0] - 2026-08-22

TLS 1.3 mutual Ed25519 certificates, signed chat/join/leave, `/whoami`, bilingual README.
TLS 1.3 双向 Ed25519 证书、消息签名、`/whoami`、中英 README。

## [0.1.0] - 2026-08-22

LAN TCP gossip chat, Ed25519 identity, NDJSON protocol.
局域网 TCP gossip 聊天、Ed25519 身份、NDJSON 协议。
