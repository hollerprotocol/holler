# Changelog

Release versions of this implementation. The protocol version (`v` in `hello`) is separate and is still 0.

## [Unreleased]

### Added

- `install.sh`:
  - installs the latest (or a pinned) release for this OS and CPU, checking `SHA256SUMS`
  - downloads with `gh`, or with `curl` plus `GITHUB_TOKEN` for the private repository
  - then offers to run `holler bootstrap`
- `holler bootstrap`:
  - detects the agent harnesses on the machine: Claude Code, opencode, Codex, Cursor, Gemini CLI, Copilot CLI, grok, pi
  - installs holler into the ones you pick, through an interactive picker or `--all` / `--harness`; `--list`, `--dry-run` and `--uninstall` are also available
  - edits config files in place, keeping key order, and makes a one-time backup of each
- `holler hook --format cursor|gemini|codex`, so hooks work in Cursor and Gemini CLI as well as Claude Code.
- opencode gets a plugin (`~/.config/opencode/plugins/holler.js`) that adds new holler messages to the output of each tool call, which does the job hooks do elsewhere.
- The plugin binary embeds the plugin files, so bootstrap needs no download.

### Changed

- The Claude Code plugin now declares its MCP server in `.mcp.json`. Plugins installed as `~/.claude/skills/holler` only load MCP servers from there.

## [0.1.1] - 2026-09-25

### Fixed

- The daemon failed to start ("bind: invalid argument") when the holler home was deep enough to push the control socket path past the Unix limit, as happens in sandbox scratch directories. Such homes now put the socket in the user's private runtime directory.

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
