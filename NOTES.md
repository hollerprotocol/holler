# Implementation notes for SPEC.md draft 1

These notes come from building holler twice, independently. The Go reference daemon is in this repository. The single-file Python peer (`python/holler_peer.py`) was written from SPEC.md alone, without looking at the Go code. The two interoperate (`python/interop_test.py`). The places where either implementation had to guess are listed below, with the choice made and a proposed spec change.

## Decisions on the open questions

| question | decision |
|----------|----------|
| 15, Q1: daemon or per session | **Daemon.** One per holler home (`~/.holler`), started on demand by any command, the MCP server or a hook. It owns the key, the tailcat listener, every connection and the store. Switching harness mid-task keeps the identity and the conversations, and the awake side keeps retrying with no session running. |
| 15, Q2: getting inbound messages to the model | Four ways, in order of preference: (1) Claude Code hooks: SessionStart, UserPromptSubmit, PostToolUse and Stop (`holler hook ...`); (2) blocking waits the model calls (`holler wait`, MCP `holler_read` with `wait_seconds`, `until_state`); (3) `holler tail --once` at natural checkpoints; (4) MCP push through Claude Code *channels*. Channels are opt-in (`HOLLER_CHANNEL=1` plus `claude --channels ...`). Claude Code sends the same client capabilities whether or not channels are on (verified empirically), so a server cannot detect them. |
| 15, Q3: ship tailcat or require it on PATH | **Embedded** as a Go library (`github.com/tailscale/tailcat`), so there is no second binary. holler listens on tunnel port 1, the port tailcat's pipe mode dials. That means `tailcat <address>` reaches a holler peer, and you can read its `hello` and type NDJSON at it. Verified. |
| 16, Q2: outbox retention | 7 days, configurable (`outbox_ttl`). |
| 16, Q3: in-band blobs | Kept in band, as drafted. Chunks are sent *before* the msg that references them (see "chunks" below). |
| 16, Q4: multi-party | Not implemented. Connections are pairwise. |
| 16, Q5: grant delegation | One level, as drafted. It is attenuated: a delegated grant confers at most what the introducer holds, minus `introduce`. Introduction grants are audience-bound (see "grants" below). |
| 16, Q6: exec and fs shapes | A companion profile: [PROFILE.md](PROFILE.md). |
| 16, Q7: multiple keys | Not implemented. |
| 16, Q8: language | Go for the reference, plus the Python peer. Full resume, blobs and grants took about 2,500 lines of Python. A peer without persistence would be the few-hundred-line afternoon job the spec promises. |

## Protocol issues found

Each item gives the issue, what the reference does, and a proposed change.

1. **Chunks versus acks, threads and the outbox.** Chunks carry no `th` in the envelope table and are never acked, yet 12.1 replays a chunk from the outbox. Replay is per thread, and a sender MUST keep a message until it is acked. As written, a chunk is either never replayed or never removed from the outbox.
   - *Reference:* chunks carry `th`, which the envelope allows. Chunks go out before the msg that names the blob. When that msg is acked, the sender drops the earlier never-acked lines (chunk, introduce) in the same thread from its outbox. That is safe because delivery within a thread is in order. The peer's `seen` also drops everything up to the seen id.
   - *Proposal:* make chunk `th` a SHOULD, and state the cumulative-ack rule.
2. **What counts as seen.** In 12.1, B reports `"seen":{"t1":"a9"}`, where `a9` is an *ack*.
   - *Problem:* if acks count as seen, an ack that overtakes an older queued msg in the same thread makes the sender skip that msg on replay, and it is lost.
   - *Reference:* seen counts only content lines: msg, state, chunk and introduce.
   - *Proposal:* say so, and fix the example.
3. **Ids must increase in send order.** Resume compares ids, so a sender's ids MUST be strictly increasing in send order: within a millisecond, across restarts, and even if the clock steps back after a snapshot restore. The outbox order must also match id order. The reference takes ids inside the transaction that enqueues the message. "ULID or UUIDv7 recommended" does not guarantee any of this.
   - *Proposal:* MUST be strictly increasing per sender, compared as byte strings.
4. **One active connection per peer.** Dedup and resume assume a single ordered stream. Two live connections (a reconnect race, or both sides dialing) can deliver a later id before an earlier one. The receiver's seen then jumps ahead, and the earlier message is never replayed.
   - *Reference:* activates one connection at a time and stops reading the old one before the new one's resume. For simultaneous opens, both sides keep the connection dialed by the smaller key.
   - *Proposal:* add this to 4.3.
5. **Reflection in auth.** The auth signature is symmetric in form, so an attacker can echo your hello back to you, presenting your own key, and then replay your own auth signature. It verifies.
   - *Both implementations:* reject a hello carrying their own key.
   - *Proposal:* make that a MUST, or add a role byte to the signed transcript.
6. **Who can reconnect.** "The awake side owns reconnection", but a listener never learns the dialer's address. If the dialer goes away and the listener has results queued, nobody can reconnect: for example, a delegate finishes while the delegator's sandbox sleeps.
   - *Reference:* sends an optional `addr` in hello, the sender's own reachable address, covered by auth. The listener dials back after a grace period, so it does not race the original dialer.
   - *Proposal:* standardize `hello.addr`.
7. **Grants have no audience.** A grant from B to C is honored by B itself (iss is B's own key) and by anyone who trusts B. So an introduction grant that B mints for A to honor also gives C those capabilities on B.
   - *Reference:* adds a signed `aud` field to introduction grants and honors a grant with `aud` only when `aud` is its own key. This is backward compatible: a peer that does not know `aud` honors no more than the spec already says.
   - *Proposal:* add `aud` to the grant object.
   - Also define delegation as attenuating: a delegated grant confers at most the introducer's capabilities, minus `introduce`.
8. **Refusing a key.** 7.3 lets a peer refuse a key, but no code fits a refusal after a *valid* auth, and it arrives right after auth.
   - *Reference:* sends `err auth` with a detail and closes. A dialer treats the connection as established only once the peer's `resume` (or any non-err line) has arrived.
   - *Proposal:* say this, or add a `refused` code.
9. **Small contradictions.**
   - `caps`: the text says `chat` is not listed, but the examples list it. Both implementations send it and ignore it on receipt.
   - `too_large`: 9.7 says close behaviour is stated per code, but it is not stated for `too_large`. Both implementations close.
   - Section 15's subsections are numbered 16.x.
10. **Encodings left open.**
    - *base64:* padding is unspecified for keys, nonces and signatures (base64url), and so is the alphabet for chunk data. Both implementations send unpadded base64url and padded standard base64 respectively, and accept any variant.
    - *Timestamp precision:* unspecified. Both send milliseconds and accept any RFC 3339.
11. **Canonical JSON.** "Keys sorted, no whitespace, UTF-8" leaves string escaping open. Go escapes `<`, `>`, `&` and U+2028 by default, and Python escapes all non-ASCII by default.
    - *Reference:* escapes only the quote, backslash and control characters, using the short forms `\b \f \n \r \t`. That equals Python's `json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)`, checked in a Go test and live between the two implementations.
    - *Proposal:* spell out the escaping, or cite RFC 8785 (JCS).
12. **"Open thread" is undefined** for the reconnect rule.
    - *Reference:* a thread is open until either side reports done, failed or closed, and only if it saw activity within the retention window.
    - *Python peer:* only `closed` ends a thread.
    - This is local policy and does not affect interop, but the spec should say which reading it means.
13. **Plain bindings.** Auth binds only the handshake. Over TCP or a Unix socket nothing after it is tied to the key, so 4.2's advice is load-bearing. The reference refuses plain TCP to public addresses unless `HOLLER_ALLOW_PLAINTEXT=1` is set. Loopback, RFC 1918, IPv6 ULA (which includes Fly's 6PN), link-local and CGNAT addresses are allowed.
14. **Wake-on-connect over tailcat.** A tailcat listener waits on its DERP connection. A frozen sandbox's DERP client cannot wake it, so wake-on-connect needs a platform-routed binding, such as a WebSocket through the sandbox's HTTP URL.
    - Worth a sentence in 4.3.
    - A candidate for a second binding.

## Plugin packaging

- Agent Plugins 1.0.0 puts the MCP config at the plugin root as `mcp.json`, so the gist's `mcp/holler-mcp.json` moved there.
- Claude Code 2.1 reads `.claude-plugin/plugin.json`, not the portable root `plugin.json`, so the plugin ships both.
  - The Codex and Cursor CLIs installed here both reference the agent-plugins.org 1.0.0 schemas.
- Hooks are client-specific, so they live in the reverse-domain extension directory `com.anthropic.claude-code/hooks.json`, referenced from `.claude-plugin/plugin.json`.
- Claude Code puts a plugin's `bin/` on PATH for its shell tool. `bin/holler` is a small launcher that picks `libexec/holler-<os>-<arch>`.
