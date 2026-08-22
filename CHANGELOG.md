# Changelog

All notable changes to P2P-IRC are documented here.
本文件记录 P2P-IRC 的用户可见变更。

The format follows [Keep a Changelog](https://keepachangelog.com/).
Versions match `internal/version`.

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
