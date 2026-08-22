# P2P-IRC

[中文](#中文) · [English](#english)

Serverless, local-first, peer-to-peer IRC-style chat.  
无中心服务器的局域网 P2P 聊天。

---

## 中文

P2P-IRC 是一个**最小、无服务器、点对点**的 IRC 风格终端聊天程序。

每一个运行中的进程同时是：

1. 聊天客户端
2. P2P 节点

**没有**中心 IRC 服务器、云后端、账号系统。同一局域网里两台电脑，一条 `/connect` 就能说话。

```
简单 > 功能堆砌
P2P > 中心服务器
本地优先 > 云优先
```

这**不是**完整 IRC RFC 实现。它只是一个小的 Go 程序：IRC 式频道 + 直连 TCP/TLS。

当前版本 **0.3**：TLS 1.3、消息签名、**签名局域网发现**、**可选自动连接**、**本地别名**。

### 为什么要无服务器？

传统 IRC 需要一台所有人信任的服务器。它是单点故障，也是日志和审查发生的地方。

P2P-IRC 用 gossip 取代服务器：

- 在 TCP `7777` 端口监听（可改）
- 直接连到对方的 IP:端口（默认先做 TLS 1.3）
- 聊天消息洪泛给邻居，邻居再转发出去
- 有界的「已见消息 ID」缓存阻止死循环
- 频道消息带发送者私钥签名，两跳之外也能验真
- 局域网发现包可验签；`--auto-connect` 只自动连已验证的邻居
- 本地别名（`/alias`）和已知节点地址会记在 `~/.p2pirc/peers.json`

断网时，只要局域网还在，聊天就还在。

### 架构

```
+--------------------------+
|          P2PIRC          |
|                          |
|  终端 UI                 |
|       |                  |
|  聊天 / 频道             |
|       |                  |
|  消息路由 (gossip)       |
|       |                  |
|  对等节点管理            |
|       |                  |
|  TLS 1.3 + TCP           |
+--------------------------+
```

每个节点有 Ed25519 身份。Peer ID = `SHA-256(公钥)`，界面显示前 8 位十六进制（例如 `7f3a91c2`）。昵称只是cosmetic，不是身份。

细节：[docs/architecture.md](docs/architecture.md)  
协议：[docs/protocol.md](docs/protocol.md)

### 要求

- Go 1.22 或更新
- 终端
- 节点之间的 TCP 连通（本版本针对局域网）

不需要 Docker、数据库、Redis 或其它运行时。

### 安装

```bash
git clone https://github.com/Andyccr/RainIRC.git
cd RainIRC
go build -o p2pirc ./cmd/p2pirc
```

### 运行

```bash
./p2pirc --port 7777 --nickname Alice
```

| 参数 | 默认 | 含义 |
|---|---|---|
| `--port` | `7777` | TCP 监听端口（`0` = 系统分配） |
| `--nickname` | Peer ID 前 8 位 | 显示用昵称 |
| `--data-dir` | `~/.p2pirc` | 身份目录 |
| `--debug` | 关 | stderr 调试日志 |
| `--no-discover` | 关 | 关闭局域网 UDP 组播发现 |
| `--plain` | 关 | **关闭 TLS**（不安全，仅调试） |
| `--auto-connect` | 关 | 自动连接**已验签**的局域网邻居 |
| `--reconnect` | 关 | 启动时重连 `peers.json` 里上次的地址 |

身份存在 `~/.p2pirc/identity.json`（Windows 为 `%USERPROFILE%\.p2pirc\`），重启后复用。已知节点和别名在 `peers.json`。输入 `/whoami` 可查看完整 Peer ID、指纹和公钥。

### 两台机器互连

机器 A（`192.168.1.10`）：

```bash
./p2pirc --port 7777 --nickname Alice
```

机器 B：

```bash
./p2pirc --port 7777 --nickname Bob
```

然后在 B 上：

```
/connect 192.168.1.10:7777
```

同一台电脑用两个端口、两个数据目录：

```bash
./p2pirc --port 7777 --nickname Alice
./p2pirc --port 7778 --nickname Bob --data-dir /tmp/bob
```

```
/connect 127.0.0.1:7777
```

同一用户目录会加载同一把密钥，节点会拒绝连自己。

默认频道是 `#general`。普通输入会发到当前频道。

### 命令

```
/help                      显示帮助
/connect <host:port|别名>  按地址或本地别名连接
/disconnect <peer-id>      断开一个节点
/peers                     列出已连接节点
/known                     列出已知节点与别名
/alias [peer-id] [name]    查看或设置本地别名
/unalias <name|peer-id>    删除别名
/join <#channel>           加入频道
/leave <#channel>          离开频道
/channels                  列出频道
/nick <name>               设置昵称（仅显示）
/me <action>               发送动作
/msg <peer-id> <text>      给直连节点发私信
/discover [connect]        显示附近节点；connect 会连上已验签的邻居
/whoami                    显示本机密码学身份
/quit                      退出
```

频道名以 `#` 开头。`/join dev` 等于 `#dev`。给对端起名后可以用 `/connect laptop` 再连回去。

### 协议

TCP 上先做 **TLS 1.3**（ALPN `p2pirc/2`），再跑换行分隔 JSON。握手是带签名的 `hello` → `welcome`。`chat` / `join` / `leave` 会 gossip，并带 Ed25519 签名。Ping/pong 维持连接。

完整字段见 [docs/protocol.md](docs/protocol.md)。

### 局域网发现

节点会向 UDP 组播 `239.255.77.77:7776` 宣告自己，公告带 Ed25519 签名。`/discover` 会标明 `verified` / `unverified`。

默认**不会**自动连接。打开自动连接：

```bash
./p2pirc --nickname Alice --auto-connect
```

只会对**验签通过**的邻居发起 TCP/TLS。`/discover connect` 是一次性手动版。`--reconnect` 会在启动时尝试 `peers.json` 里记下的上次地址。

组播被墙时，手动 `/connect` 仍然可用。未签名的发现包只会显示，不会被 auto-connect。

### 安全（如实说明）

0.3 提供：

1. 链路：**TLS 1.3 双向证书**（证书由本机 Ed25519 身份自签，无 CA）
2. 消息：可 gossip 的帧带 **Ed25519 签名**，`sender` 必须等于 `SHA-256(公钥)`
3. 握手：TLS 证书公钥必须与 hello/welcome 公钥一致
4. 输入校验与 64 KiB 消息上限
5. 远程内容不会变成 shell / 代码执行
6. 局域网发现包可验签（未签名的仍显示为 unverified）
7. 别名只存在本机，不能冒充 Peer ID

0.3 **仍然不是**：

- 面向互联网的匿名聊天（没有 NAT 穿透）
- 完美前向保密的 Noise 配置（TLS 1.3 有自己的握手；长期身份密钥也用于 TLS 证书）
- 对私钥文件失窃的防护：`identity.json` 仍是 **明文 hex**，权限 `0600`
- 发现包加密；`/msg` 只发到直连节点

`--plain` 会关掉 TLS。不要在不可信网络上使用。

谁能读到 `identity.json`，谁就能冒充你。不要把该文件拷到不信任的机器。

### 当前限制

- 面向小型局域网 mesh，不是公网
- 没有 NAT 穿透 / 中继
- 历史只存在内存
- 已见-ID 缓存有上限（默认 24 小时 / 5 万条）
- 频道成员是最终一致，没有共识
- 终端是按行 UI，不是全屏 TUI
- UDP 发现未签名的包仍可能出现（显示为 unverified，不会 auto-connect）

### 测试

```bash
go test ./...
gofmt -w .
go vet ./...
```

测试绑定 `127.0.0.1`、临时端口和临时目录，不需要互联网。

### 路线图

| 版本 | 重点 |
|---|---|
| 0.1 | 局域网 TCP gossip 聊天 |
| 0.2 | TLS 1.3 + 消息签名 + `/whoami` |
| 0.3 | 签名发现、自动连接、本地别名（当前） |
| 0.4 | NAT 穿透、STUN、UDP 打洞 |
| 0.5 | 可选中继，跨互联网 P2P |
| 0.6 | 加密持久化历史 |
| 0.7 | 文件传输 |
| 0.8+ | 语音、移动端、稳定协议 |

### 许可证

GNU Affero General Public License v3.0。见 [LICENSE](LICENSE)。

---

## English

A minimal, serverless, peer-to-peer IRC-style chat for the local network.

Every running process is both a chat client and a P2P node. There is **no central IRC server**, no cloud backend, and no account service. Two laptops on the same LAN can talk after one `/connect`.

```
SIMPLE > FEATURE-RICH
P2P > CENTRAL SERVER
LOCAL-FIRST > CLOUD-FIRST
```

This is **not** a full IRC RFC implementation. It is a small Go program with an IRC-like channel model over direct TCP/TLS.

Current version **0.3**: TLS 1.3, signed messages, **signed LAN discovery**, **optional auto-connect**, and **local peer aliases**.

### Why serverless?

Classic IRC needs a server that every client trusts. That server is a single point of failure, a place to log conversations, and something you must host.

P2P-IRC replaces the server with gossip:

- You listen on TCP port `7777` (configurable).
- You connect directly to another peer's IP and port (TLS 1.3 first).
- Chat messages are flooded to your connected peers, who forward them onward.
- A bounded seen-ID cache stops loops.
- Channel messages are signed, so a two-hop frame is still attributable.
- LAN discovery packets can be verified; `--auto-connect` only dials verified neighbors.
- Local aliases (`/alias`) and last-seen addresses are stored in `~/.p2pirc/peers.json`.

If the Internet is down, LAN chat still works.

### Architecture

```
+--------------------------+
|          P2PIRC          |
|                          |
|  Terminal UI             |
|       |                  |
|  Chat / Channels         |
|       |                  |
|  Message Router          |
|       |                  |
|  Peer Manager            |
|       |                  |
|  TLS 1.3 + TCP           |
+--------------------------+
```

Each peer has an Ed25519 identity. The Peer ID is `SHA-256(public key)`, shown as the first 8 hex characters (for example `7f3a91c2`). Nicknames are cosmetic only.

Details: [docs/architecture.md](docs/architecture.md)  
Wire format: [docs/protocol.md](docs/protocol.md)

### Requirements

- Go 1.22 or newer
- A terminal
- TCP connectivity between peers (LAN for this version)

No Docker, database, Redis, or extra runtime.

### Installation

```bash
git clone https://github.com/Andyccr/RainIRC.git
cd RainIRC
go build -o p2pirc ./cmd/p2pirc
```

### Running

```bash
./p2pirc --port 7777 --nickname Alice
```

| Flag | Default | Meaning |
|---|---|---|
| `--port` | `7777` | TCP listen port (`0` = ephemeral) |
| `--nickname` | first 8 chars of Peer ID | Cosmetic nick |
| `--data-dir` | `~/.p2pirc` | Identity directory |
| `--debug` | off | Debug logs on stderr |
| `--no-discover` | off | Disable LAN UDP multicast discovery |
| `--plain` | off | **Disable TLS** (insecure; debug only) |
| `--auto-connect` | off | Automatically connect to **verified** LAN peers |
| `--reconnect` | off | Reconnect to last-known addresses in `peers.json` on start |

Identity is stored in `~/.p2pirc/identity.json` (or `%USERPROFILE%\.p2pirc\` on Windows) and reused across restarts. Known peers and aliases live in `peers.json`. `/whoami` prints the full Peer ID, fingerprint, and public key.

### Connecting two peers

Machine A (`192.168.1.10`):

```bash
./p2pirc --port 7777 --nickname Alice
```

Machine B:

```bash
./p2pirc --port 7777 --nickname Bob
```

Then on B:

```
/connect 192.168.1.10:7777
```

On the same computer, use two ports and two data dirs:

```bash
./p2pirc --port 7777 --nickname Alice
./p2pirc --port 7778 --nickname Bob --data-dir /tmp/bob
```

```
/connect 127.0.0.1:7777
```

Each instance needs its own `--data-dir` if they share an account, otherwise they load the same key and refuse to connect to themselves.

Type a line of text to send it to the current channel (`#general` by default).

### Commands

```
/help                      Show commands
/connect <host:port|alias> Connect by address or local alias
/disconnect <peer-id>      Disconnect a peer
/peers                     List connected peers
/known                     List known peers and aliases
/alias [peer-id] [name]    List or set a local alias
/unalias <name|peer-id>    Remove a local alias
/join <#channel>           Join a channel
/leave <#channel>          Leave a channel
/channels                  List channels
/nick <name>               Set nickname (cosmetic)
/me <action>               Send an action
/msg <peer-id> <text>      Direct message a connected peer
/discover [connect]        Show nearby LAN peers; connect dials verified ones
/whoami                    Show local cryptographic identity
/quit                      Exit
```

Channel names start with `#`. `/join dev` is treated as `#dev`. After `/alias 7f3a91c2 laptop` you can `/connect laptop`.

### Protocol

TLS 1.3 (ALPN `p2pirc/2`) wraps TCP, then newline-delimited JSON. Handshake is signed `hello` then `welcome`. Chat, join, and leave are gossiped and Ed25519-signed. Ping/pong keep connections alive.

See [docs/protocol.md](docs/protocol.md) for the full schema.

### LAN discovery

Peers announce themselves on UDP multicast `239.255.77.77:7776` with an Ed25519 signature. `/discover` marks each neighbor `verified` or `unverified`.

Auto-connect is **off** by default:

```bash
./p2pirc --nickname Alice --auto-connect
```

Only **verified** announcements trigger a TCP/TLS dial. `/discover connect` is the one-shot version. `--reconnect` retries last-seen addresses from `peers.json` at startup.

If multicast is blocked, `/connect` still works. Unsigned discovery packets are shown but never auto-connected.

### Security (honest)

Version 0.3 provides:

1. Link: **mutual TLS 1.3** with self-signed Ed25519 certificates (no CA)
2. Messages: gossip frames carry an **Ed25519 signature**; `sender` must be `SHA-256(public key)`
3. Handshake: TLS certificate public key must match hello/welcome
4. Input validation and a 64 KiB message size limit
5. No remote shell / no code execution from chat
6. LAN discovery packets can be signed (unsigned ones show as unverified)
7. Aliases are local-only and never replace the Peer ID

It does **not** provide:

- Internet-wide anonymous chat (no NAT traversal)
- A Noise-protocol identity-hiding handshake
- Protection if `identity.json` is stolen: the private key is still **plaintext hex**, mode `0600`
- Internet-wide routing; `/msg` only reaches a directly connected peer

`--plain` turns TLS off. Do not use it on an untrusted network.

Anyone who can read `identity.json` can impersonate you. Do not copy it to an untrusted machine.

### Current limitations

- Designed for a small LAN mesh, not the public Internet
- NAT traversal is not implemented
- Message history is in-memory only
- Duplicate-message cache is bounded (default 24 h / 50k IDs)
- Channel membership is eventually consistent (no consensus)
- Terminal UI is line-based, not a full TUI
- UDP discovery that is unsigned still appears (as unverified; never auto-connected)

### Testing

```bash
go test ./...
gofmt -w .
go vet ./...
```

Tests bind to `127.0.0.1` with ephemeral ports and temporary directories. They do not need Internet access.

### Roadmap

| Version | Focus |
|---|---|
| 0.1 | LAN TCP gossip chat |
| 0.2 | TLS 1.3, message signatures, `/whoami` |
| 0.3 | Signed discovery, auto-connect, local aliases (current) |
| 0.4 | NAT traversal, STUN, UDP hole punching |
| 0.5 | Optional relays for Internet-wide P2P |
| 0.6 | Encrypted persistent history |
| 0.7 | File transfer |
| 0.8+ | Voice, mobile, stable protocol |

### License

GNU Affero General Public License v3.0. See [LICENSE](LICENSE).
