// Quiet cues for live activity, synthesized with @web-kits/audio. Off until
// the viewer turns them on (browsers need a gesture to start audio), and
// throttled so a burst of events is one sound, not a clatter.
import { useSyncExternalStore } from "react"
import type { Activity } from "./types"

type Cue = "message" | "working" | "waiting" | "done" | "failed" | "joined" | "left"

const KEY = "holler.sound"

function readPref(): boolean {
  try {
    return localStorage.getItem(KEY) === "on"
  } catch {
    return false
  }
}

let enabled = readPref()
const listeners = new Set<() => void>()
let players: Record<Cue, (o?: { volume?: number }) => unknown> | undefined
let loading: Promise<void> | undefined

async function load() {
  if (players) return
  loading ??= (async () => {
    const { defineSound, ensureReady, setMasterVolume } = await import("@web-kits/audio")
    await ensureReady()
    setMasterVolume(0.55)
    const soft = { attack: 0.004, decay: 0.16, sustain: 0, release: 0.08 }
    players = {
      // a soft droplet
      message: defineSound({
        source: { type: "sine", frequency: { start: 1180, end: 760 } },
        envelope: { attack: 0.002, decay: 0.09, sustain: 0, release: 0.05 },
        gain: 0.16,
      }),
      // two notes rising
      working: defineSound({
        layers: [
          { source: { type: "triangle", frequency: 523.25 }, envelope: soft, gain: 0.11 },
          { source: { type: "triangle", frequency: 659.25 }, envelope: soft, gain: 0.1, delay: 0.07 },
        ],
      }),
      // a held, questioning fifth
      waiting: defineSound({
        layers: [
          { source: { type: "sine", frequency: 587.33 }, envelope: soft, gain: 0.11 },
          { source: { type: "sine", frequency: 440 }, envelope: soft, gain: 0.09, delay: 0.09 },
        ],
      }),
      // a bright major chord, arpeggiated
      done: defineSound({
        layers: [
          { source: { type: "sine", frequency: 659.25 }, envelope: { ...soft, decay: 0.3 }, gain: 0.1 },
          { source: { type: "sine", frequency: 830.61 }, envelope: { ...soft, decay: 0.3 }, gain: 0.09, delay: 0.06 },
          { source: { type: "sine", frequency: 987.77 }, envelope: { ...soft, decay: 0.4 }, gain: 0.08, delay: 0.12 },
        ],
      }),
      // low and falling
      failed: defineSound({
        source: { type: "triangle", frequency: { start: 330, end: 196 } },
        filter: { type: "lowpass", frequency: 1400 },
        envelope: { attack: 0.004, decay: 0.28, sustain: 0, release: 0.08 },
        gain: 0.14,
      }),
      // an airy swell
      joined: defineSound({
        layers: [
          { source: { type: "noise", color: "pink" }, filter: { type: "bandpass", frequency: 1800, resonance: 3 }, envelope: { attack: 0.05, decay: 0.25 }, gain: 0.05 },
          { source: { type: "sine", frequency: { start: 392, end: 784 } }, envelope: { attack: 0.02, decay: 0.25 }, gain: 0.07 },
        ],
      }),
      left: defineSound({
        source: { type: "sine", frequency: { start: 660, end: 330 } },
        envelope: { attack: 0.02, decay: 0.3 },
        gain: 0.06,
      }),
    }
  })()
  await loading
}

export function soundEnabled(): boolean {
  return enabled
}

export async function setSound(on: boolean) {
  enabled = on
  try {
    localStorage.setItem(KEY, on ? "on" : "off")
  } catch {
    // not remembered; still works for this visit
  }
  listeners.forEach((fn) => fn())
  if (on) {
    try {
      await load()
      play("message", true)
    } catch {
      enabled = false
      listeners.forEach((fn) => fn())
    }
  }
}

export function useSound(): boolean {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn)
      return () => {
        listeners.delete(fn)
      }
    },
    () => enabled,
  )
}

let lastAny = 0
const lastCue = new Map<Cue, number>()

function play(cue: Cue, force = false) {
  if (!enabled || !players) return
  const t = performance.now()
  if (!force && (t - lastAny < 140 || t - (lastCue.get(cue) ?? 0) < 450)) return
  lastAny = t
  lastCue.set(cue, t)
  try {
    players[cue]()
  } catch {
    // audio can fail (device gone); a missing cue is fine
  }
}

/** The cue for an activity item, if any. */
export function cueFor(a: Activity): Cue | undefined {
  switch (a.kind) {
    case "msg":
      return "message"
    case "state":
      if (a.state === "working") return "working"
      if (a.state === "waiting") return "waiting"
      if (a.state === "done") return "done"
      if (a.state === "failed") return "failed"
      return undefined
    case "error":
      return "failed"
    case "joined":
    case "connected":
      return "joined"
    case "left":
    case "disconnected":
      return "left"
  }
  return undefined
}

export function playFor(a: Activity) {
  const cue = cueFor(a)
  if (!cue || !enabled) return
  // the first enable loads the synth; after a reload with sound on, the
  // first gesture-free event may not be able to start audio yet
  if (!players) {
    load().catch(() => {})
    return
  }
  play(cue)
}

// With sound remembered as on, start the audio context on the first gesture.
if (typeof window !== "undefined" && enabled) {
  const start = () => {
    load().catch(() => {})
    window.removeEventListener("pointerdown", start)
    window.removeEventListener("keydown", start)
  }
  window.addEventListener("pointerdown", start)
  window.addEventListener("keydown", start)
}
