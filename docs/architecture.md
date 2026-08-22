# P2P-IRC architecture

This document describes version **0.4.1**: a LAN-first, serverless peer-to-peer
chat process with optional STUN/UPnP **address hints**, bounded mesh degree,
and a timestamp window on signed frames. There is no central server process
in this repository. STUN is not a TCP NAT traversal.

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
     | peers    |<-TLS/TCP->| peers    |<-TLS/TCP->| peers    |
     +----------+           +----------+           +----------+
```

A, B, and C form a mesh. Chat from A reaches C through B via gossip.

## Identity model

Keys live in `<data-dir>/identity.json` (default `~/.p2pirc/`). The file is
created on first run and reused thereafter.

The private key is stored as hex. File mode is `0600`. This is **not** a
hardware-backed keystore. See the README security section.

Handshake proof:

1. TLS 1.3 mutual handshake with self-signed Ed25519 certificates
2. Remote sends signed `public_key` and `peer_id` in hello/welcome
3. Local checks certificate pubkey == hello pubkey
4. Local computes `SHA-256(decode(public_key))`
5. Connection is rejected if that digest is not exactly `peer_id`
6. Signature is rejected if it does not verify under that key

Chat/join/leave frames are also signed, so a two-hop message from a peer you
never dialed is still attributable.

## Connection lifecycle

```
dial / accept
     |
     v
  TLS 1.3 (mutual Ed25519 certs)   [--plain skips this]
     |
     v
  handshake (signed hello -> signed welcome)
     |
     v
  verify cert pubkey == hello pubkey == peer id
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
| STUN Binding + optional UPnP (fail-soft) | startup |
| per-connection read loop | connection |
| per-connection write loop (includes ping) | connection |

Hooks into the peer manager (`onMsg`, `onUp`, `onDown`) run on connection
goroutines and must not block. UI delivery uses a buffered event channel
and drops on overflow rather than stalling the network.

## Failure handling

| Failure | Behavior |
|---|---|
| Malformed JSON line | Skip; disconnect after 5 consecutive |
| Message larger than 64 KiB | Close that connection |
| Handshake failure | Close socket, do not register |
| Idle (no recv for 60s) | Close connection |
| Write / send-queue full | Close slow peer |
| Discovery bind failure | Warn and continue without `/discover` |
| Discovery UDP read error | Log, keep listening |
| STUN Binding failure | Debug log; `/addr` shows no mapping |
| UPnP mapping failure | Debug log; continue without a public TCP mapping |
| Malformed JSON after handshake | Skip line; disconnect after 5 consecutive |
| Handshake slots / max peers full | Close the new socket |
| Signed frame outside clock window | Drop, do not forward |
| Peer disconnect | Unregister, drop channel membership, print a system line |
| Corrupt `identity.json` | Refuse to start (do not silently mint a new key) |

## Transport

```
TCP  ->  TLS 1.3 (mutual Ed25519)  ->  NDJSON protocol  ->  router / chat
```

`--plain` removes the TLS box. Application logic still talks to `net.Conn`
through `protocol.Read` / `protocol.Write` and `HandshakeAndAdopt`, so a
future Noise wrapper can replace TLS without touching channels or the UI.

TLS here is **not** WebPKI. Certificates are generated from the same Ed25519
identity used for Peer IDs. Hostname checks are skipped; identity is the key.

## LAN discovery and aliases

Optional UDP multicast to `239.255.77.77:7776`. Version-2 announcements are
Ed25519-signed (same identity as TLS). Unsigned packets are kept for display
as `unverified` and are never used by `--auto-connect`.

`--auto-connect` dials verified neighbors. `--reconnect` dials last-seen
addresses from `~/.p2pirc/peers.json`. `/alias` stores a **local** label
for a Peer ID; `/connect laptop` resolves through that directory. Aliases
never travel on the wire and never replace cryptographic identity.

`hello`/`welcome` may include an unsigned `port` hint so an inbound peer
can be reconnection-addressed (remote host + advertised listen port).

They may also include unsigned `addrs` (host:port list). Those are TCP
candidates collected locally: private IPv4 listen addresses, plus a UPnP
external mapping when `--upnp` succeeds. They are **not** covered by
`SignBytes` (same as `port`). Receivers sanitize and cap the list.
Reconnect persistence uses the TCP session address (and listen `port` on
inbound), not the unsigned `addrs` list.

When a duplicate TCP session is replaced, the keeper re-sends `join` for
every locally joined channel so membership does not stall on the old socket.

Live mesh degree defaults to 64 (`--max-peers`). In-flight inbound
handshakes default to 32. `peers.json` is written on a 3-second debounce
with a single save lock.

## Addressing, STUN, and UPnP

On listen, the node records:

1. Loopback `127.0.0.1:<port>` (local only; not advertised)
2. Non-loopback IPv4 `host:port` on up interfaces (advertised)
3. Optional STUN XOR-MAPPED-ADDRESS from `--stun` (default
   `stun.l.google.com:19302`, disable with `--no-stun` or empty `--stun`)
4. Optional UPnP `AddPortMapping` when `--upnp` is set

STUN here is a UDP Binding client. The mapped port is the UDP source port of
that request, **not** the TCP listen port. Advertising it as a TCP dial
target would be a lie, so it is shown by `/addr` / `/whoami` and is **not**
placed on hello `addrs`.

UPnP, when it works, maps the TCP listen port through the IGD. That address
**is** advertised. Many networks have no IGD or block UPnP; failure must not
stop the node.

`/connect alias` tries `last_addr` then saved extra `addrs` from
`peers.json`. Full UDP hole punching still needs a rendezvous (roadmap 0.5).
