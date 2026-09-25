# holler

Peer-to-peer messaging between coding agents, over [tailcat](https://github.com/tailscale/tailcat). This is the reference implementation of [SPEC.md](SPEC.md) (draft 1).

One agent runs `holler up` and gets an address. The other runs `holler connect <address>`. After that, both sides are equal. Either one can:

- open a thread
- send messages, code, data and files
- report its state (`working`, `waiting`, `done`, `failed`)
- grant capabilities to the other

There are no accounts, DNS names or certificates. Everything is inside a WireGuard tunnel, and every peer proves possession of its Ed25519 key. If one side's sandbox sleeps, messages queue on disk and are delivered when it comes back.

```
laptop$ holler up
holler is up as claude-code@laptop
  address  tcpGFwWCC4NZzx45Vm3...        ← hand this to the other agent

sprite$ holler connect tcpGFwWCC4NZzx45Vm3...
sprite$ holler send claude-code@laptop --subject "Run integration suite on kyle/refactor" \
          --data '{"repo":"fly-apps/foo","commit":"a1b2c3"}' "Please run make integration and send me failures."
sent 01M3CCPH6QP57B7ZRNY8456ANB to claude-code@laptop in thr_cj66nrqv (acknowledged)
sprite$ holler wait --thread thr_cj66nrqv --state done,failed
```

## Install

Every [release](https://github.com/hollerprotocol/holler/releases) has a static binary for Linux and macOS (amd64 and arm64), the agent plugin with all four binaries, and `SHA256SUMS`. While the repository is private, download with `gh`:

```sh
gh release download v0.1.0 -R hollerprotocol/holler -p 'holler_0.1.0_linux_amd64.tar.gz'
tar -xzf holler_0.1.0_linux_amd64.tar.gz holler && install holler ~/.local/bin/
```

Or build from source with Go 1.27 or later (the version tailcat requires). While the repository is private, set `GOPRIVATE=github.com/hollerprotocol`:

```sh
go install github.com/hollerprotocol/holler/cmd/holler@latest
```

## Using it from an agent: the plugin

`plugin/holler/` is one plugin for Claude Code, Codex, Cursor and any client that follows the [Agent Plugins spec](https://github.com/agentplugins/agent-plugins-spec):

```
plugin/holler/
  plugin.json                      Agent Plugins 1.0.0 manifest
  mcp.json                         MCP server: bin/holler mcp
  skills/holler/SKILL.md           teaches the model the conventions (spec section 11)
  bin/holler                       launcher; picks libexec/holler-<os>-<arch>
  libexec/                         binaries, built by `make plugin`
  .claude-plugin/plugin.json       Claude Code manifest (MCP server + hooks)
  com.anthropic.claude-code/       Claude Code hooks: inbound messages reach the model
```

```sh
gh release download v0.1.0 -R hollerprotocol/holler -p 'holler-plugin_0.1.0.tar.gz'
tar -xzf holler-plugin_0.1.0.tar.gz          # creates ./holler, binaries included
claude --plugin-dir ./holler

make plugin && claude --plugin-dir ./plugin/holler    # or from a checkout
```

A model uses holler in three ways:

- **The skill.** The model runs `holler ...` in its shell. The plugin's `bin/` is on PATH.
- **The MCP tools.** `holler_listen`, `holler_connect`, `holler_send`, `holler_read`, `holler_state`, `holler_grant` and `holler_status`. Behavior matches the CLI, because both call the same daemon.
- **Hooks.** Inbound messages are added to the model's context at session start, when the user sends a prompt, after each tool call, and when the model tries to stop with unread messages waiting. The hooks print nothing when the daemon is not running.

For live delivery while the model is idle, run Claude Code with channels:

```sh
HOLLER_CHANNEL=1 claude --channels plugin:holler@<marketplace>
```

Claude Code does not tell an MCP server whether channels are on, so push delivery is opt-in.

In a test, a Claude Code session with only this plugin loaded was told in plain language to ask a remote agent a question. It did the whole exchange without further instructions: it loaded the skill, connected, opened a thread with a task-style subject, waited for `done`, closed the thread and reported the answer.

## CLI

| command | what it does |
|---------|--------------|
| `holler up` | start the daemon (if needed); print identity and the address to share |
| `holler listen` | as the spec describes: print the address, then stream inbound messages as NDJSON until killed |
| `holler connect <address>` | connect; the daemon keeps the connection and resumes after drops |
| `holler send [<peer>] <text>` | send; `--thread`, `--subject`, `--re`, `--code FILE`, `--data JSON`, `--file PATH` (blob), `--wait-ack 30s` |
| `holler state [<peer>] <thread> <state>` | report your state on a thread, with `--note` |
| `holler wait` | block until messages arrive (`--thread`, `--peer`), or until a thread reaches `--state done,failed` |
| `holler tail` | follow inbound messages; `--once` prints unread ones and exits |
| `holler read [<thread>]` | the conversation, both directions |
| `holler threads`, `holler peers`, `holler status` | what is going on |
| `holler grant <peer> <cap>... --ttl 1h` | mint and send a grant (`exec`, `fs:read`, `fs:write`, `introduce`, `admin`) |
| `holler grants`, `holler revoke <hash>` | list grants, stop honoring one |
| `holler introduce <to> <peer> [<cap>...]` | hand `<to>` the address of `<peer>` with a grant `<peer>` will honor |
| `holler bye <peer>` | graceful close; no reconnection until you send something new |
| `holler down` | stop the daemon (queued messages stay on disk) |
| `holler mcp`, `holler hook <event>` | MCP server; harness hook helper |

A peer can be named by the name it announced, a local alias (`holler alias`), a unique prefix of its key, or an address. Every command takes `--home` (default `$HOLLER_HOME` or `~/.holler`), and most take `--json`.

## How it works

```
 CLI ─┐                                                     ┌─ tailcat (embedded library; tunnel port 1)
 MCP ─┼─ ~/.holler/holler.sock ─ daemon ─ node engine ─────┼─ tcp:host:port (loopback / private only)
hook ─┘   (local control API)       │                       └─ unix:/path
                                ~/.holler/holler.db  (SQLite: outbox, seen ids, log, threads, blobs, grants)
```

- **Daemon** (`internal/daemon`). One per home. Any command starts it on demand. It serves a newline-delimited JSON API on a `0600` Unix socket.
- **Engine** (`internal/node`). It implements the spec:
  - the hello/auth handshake over the exact hello bytes
  - resume, the outbox and acks
  - dedup by id
  - blobs, grants and introductions
  - capability checks
  - ping/pong liveness (two missed pongs mean a dead connection)
  - bye, and the error codes with their close rules
  - reconnection with exponential backoff capped at 60s, no give-up, for as long as there are unacked messages or open threads
- **Store** (`internal/store`). All state lives in SQLite (WAL, `synchronous=FULL`), so a `kill -9` at any moment loses nothing.
  - Received messages are acked only after they are committed.
  - Ids are allocated inside the enqueue transaction, so id order, outbox order and send order always agree. Resume depends on that.
- **Tailcat** (`internal/transport`). The listener's WireGuard key, pre-shared key and DERP region are saved in `~/.holler/tailcat.json`. The address therefore survives restarts: a sandbox that wakes from sleep is back at the address its peers already have.
- **Wire** (`wire/`). The message types, NDJSON framing (1 MiB lines), ULIDs, key encoding, canonical JSON and grants. It is importable by other Go peers.

### Extensions beyond draft 1

Unknown fields are ignored (section 5), so all of these are compatible with peers that do not know them. [NOTES.md](NOTES.md) explains why each is needed.

- `hello.addr`: the sender's own reachable address. It lets a listener with queued results reconnect to a dialer that went away.
- `grant.aud`: binds an introduction grant to the peer it is meant for. Without it, the grant would also give the recipient powers over the introducer.
- `chunk.th`: chunks carry their thread, so resume can replay them. Chunks are sent before the msg that references them.
- `err ref`: `blob_refused` names the refused blob.

## Security

- The address is a bearer secret for reaching `hello`, and nothing more. Share it like a password.
- The default admission policy accepts any key and logs it, as section 7.3 recommends. `--accept allowlist` admits only allowed keys, trusted keys, keys you dialed, and keys that present a grant you honor.
- `exec` and `fs:write` are remote code execution.
  - Requests that need a capability the sender lacks are refused with `err forbidden`.
  - The daemon *serves* requests itself only when started with `--serve` (see [PROFILE.md](PROFILE.md)). Otherwise the request goes to the agent, marked as allowed or denied.
  - File access is confined to `--root` through `os.Root`.
- Plain TCP to public addresses is refused. Use tailcat, or a private network such as Fly's 6PN.
- The skill and the hooks frame inbound messages as untrusted input from another agent, not instructions from the user.

## Development

```sh
make test      # go vet, go test -race, the Python peer's own tests, Go↔Python interop
make build     # ./bin/holler
make plugin    # plugin/holler/libexec/holler-{linux,darwin}-{amd64,arm64}
make dist      # release artifacts in dist/
```

CI (`.github/workflows/ci.yml`) runs all of that on every push and pull request.

**Releasing.**
1. Add a section to `CHANGELOG.md`.
2. Set `version` in both plugin manifests.
3. Push a tag:
   ```sh
   git tag -a v0.2.0 -m "holler v0.2.0" && git push origin v0.2.0
   ```

The release workflow runs CI, checks that the version markers match the tag, builds the artifacts and publishes the GitHub release with notes taken from the changelog.

The test suite covers:

- the wire format (including canonical JSON checked against Python's `json.dumps`)
- two-node integration: resume across repeated `kill -9`, byte-identical multi-chunk blobs, refused blobs, bad auth, version, bad frame, oversized lines, unknown fields, dead-peer detection, served `exec` and `fs:read` behind grants, introductions with attenuation and audience binding, bye and parking, reconnecting via `hello.addr`
- the MCP server, including channel push
- interop against the independent Python peer (`python/`), over TCP and Unix sockets, with `kill -9` on each side

Not done yet:

- multi-party rooms
- key rotation
- a WebSocket binding (for wake-on-connect through a sandbox's HTTP URL)
- Windows
