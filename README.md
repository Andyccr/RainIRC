# P2P-IRC

A minimal, serverless, peer-to-peer IRC-style chat for the local network.

Every running process is both a chat client and a P2P node. There is **no central IRC server**, no cloud backend, and no account service. Two laptops on the same LAN can talk after one `/connect`.

```
SIMPLE > FEATURE-RICH
P2P > CENTRAL SERVER
LOCAL-FIRST > CLOUD-FIRST
```

This is **not** a full IRC RFC implementation. It is a small Go program with an IRC-like channel model over direct TCP.

## Why serverless?

Classic IRC needs a server that every client trusts. That server is a single point of failure, a place to log conversations, and something you must host.

P2P-IRC replaces the server with gossip:

- You listen on TCP port `7777` (configurable).
- You connect directly to another peer's IP and port.
- Chat messages are flooded to your connected peers, who forward them onward.
- A bounded seen-ID cache stops loops.

If the Internet is down, LAN chat still works.

## Architecture

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
|  TCP Connections         |
+--------------------------+
```

Each peer has an Ed25519 identity. The Peer ID is `SHA-256(public key)`, shown as the first 8 hex characters (for example `7f3a91c2`). Nicknames are cosmetic only.

Details: [docs/architecture.md](docs/architecture.md)  
Wire format: [docs/protocol.md](docs/protocol.md)

## Requirements

- Go 1.22 or newer
- A terminal
- TCP connectivity between peers (LAN for this MVP)

No Docker, database, Redis, or extra runtime.

## Installation

```bash
git clone https://github.com/Andyccr/RainIRC.git
cd RainIRC
go build -o p2pirc ./cmd/p2pirc
```

## Running

```bash
./p2pirc --port 7777 --nickname Alice
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `--port` | `7777` | TCP listen port (`0` = ephemeral) |
| `--nickname` | first 8 chars of Peer ID | Cosmetic nick |
| `--data-dir` | `~/.p2pirc` | Identity directory |
| `--debug` | off | Debug logs on stderr |
| `--no-discover` | off | Disable LAN UDP multicast |

Identity is stored in `~/.p2pirc/identity.json` (or `%USERPROFILE%\.p2pirc\` on Windows) and reused across restarts.

## Connecting two peers

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

On the same computer, use two ports:

```bash
./p2pirc --port 7777 --nickname Alice
./p2pirc --port 7778 --nickname Bob --data-dir /tmp/bob
```

```
/connect 127.0.0.1:7777
```

Each instance needs its own `--data-dir` if they share an account, otherwise they load the same key and refuse to connect to themselves.

Type a line of text to send it to the current channel (`#general` by default).

## Commands

```
/help                      Show commands
/connect <host:port>       Connect to a peer
/disconnect <peer-id>      Disconnect a peer
/peers                     List connected peers
/join <#channel>           Join a channel
/leave <#channel>          Leave a channel
/channels                  List channels
/nick <name>               Set nickname (cosmetic)
/me <action>               Send an action
/msg <peer-id> <text>      Direct message a connected peer
/discover                  Show nearby LAN peers
/quit                      Exit
```

Channel names start with `#`. `/join dev` is treated as `#dev`.

## Protocol

Newline-delimited JSON on TCP. Handshake is `hello` then `welcome`. Chat, join, and leave are gossiped. Ping/pong keep connections alive.

See [docs/protocol.md](docs/protocol.md) for the full schema.

## LAN discovery

Peers periodically announce themselves on UDP multicast `239.255.77.77:7776`. `/discover` lists recently seen neighbors. The MVP **does not** auto-connect; you still `/connect` the address you want.

Discovery is best-effort. If multicast is blocked, `/connect` still works.

## Security limitations

**MVP transport is not encrypted.** Anyone on the path can read and modify messages.

What the MVP *does* provide:

1. Cryptographic peer identity (Ed25519)
2. Handshake verification that `peer_id == SHA-256(public_key)`
3. Input validation and a 64 KiB message size limit
4. No remote shell / no code execution from chat

What it does **not** provide:

- TLS / Noise / end-to-end encryption
- Message signatures
- Authentication of chat contents
- Protection against a peer who floods or lies about nicknames

The private key is stored **in plaintext hex** in `identity.json` with mode `0600`. Anyone who can read that file can impersonate you. Do not copy it to an untrusted machine.

Future versions can wrap the existing TCP session with TLS or Noise without changing the chat/channel model. The codec already reads and writes `io.Reader` / `io.Writer`.

## Current limitations

- Designed for a small LAN mesh, not the public Internet
- NAT traversal is not implemented
- Message history is in-memory only
- Duplicate-message cache is bounded (default 24 h / 50k IDs) then forgets
- Channel membership is eventually consistent (no consensus)
- Terminal UI is line-based, not a full TUI
- Direct messages are not encrypted and only travel to a directly connected peer

## Testing

```bash
go test ./...
```

Tests bind to `127.0.0.1` with ephemeral ports and temporary directories. They do not need Internet access.

```bash
gofmt -w .
go vet ./...
```

## Roadmap

| Version | Focus |
|---|---|
| 0.1 | LAN TCP gossip chat (this MVP) |
| 0.2 | TLS / Noise transport, message signatures |
| 0.3 | Richer LAN discovery, peer aliases |
| 0.4 | NAT traversal, STUN, UDP hole punching |
| 0.5 | Optional relays for Internet-wide P2P |
| 0.6 | Encrypted persistent history |
| 0.7 | File transfer |
| 0.8+ | Voice, mobile, stable protocol |

## License

GNU Affero General Public License v3.0. See [LICENSE](LICENSE).
