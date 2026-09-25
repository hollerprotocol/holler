// Quiet cues for live activity and for the interface itself, synthesized
// with @web-kits/audio. Off until the viewer turns them on (browsers need a
// gesture to start audio), and throttled so a burst of events is one sound,
// not a clatter.
//
// Interface sounds: every button, link, tab and row taps when pressed. An
// element (or an ancestor) can ask for another cue with data-sound="select",
// "open", "close", "on", "off" or "copy", or for silence with
// data-sound="none". Opening and closing panels play from the code that does
// it (ui("open")).
import { useEffect, useRef, useSyncExternalStore } from "react"
import type { Activity } from "./types"

type Cue = "message" | "working" | "waiting" | "done" | "failed" | "joined" | "left"
export type UiCue = "tap" | "select" | "open" | "close" | "on" | "off" | "copy"

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
let uiPlayers: Record<UiCue, () => unknown> | undefined
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
    const click = { attack: 0.001, decay: 0.03, sustain: 0, release: 0.01 }
    uiPlayers = {
      // a dry, tiny click: felt more than heard
      tap: defineSound({
        layers: [
          { source: { type: "noise", color: "white" }, filter: { type: "bandpass", frequency: 3200, resonance: 1.4 }, envelope: click, gain: 0.05 },
          { source: { type: "sine", frequency: { start: 1900, end: 1300 } }, envelope: click, gain: 0.035 },
        ],
      }),
      // a rounder tick, for choosing a view or a filter
      select: defineSound({
        source: { type: "triangle", frequency: { start: 880, end: 990 } },
        envelope: { attack: 0.002, decay: 0.06, sustain: 0, release: 0.03 },
        gain: 0.08,
      }),
      // a soft upward swish as a panel slides in
      open: defineSound({
        layers: [
          { source: { type: "noise", color: "pink" }, filter: { type: "bandpass", frequency: 900, resonance: 2, envelope: { attack: 0.1, peak: 2600, decay: 0.05 } }, envelope: { attack: 0.03, decay: 0.12, sustain: 0, release: 0.05 }, gain: 0.05 },
          { source: { type: "sine", frequency: { start: 520, end: 780 } }, envelope: { attack: 0.01, decay: 0.1, sustain: 0, release: 0.04 }, gain: 0.05 },
        ],
      }),
      // and back down as it leaves
      close: defineSound({
        layers: [
          { source: { type: "noise", color: "pink" }, filter: { type: "bandpass", frequency: 800, resonance: 2, envelope: { attack: 0, peak: 2400, decay: 0.1 } }, envelope: { attack: 0.02, decay: 0.1, sustain: 0, release: 0.04 }, gain: 0.04 },
          { source: { type: "sine", frequency: { start: 700, end: 460 } }, envelope: { attack: 0.005, decay: 0.08, sustain: 0, release: 0.03 }, gain: 0.04 },
        ],
      }),
      // a switch flipping up, and down
      on: defineSound({
        layers: [
          { source: { type: "sine", frequency: 660 }, envelope: click, gain: 0.07 },
          { source: { type: "sine", frequency: 990 }, envelope: { ...click, decay: 0.05 }, gain: 0.06, delay: 0.045 },
        ],
      }),
      off: defineSound({
        layers: [
          { source: { type: "sine", frequency: 990 }, envelope: click, gain: 0.06 },
          { source: { type: "sine", frequency: 660 }, envelope: { ...click, decay: 0.05 }, gain: 0.06, delay: 0.045 },
        ],
      }),
      // a bright little confirmation
      copy: defineSound({
        layers: [
          { source: { type: "sine", frequency: 1318.5 }, envelope: { attack: 0.002, decay: 0.07, sustain: 0, release: 0.04 }, gain: 0.07 },
          { source: { type: "sine", frequency: 1760 }, envelope: { attack: 0.002, decay: 0.12, sustain: 0, release: 0.05 }, gain: 0.06, delay: 0.06 },
        ],
      }),
    }
  })()
  await loading
}

export function soundEnabled(): boolean {
  return enabled
}

export async function setSound(on: boolean) {
  if (!on) ui("off")
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
      ui("on")
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

let lastUi = 0

/** Plays an interface sound, when sounds are on. */
export function ui(cue: UiCue) {
  if (!enabled) return
  if (!uiPlayers) {
    // this is a gesture, so the audio context may start now
    load().catch(() => {})
    return
  }
  const t = performance.now()
  // a tap right before a richer cue (a click that opens a panel) is enough
  if (cue === "tap" && t - lastUi < 60) return
  lastUi = t
  try {
    uiPlayers[cue]()
  } catch {
    // no audio device; silence is fine
  }
}

const PRESSABLE = 'button, a[href], [role="tab"], [role="option"], [role="menuitem"], [data-row], [data-menu-row], summary, label'

/** Taps (or data-sound cues) for every press in the page. */
export function installUiSounds() {
  if (typeof window === "undefined") return
  document.addEventListener(
    "pointerdown",
    (e) => {
      if (e.button !== 0) return
      const el = (e.target as Element | null)?.closest?.(PRESSABLE)
      if (!el || (el as HTMLButtonElement).disabled) return
      const cue = el.closest("[data-sound]")?.getAttribute("data-sound")
      if (cue === "none") return
      ui((cue as UiCue) || "tap")
    },
    { capture: true },
  )
}

/** Plays "open" when a panel opens and "close" when it closes. */
export function usePanelSound(open: boolean) {
  const first = useRef(true)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    ui(open ? "open" : "close")
  }, [open])
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
