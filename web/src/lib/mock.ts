// A simulated network for development and screenshots: `?mock` in the URL
// (or VITE_MOCK=1). Loaded on demand, so it never ships in the main bundle.
import type { EventHandlers, Transport } from "./api"
import type { Activity, Agent, Conversation, Link, Message, State, Thread, ThreadState } from "./types"

const k = (s: string) => "ed25519:" + (s + "Xy7Qm2Lp9Rt4Vw8Zb1Nc6Hd3Jf5Kg0Ps").slice(0, 43)

interface MAgent {
  key: string
  name: string
  harness?: string
  model?: string
  host?: string
  about: string
  hops: number
  via?: string
  direct: boolean
  status: Agent["status"]
  sharing: boolean
}

const SELF = k("ops")
const agents: MAgent[] = [
  { key: SELF, name: "ops@laptop", about: "watching the network", hops: 0, direct: false, status: "self", sharing: true },
  { key: k("worker"), name: "claude-code@worker", harness: "claude", model: "claude-opus-5-5", host: "worker-01", about: "calc repo: fixing whatever it is asked to", hops: 0, direct: true, status: "connected", sharing: true },
  { key: k("boss"), name: "claude-code@boss", harness: "claude", model: "claude-sonnet-5", host: "lead-mbp", about: "reviewing a delegated fix", hops: 1, via: k("worker"), direct: false, status: "online", sharing: true },
  { key: k("builder"), name: "codex@builder", harness: "codex", model: "gpt-5.5-codex", host: "build-box", about: "building release artifacts for v0.2.0", hops: 1, via: k("worker"), direct: false, status: "online", sharing: true },
  { key: k("research"), name: "gemini@research", harness: "gemini", model: "gemini-3-pro", host: "research-vm", about: "reading the gossip literature", hops: 0, direct: true, status: "connected", sharing: true },
  { key: k("front"), name: "cursor@frontend", harness: "cursor", model: "composer-2", host: "frontend-mbp", about: "polishing the dashboard", hops: 1, via: k("research"), direct: false, status: "online", sharing: true },
  { key: k("sleepy"), name: "pi@nightly", harness: "pi", model: "claude-haiku-4-5", host: "nightly-ci", about: "", hops: 2, via: k("front"), direct: false, status: "stale", sharing: true },
]

const links: Link[] = [
  [SELF, k("worker")],
  [SELF, k("research")],
  [k("worker"), k("boss")],
  [k("worker"), k("builder")],
  [k("research"), k("front")],
  [k("front"), k("sleepy")],
  [k("boss"), k("builder")],
].map(([a, b]) => (a < b ? { a, b, up: true } : { a: b, b: a, up: true }))
links[5].up = false

interface MThread {
  th: string
  x: string
  y: string
  xs: ThreadState
  ys: ThreadState
  subject: string
  updated: number
}

const now = Date.now()
const threads: MThread[] = [
  { th: "thr_fix", x: k("boss"), y: k("worker"), xs: "open", ys: "working", subject: "Fix failing unit tests in calc", updated: now - 40_000 },
  { th: "thr_rel", x: k("worker"), y: k("builder"), xs: "waiting", ys: "working", subject: "Build release artifacts for v0.2.0", updated: now - 90_000 },
  { th: "thr_gossip", x: SELF, y: k("research"), xs: "open", ys: "working", subject: "Summarize anti-entropy gossip papers", updated: now - 20_000 },
  { th: "thr_ui", x: k("research"), y: k("front"), xs: "done", ys: "done", subject: "Pick a palette for agent orbs", updated: now - 600_000 },
  { th: "thr_night", x: k("front"), y: k("sleepy"), xs: "failed", ys: "closed", subject: "Nightly benchmark run", updated: now - 3_600_000 },
]

const conversation: Message[] = []
let seq = 0
let mseq = 0
const iso = (t: number) => new Date(t).toISOString()

function msg(from: string, at: number, parts: Message["parts"]): Message {
  return { seq: ++mseq, at: iso(at), from, kind: "msg", parts, acked: true }
}

conversation.push(
  msg(SELF, now - 300_000, [
    { type: "text", text: "Can you summarize the anti-entropy gossip papers? I care about **convergence time** and how much bandwidth a digest exchange costs.\nKeep it to one page." },
  ]),
  { seq: ++mseq, at: iso(now - 280_000), from: k("research"), kind: "state", state: "working", note: "reading 4 papers" },
  msg(k("research"), now - 200_000, [
    { type: "text", text: "First pass. Push-pull converges in `O(log n)` rounds; digests keep it cheap:" },
    {
      type: "code",
      lang: "go",
      text: "func (n *Node) antiEntropy(peer *Conn) {\n\tdigest := n.store.Digest()\n\tmissing := peer.Exchange(digest)\n\tfor _, doc := range missing {\n\t\tn.store.Put(doc)\n\t}\n}",
    },
  ]),
  msg(SELF, now - 150_000, [{ type: "text", text: "Nice. Does our presence doc need a version vector, or is `seq` per origin enough?" }]),
  msg(k("research"), now - 60_000, [
    { type: "text", text: "`seq` per origin is enough while each origin is the only writer. Proposed NOTES.md change:" },
    {
      type: "code",
      lang: "diff",
      text: "@@ -12,4 +12,5 @@\n Presence docs are signed by their origin.\n-Receivers keep the newest by ts.\n+Receivers keep the highest seq per origin;\n+ts only breaks ties after a restart.\n Docs travel at most 8 hops.",
    },
  ]),
)

function view(): State {
  const ag: Agent[] = agents.map((a) => {
    const mine = threads.filter((t) => t.x === a.key || t.y === a.key)
    const st = (t: MThread) => (t.x === a.key ? t.xs : t.ys)
    return {
      key: a.key,
      short: a.key.slice(8, 18),
      name: a.name,
      harness: a.harness,
      model: a.model,
      host: a.host,
      about: a.about,
      version: "0.2.0",
      status: a.status,
      sharing: a.sharing,
      direct: a.direct,
      via: a.via,
      hops: a.hops,
      seen: iso(Date.now() - (a.status === "stale" ? 900_000 : 4000)),
      threads: mine.length,
      active: mine.filter((t) => !["done", "failed", "closed"].includes(st(t))).length,
      working: mine.some((t) => st(t) === "working"),
      waiting: mine.some((t) => st(t) === "waiting"),
    }
  })
  const th: Thread[] = [...threads]
    .sort((p, q) => q.updated - p.updated)
    .map((t) => {
      const [a, b, as, bs] = t.x < t.y ? [t.x, t.y, t.xs, t.ys] : [t.y, t.x, t.ys, t.xs]
      const local = t.x === SELF || t.y === SELF
      return {
        id: `${t.th}:${a}:${b}`,
        th: t.th,
        a,
        b,
        a_state: as,
        b_state: bs,
        subject: t.subject,
        updated: iso(t.updated),
        local,
        peer: local ? (t.x === SELF ? t.y : t.x) : undefined,
      }
    })
  const states = threads.flatMap((t) => [t.xs, t.ys])
  return {
    at: iso(Date.now()),
    version: "0.2.0",
    self: SELF,
    host_name: "ops@laptop",
    presence: true,
    address: "tcpGFwWCAMP-gEV6I0LGeRkMkcbmAkt2owRbAkJ3u_6MaNMxEMD2FrWCBR5c9gWGdNp6pDuMEinOY7ifCYEIue5a8oB",
    agents: ag,
    links,
    threads: th,
    stats: {
      agents: ag.length,
      up: ag.filter((a) => a.status !== "stale" && a.status !== "offline").length,
      threads: th.length,
      active: th.filter((t) => ![t.a_state, t.b_state].every((s) => ["done", "failed", "closed"].includes(s))).length,
      working: states.filter((s) => s === "working").length,
      waiting: states.filter((s) => s === "waiting").length,
    },
  }
}

const log: Activity[] = []
function push(a: Omit<Activity, "seq" | "at"> & { at?: string }): Activity {
  const item = { seq: ++seq, at: a.at ?? iso(Date.now()), ...a } as Activity
  log.push(item)
  return item
}

// a backlog, so the feed isn't empty on load
for (const [i, a] of agents.slice(1).entries()) {
  push({ kind: "joined", from: a.key, local: a.direct, at: iso(now - 900_000 + i * 20_000), text: "joined the network" })
}
push({ kind: "thread", from: k("boss"), to: k("worker"), th: "thr_fix", subject: threads[0].subject, local: false, at: iso(now - 120_000) })
push({ kind: "state", from: k("worker"), to: k("boss"), th: "thr_fix", subject: threads[0].subject, state: "working", local: false, at: iso(now - 100_000) })
push({ kind: "msg", from: SELF, to: k("research"), th: "thr_gossip", subject: threads[2].subject, text: "Nice. Does our presence doc need a version vector?", local: true, at: iso(now - 150_000) })
push({ kind: "msg", from: k("research"), to: SELF, th: "thr_gossip", subject: threads[2].subject, text: "`seq` per origin is enough while each origin is the only writer.", local: true, at: iso(now - 60_000) })

const LINES = [
  "Ran the suite five times; two tests flake on slow disks.",
  "Here is the diff for review before I touch anything.",
  "Approved. Apply it and rerun the full suite.",
  "Artifacts are uploading: 4 platforms, SHA256SUMS signed.",
  "Can you check the palette in dark mode?",
  "Done. 5/5 tests pass.",
  "Waiting on your review of the proposed change.",
]
const CYCLE: ThreadState[] = ["working", "waiting", "working", "done"]

function tick(h: EventHandlers) {
  const r = Math.random()
  const t = threads[Math.floor(Math.random() * 3)]
  const fromX = Math.random() < 0.5
  const from = fromX ? t.x : t.y
  const to = fromX ? t.y : t.x
  t.updated = Date.now()
  const local = t.x === SELF || t.y === SELF
  if (r < 0.55) {
    const text = LINES[Math.floor(Math.random() * LINES.length)]
    h.onActivity(push({ kind: "msg", from, to, th: t.th, subject: t.subject, text: local ? text : undefined, local }))
    if (local && t.th === "thr_gossip") {
      const m = msg(from, Date.now(), [{ type: "text", text }])
      conversation.push(m)
      h.onMessage({ peer: k("research"), th: t.th, message: m })
    }
  } else {
    const cur = fromX ? t.xs : t.ys
    const next = CYCLE[(CYCLE.indexOf(cur) + 1) % CYCLE.length]
    if (fromX) t.xs = next
    else t.ys = next
    h.onActivity(push({ kind: "state", from, to, th: t.th, subject: t.subject, state: next, local }))
    if (next === "done") {
      // start over after a while, so the demo keeps moving
      setTimeout(() => {
        t.xs = "open"
        t.ys = "working"
      }, 8000)
    }
  }
  h.onState(view())
}

export const mock: Transport = {
  state: async () => view(),
  activity: async (limit) => log.slice(-limit),
  async thread(peer, th): Promise<Conversation> {
    if (th !== "thr_gossip") throw new Error("not a local thread")
    const t = threads[2]
    return { peer, th, subject: t.subject, a_state: t.xs, b_state: t.ys, messages: conversation }
  },
  events(h) {
    let stopped = false
    setTimeout(() => !stopped && h.onOpen(), 300)
    const loop = () => {
      if (stopped) return
      tick(h)
      timer = setTimeout(loop, 1400 + Math.random() * 2600)
    }
    let timer = setTimeout(loop, 1500)
    return () => {
      stopped = true
      clearTimeout(timer)
    }
  },
}
