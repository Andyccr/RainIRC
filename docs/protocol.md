# P2P-IRC protocol (version 1)

Encoding: **newline-delimited JSON** (one object per line, `\n` terminated).
Transport: **TCP**, default port **7777**.
Maximum object size: **65536 bytes** (excluding the newline).

This is not RFC 1459 IRC. It is a small custom protocol designed so a second
implementation can interoperate from this page alone.

## Protocol version

Field `version` is an integer. MVP implementations **must** send `1` on
`hello` and `welcome` and **must** reject any other handshake version.

Routable messages do not include `version`.

## Common fields

Every message has:

| Field | Type | Required |
|---|---|---|
| `type` | string | yes |
| `id` | string | yes (max 128 chars) |
| `timestamp` | integer (Unix seconds) | yes, non-zero |

Message IDs should be unique. This implementation uses

```
hex(SHA-256(sender || timestamp || 16-byte nonce || payload))
```

Receivers treat `id` as an opaque string.

## Handshake

The **dialer** speaks first.

```
dialer ---- hello   ----> listener
dialer <--- welcome ----- listener
```

After both sides have the remote public key and a verified Peer ID, they
start independent read/write loops. Further `hello`/`welcome` frames are
ignored.

### `hello`

```json
{
  "type": "hello",
  "id": "a1b2c3d4e5f6...",
  "timestamp": 1787390200,
  "version": 1,
  "peer_id": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "public_key": "<64 hex chars, Ed25519 public key>",
  "nickname": "Alice"
}
```

`peer_id` is 64 lowercase hex characters: `SHA-256(raw 32-byte public key)`.
Do not trust `peer_id` without recomputing it. Do not trust `nickname`.

### `welcome`

Same shape as `hello` with `"type": "welcome"`. It describes the **listener**.

Reject the connection if:

- JSON is malformed
- `version != 1`
- `public_key` is not 32 bytes after hex-decode
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
seconds, close the socket. `ping`/`pong` are not gossiped.

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
  "nickname": "Alice",
  "channel": "#general",
  "text": "hello"
}
```

Optional `"action": true` means an IRC-style `/me` (`* Alice waves`).

Optional `"to": "<full peer id>"` is a direct message. Direct messages are
**not** flooded; they are sent only on the connection to that peer. `channel`
may be omitted when `to` is present.

`text` is required and at most 4096 Unicode characters. `channel` must match
`#[a-z0-9_-]{1,31}` after lowercasing. A leading `#` is implied if missing
at the command layer; on the wire the `#` is required.

### `join`

```json
{
  "type": "join",
  "id": "...",
  "timestamp": 1787390200,
  "sender": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "nickname": "Alice",
  "channel": "#general"
}
```

### `leave`

Same fields as `join` with `"type": "leave"`.

## Discovery (UDP, optional)

Not part of the TCP stream. UDP multicast to `239.255.77.77:7776`:

```json
{
  "type": "discovery",
  "peer_id": "7f3a91c2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "nickname": "Alice",
  "port": 7777
}
```

Announcements are unauthenticated hints. A TCP handshake is still required.

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
B -> A   hello {version 1, peer_id B, public_key B, nickname Bob}
A -> B   welcome {version 1, peer_id A, public_key A, nickname Alice}
B -> A   join {channel "#general", sender B}
A -> B   join {channel "#general", sender A}
B -> A   chat {channel "#general", sender B, text "hello everyone"}
A        displays: [91ab72e1] Bob: hello everyone
A -> B   chat {channel "#general", sender A, text "welcome"}
```

If a third peer C is connected only to A, A forwards B's chat to C with the
same `id`. C does not send it back to A in a useful way: A already has the
id in its seen cache.
