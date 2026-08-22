# P2P-IRC architecture

This document describes the MVP (version 0.1): a LAN-first, serverless
peer-to-peer chat process. There is no central server process in this
repository.

## Peer model

Every running `p2pirc` binary is a **peer**. A peer has:

- an Ed25519 keypair
- a Peer ID = `SHA-256(raw public key)` (64 hex characters; UI shows 8)
- a TCP listen address (`0.0.0.0:7777` by default)
- a table of live TCP connections
- a set of joined channels
- an in-memory history per joined channel
- a bounded set of seen message IDs

Nicknames are labels. They are never used as identity.

```
        Peer A                 Peer B                 Peer C
     +----------+           +----------+           +----------+
     | UI/chat  |           | UI/chat  |           | UI/chat  |
     | router   |           | router   |           | router   |
     | peers    |<---TCP--->| peers    |<---TCP--->| peers    |
     +----------+           +----------+           +----------+
```

A, B, and C form a mesh. Chat from A reaches C through B via gossip.

## Identity model

Keys live in `<data-dir>/identity.json` (default `~/.p2pirc/`). The file is
created on first run and reused thereafter.

The private key is stored as hex. File mode is `0600`. This is **not** a
hardware-backed keystore. See the README security section.

Handshake proof:

1. Remote sends `public_key` and `peer_id`
2. Local computes `SHA-256(decode(public_key))`
3. Connection is rejected if that digest is not exactly `peer_id`

## Connection lifecycle

```
dial / accept
     |
     v
  handshake (hello -> welcome)
     |
     v
  verify public key <-> peer id
     |
     v
  register in peer manager
  (drop duplicate if the other session is preferred)
     |
     v
  read loop  +  write loop  +  ping ticker
     |
     v
  error / idle timeout / /disconnect / process exit
     |
     v
  cancel context, close socket, unregister
```

Writes never share the TCP connection with another goroutine: all outbound
messages go through a buffered `sendCh` consumed by one write loop.

The application uses `context.Context` from `signal.NotifyContext`. On
SIGINT/SIGTERM or `/quit`:

1. stop accepting
2. close every peer socket (unblocks read loops)
3. save identity
4. exit

### Duplicate connections

If A dials B and B also dials A, two TCP sessions exist for the same pair.

**Rule:** keep the session **initiated by the lexicographically smaller
full Peer ID**. Close the other.

- If `id(A) < id(B)`, keep A's outbound socket (B's inbound).
- If `id(A) > id(B)`, keep B's outbound socket (A's inbound).

The peer manager compares connection pointers on unregister so closing the
losing session cannot delete the winner from the map.

Self-connections (same Peer ID on both ends) are refused.

## Message routing (gossip)

Routable types: `chat`, `join`, `leave`.

```
receive message M from peer P
        |
        v
   seen(M.id) ? --yes--> drop
        |
       no
        v
   remember M.id
   deliver to local chat/channels
   forward M to every connected peer except P
```

Locally composed messages use the same path with `P` empty (forward to all).

Direct messages (`/msg`) are **not** flooded. They are written only on the
TCP session to that peer.

### Loop prevention

A triangle A—B—C—A cannot amplify a message: the first time a peer sees an
ID it forwards; the second time it ignores. The seen store is a
`map[string]time.Time` with:

- TTL default 24 hours
- hard cap default 50,000 entries (oldest dropped if still full after TTL sweep)

## Channel model

A channel is a string that starts with `#`. Nobody owns it. There is no
registry. Membership is gossiped with `join` / `leave` and is eventually
consistent.

The process auto-joins `#general` locally on startup. When a new TCP peer
comes up, the local node sends `join` for each currently joined channel to
that peer only (so the new neighbor learns membership without re-flooding
the mesh).

## Concurrency model

Expected goroutines:

| Goroutine | Lifetime |
|---|---|
| `main` / UI input | process |
| TCP `acceptLoop` | process |
| seen-cache sweep | process |
| discovery read + announce (optional) | process |
| per-connection read loop | connection |
| per-connection write loop (includes ping) | connection |

Hooks into the peer manager (`onMsg`, `onUp`, `onDown`) run on connection
goroutines and must not block. UI delivery uses a buffered event channel
and drops on overflow rather than stalling the network.

## Failure handling

| Failure | Behavior |
|---|---|
| Malformed JSON line | Log, skip line, keep connection |
| Message larger than 64 KiB | Close that connection |
| Handshake failure | Close socket, do not register |
| Idle (no recv for 60s) | Close connection |
| Write / send-queue full | Close slow peer |
| Discovery bind failure | Warn and continue without `/discover` |
| Peer disconnect | Unregister, print a system line |
| Corrupt `identity.json` | Refuse to start (do not silently mint a new key) |

## Transport seam (for later encryption)

Application logic talks to `net.Conn` only through:

- `protocol.Read(*bufio.Reader)` / `protocol.Write(io.Writer)`
- `peer.Manager.HandshakeAndAdopt(net.Conn, ...)`

A future TLS or Noise wrapper can sit under `HandshakeAndAdopt` without
changing channels, gossip, or the UI:

```
Plain TCP  ->  Secure Transport  ->  NDJSON protocol  ->  router / chat
```

The MVP does not implement that wrapper. Transport is plaintext TCP.

## LAN discovery

Optional UDP multicast to `239.255.77.77:7776`. Announcements carry peer
ID, nickname, and TCP port. They are not a trust signal; `/connect` still
performs the TCP handshake.
