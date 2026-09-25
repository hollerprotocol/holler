// Which avatar agents wear: the gradient orbs, or Bot avatars
// (libraries.dev/bots). A per-viewer choice, remembered in this browser.
import { useSyncExternalStore } from "react"
import type { BotAvatarFace, BotAvatarType } from "bot-avatars"

export type AvatarStyle = "orbs" | "bots"

const KEY = "holler.avatars"
let style: AvatarStyle = (() => {
  try {
    return localStorage.getItem(KEY) === "orbs" ? "orbs" : "bots"
  } catch {
    return "bots"
  }
})()
const listeners = new Set<() => void>()

export function setAvatarStyle(s: AvatarStyle) {
  style = s
  try {
    localStorage.setItem(KEY, s)
  } catch {
    // not remembered; fine for this visit
  }
  listeners.forEach((fn) => fn())
}

export function useAvatarStyle(): AvatarStyle {
  return useSyncExternalStore(
    (fn) => {
      listeners.add(fn)
      return () => {
        listeners.delete(fn)
      }
    },
    () => style,
  )
}

const TYPES: BotAvatarType[] = [
  "clover", "flower", "triangle", "square", "blob", "ghost", "circle", "drop", "star",
  "droid", "mech", "alien", "hexagon", "cat", "cloud", "pill", "pebble", "puddle",
]

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** An agent's bot: the same shape, face, shade and rhythm wherever it
 *  appears. With 18 shapes, agents on a network often share one, so the
 *  shade varies too (brightness 0.88 to 1.12). */
export function botFor(key: string): { type: BotAvatarType; face: BotAvatarFace; seed: number; brightness: number } {
  const h = hash(key)
  const g = hash(key + "#shade")
  return {
    type: TYPES[h % TYPES.length],
    face: (h >>> 8) & 1 ? "mouth" : "eyes",
    seed: ((h >>> 12) % 1000) / 1000,
    brightness: 0.88 + ((g % 1000) / 1000) * 0.24,
  }
}
