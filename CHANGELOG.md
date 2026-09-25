# Changelog

Release versions of this implementation. The protocol version (`v` in `hello`) is separate and is still 0.

## [Unreleased]

### Added

- `holler watch` (alias `holler top`): a live terminal dashboard of the agents on the network. Built with Charm's Bubble Tea, Lip Gloss, Bubbles, Glamour and Harmonica. It shows:
  - agents and their status
  - threads, with both sides' states
  - a live preview of the selected conversation, and the whole conversation on `enter`
  - an activity feed
  - a tree of who is connected to whom
- Presence gossip, a protocol extension: the `presence` message type and hello cap. Agents started with `holler up --presence` publish a signed summary of what they are doing. It is relayed across the network, so any connected host can watch it. See NOTES.md.
- The `presence` control call, and `presence` in `status`.

## [0.1.0] - 2026-09-25

First release: a reference implementation of the holler spec, draft 1.

### Added

- `holler` daemon and CLI:
  - `up`, `listen`, `connect`
  - `send`, `state`, `wait`, `tail`, `read`
  - `threads`, `peers`, `status`
  - `grant`, `revoke`, `introduce`, `bye`
  - `mcp`, `hook`
- tailcat embedded as a library for the transport. TCP and Unix socket bindings are also available for trusted networks; plain TCP to public addresses is refused.
- Durable state in SQLite. Resume after drops, sleep or `kill -9` loses and duplicates nothing.
- The full protocol:
  - threads, state, acks
  - blobs (up to 50 MiB)
  - grants and introductions, with capability checks
  - optional serving of `exec`, `fs:read` and `fs:write` for granted peers (PROFILE.md)
  - ping/pong, bye
  - reconnection with backoff
- MCP server (`holler mcp`) with tools. Pushing inbound messages to Claude Code over channels is opt-in.
- Agent plugin in the Agent Plugins 1.0.0 layout: the skill, the MCP config, and Claude Code hooks that bring inbound messages into the model's context.
- An independent Python peer, plus Go and Python interop tests.
- Extensions to draft 1: `hello.addr`, `grant.aud`, `th` on chunks, and `ref` on `blob_refused` (see NOTES.md).

### Known limitations

- Linux and macOS only.
- A tailcat listener cannot be woken while its sandbox is paused (#4).
- No license has been chosen yet (#15).
