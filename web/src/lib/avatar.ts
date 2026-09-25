// Agents' avatars: Boring Avatars' "marble" in holler's palette, drawn from
// the agent's key, so the same agent looks the same on every host and visit.

export const PALETTE = ["#00686c", "#32c2b9", "#edecb3", "#fad928", "#ff9915"]

// The palette's strong colours: the cream and the yellow vanish as a thin
// ring or a moving dot on a light page.
const STRONG = ["#00686c", "#32c2b9", "#ff9915"]

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** One colour for an agent, for its pings and the pulses it sends. */
export function agentColor(key: string): string {
  return STRONG[hash(key || "?") % STRONG.length]
}
