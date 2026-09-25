// Every agent gets a soft, deterministic gradient from its key: the same
// agent looks the same on every host and every visit.

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

export interface OrbColors {
  o1: string
  o2: string
  o3: string
  /** a single representative color, for links, chips and chart series */
  solid: string
}

const cache = new Map<string, OrbColors>()

export function orbColors(key: string): OrbColors {
  const hit = cache.get(key)
  if (hit) return hit
  const h = hash(key || "?")
  const base = h % 360
  const spread = 40 + ((h >>> 9) % 70)
  const second = (base + spread) % 360
  const third = (base + spread * 2 + 30) % 360
  const c: OrbColors = {
    o1: `oklch(0.9 0.08 ${base})`,
    o2: `oklch(0.72 0.14 ${second})`,
    o3: `oklch(0.82 0.1 ${third})`,
    solid: `oklch(0.66 0.14 ${second})`,
  }
  cache.set(key, c)
  return c
}

/** The orb as a flat background, for chips too small for the full effect. */
export function orbBackground(key: string): string {
  const c = orbColors(key)
  return `radial-gradient(circle at 30% 28%, ${c.o1} 0%, transparent 60%), radial-gradient(circle at 72% 74%, ${c.o2} 0%, transparent 65%), ${c.o3}`
}
