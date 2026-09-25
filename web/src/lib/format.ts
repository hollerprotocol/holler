import type { Agent, ThreadState } from "./types"

/** "now", "12s", "4m", "3h", "2d". */
export function ago(iso: string | undefined, now = Date.now()): string {
  if (!iso) return ""
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ""
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 5) return "now"
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

export function clock(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ""
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
}

/** The part of a name before "@", and the host after it. */
export function splitName(name: string): { who: string; host: string } {
  const i = name.lastIndexOf("@")
  if (i <= 0) return { who: name, host: "" }
  return { who: name.slice(0, i), host: name.slice(i + 1) }
}

export function agentName(a: Pick<Agent, "name" | "short"> | undefined, fallback = "unknown"): string {
  if (!a) return fallback
  return a.name || a.short || fallback
}

export type Tone = "neutral" | "green" | "orange" | "red" | "accent"

/** How a thread state reads at a glance. */
export function stateTone(s: ThreadState | undefined): Tone {
  switch (s) {
    case "working":
      return "accent"
    case "waiting":
      return "orange"
    case "done":
      return "green"
    case "failed":
      return "red"
    default:
      return "neutral"
  }
}

export const ENDED = new Set<ThreadState>(["done", "failed", "closed"])

export function isEnded(s: ThreadState | undefined): boolean {
  return !!s && ENDED.has(s)
}

export function plural(n: number, one: string, many = one + "s"): string {
  return `${n} ${n === 1 ? one : many}`
}

/** "just now" or "4m ago". */
export function agoText(iso: string | undefined, now = Date.now()): string {
  const a = ago(iso, now)
  if (!a) return ""
  return a === "now" ? "just now" : `${a} ago`
}
