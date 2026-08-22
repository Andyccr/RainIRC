# P2P-IRC protocol (version 2)

Encoding: **newline-delimited JSON** (one object per line, `\n` terminated).
Transport: **TLS 1.3 over TCP** (default port **7777**). `--plain` falls back
to raw TCP for debugging.
Maximum object size: **65536 bytes** (excluding the newline).

This is not RFC 1459 IRC. It is a small custom protocol designed so a second
implementation can interoperate from this page alone.

## Protocol version

Field `version` is an integer. Version **2** implementations **must** send `2`
on `hello` and `welcome` and **must** reject any other handshake version.

Routable messages do not include `version`.

Version 2 vs 1:

- Mutual TLS 1.3 using self-signed Ed25519 certificates (ALPN `p2pirc/2`)
- Ed25519 signatures on `hello`, `welcome`, `chat`, `join`, `leave`

A v1 plaintext peer cannot talk to a v2 TLS peer.

## Transport

```
TCP accept/dial
    -> TLS 1.3 handshake (both sides present an Ed25519 certificate)
    -> NDJSON hello / welcome
    -> verify: cert pubkey == hello/welcome pubkey == SHA-256 Peer ID
    -> read/write loops
```

Certificates are self-signed. There is no CA and no hostname check.
`InsecureSkipVerify` is used only so WebPKI is skipped; the application then
**must** check that the certificate's Ed25519 public key matches the signed
hello/welcome identity.

`--plain` skips the TLS layer. Signatures are still required.

## Common fields

Every message has:

| Field | Type | Required |
|---|---|---|
| `type` | string | yes |
| `id` | string | yes (max 128 chars) |
| `timestamp` | integer (Unix seconds) | yes, non-zero |

`hello`, `welcome`, `chat`, `join`, and `leave` additionally require:

| Field | Type | Required |
|---|---|---|
| `public_key` | 64 hex chars (Ed25519) | yes |
| `signature` | 128 hex chars (Ed25519) | yes |

Message IDs should be unique. This implementation uses

```
hex(SHA-256(sender || timestamp || 16-byte nonce || payload))
```

Receivers treat `id` as an opaque string.

### Signatures

Canonical payload (UTF-8), fields joined by `\n`, **excluding** `signature`:

```
type
id
timestamp          (decimal)
peer_id
sender
public_key
nickname
channel
to
text
action             ("1" or "0")
```

`signature = hex(Ed25519Sign(private_key, payload))`.

Verify:

1. Decode `public_key` as 32 bytes
2. `SHA-256(public_key)` must equal `sender` (or `peer_id` on hello/welcome)
3. `Ed25519Verify(public_key, payload, signature)`

A gossiped message from a peer you have never dialed is still verifiable
because `public_key` travels with the message. Invalid signatures are dropped
and **not** forwarded.

## Handshake

The **dialer** speaks first, after TLS.

```
dialer ---- TLS ClientHello ... ----> listener
dialer <--- TLS ... ------------------ listener
dialer ---- hello   -----------------> listener
dialer <--- welcome ------------------ listener
```

### `hello`

```json
{
  "type": "hello",
  "id": "a1b2c3d4e5f6...",
  "timestamp": 1787390200,
  "version": 2,
  "peer_id": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "public_key": "<64 hex chars, Ed25519 public key>",
  "nickname": "Alice",
  "signature": "<128 hex chars>"
}
```

`peer_id` is 64 lowercase hex characters: `SHA-256(raw 32-byte public key)`.
Do not trust `peer_id` without recomputing it. Do not trust `nickname`.

### `welcome`

Same shape as `hello` with `"type": "welcome"`. It describes the **listener**.

Reject the connection if:

- TLS fails or the peer certificate is not Ed25519
- JSON is malformed
- `version != 2`
- signature is missing or invalid
- TLS certificate public key ≠ hello/welcome public key
- `SHA-256(public_key) != peer_id`
- `peer_id` equals the local Peer ID (self-connect)

## Keepalive

### `ping`

```json
{"type": "ping", "id": "...", "timestamp": 1787390200}
```

### `pong`

```json
{"type": "pong", "id": "...", "timestamp": 1787390200}
```

Recommended: send `ping` every 20 seconds. If no frame is received for 60
seconds, close the socket. `ping`/`pong` are not gossiped and not signed
(they live inside the TLS session).

## Gossip messages

These are delivered locally (if applicable) and forwarded to every
connected peer except the one they arrived from, unless `to` is set.

### `chat`

```json
{
  "type": "chat",
  "id": "abc123...",
  "timestamp": 1787390200,
  "sender": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "public_key": "<64 hex chars>",
  "nickname": "Alice",
  "channel": "#general",
  "text": "hello",
  "signature": "<128 hex chars>"
}
```

Optional `"action": true` means an IRC-style `/me` (`* Alice waves`).

Optional `"to": "<full peer id>"` is a direct message. Direct messages are
**not** flooded; they are sent only on the connection to that peer. `channel`
may be omitted when `to` is present.

`text` is required and at most 4096 Unicode characters. `channel` must match
`#[a-z0-9_-]{1,31}` after lowercasing.

### `join`

```json
{
  "type": "join",
  "id": "...",
  "timestamp": 1787390200,
  "sender": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "public_key": "<64 hex chars>",
  "nickname": "Alice",
  "channel": "#general",
  "signature": "<128 hex chars>"
}
```

### `leave`

Same fields as `join` with `"type": "leave"`.

## Discovery (UDP, optional)

Not part of the TCP/TLS stream. UDP multicast to `239.255.77.77:7776`:

```json
{
  "type": "discovery",
  "peer_id": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "nickname": "Alice",
  "port": 7777
}
```

Announcements are unauthenticated hints. A TLS handshake is still required.

## Size limits and robustness

| Limit | Value |
|---|---|
| JSON object | 64 KiB |
| Chat text | 4096 runes |
| Nickname | 32 runes, no control chars |
| Channel name | 32 bytes including `#` |
| Peer ID | 64 hex chars |

Implementations **must not** allocate unbounded buffers from the network.
A line longer than the cap should abort that connection.

Malformed JSON on an established session should be rejected; this
implementation logs and continues at the next newline. Unknown `type`
values after handshake are ignored (not forwarded). Extra JSON fields are
ignored so the protocol can grow.

## Example session

Peer A (id prefix `7f3a91c2`) listens on `192.168.1.20:7777`.
Peer B (id prefix `91ab72e1`) dials.

```
B <-> A  TLS 1.3 (mutual Ed25519 certificates)
B -> A   hello {version 2, peer_id B, public_key B, nickname Bob, signature}
A -> B   welcome {version 2, peer_id A, public_key A, nickname Alice, signature}
B -> A   join {channel "#general", sender B, signature}
A -> B   join {channel "#general", sender A, signature}
B -> A   chat {channel "#general", sender B, text "hello everyone", signature}
A        displays: [91ab72e1] Bob: hello everyone
```

If a third peer C is connected only to A, A forwards B's signed chat to C
with the same `id`. C verifies B's signature using the `public_key` in the
message. The seen-ID cache prevents loops.
