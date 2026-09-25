// An agent as a Bot avatar (libraries.dev/bots), loaded on first use so the
// orb style costs nothing.
import { lazy, Suspense } from "react"

import { botFor } from "@/lib/avatar"
import { orbBackground } from "@/lib/orb"

const BotAvatar = lazy(() => import("bot-avatars").then((m) => ({ default: m.BotAvatar })))

// The canvas keeps headroom for hops; drawn at a little over its layout box,
// overflowing evenly, the body sits at a quiet size beside labels.
const SCALE = 1.05

// The dashboard's look, chosen in a side-by-side comparison of the library's
// props: matte "smooth" shading, a little desaturated, soft shadow, and
// slower, rarer, lower hops, so a busy network stays calm.
const LOOK = {
  shading: "smooth",
  saturation: 0.82,
  shadow: 0.25,
  highlight: 1.15,
  depth: 0.55,
  speed: 0.8,
  jumpHeight: 16,
  jumpSpin: 0.5,
} as const

export function Bot({ agentKey, size, working = false, dim = false }: { agentKey: string; size: number; working?: boolean; dim?: boolean }) {
  const b = botFor(agentKey)
  const drawn = Math.round(size * SCALE)
  const bleed = (drawn - size) / 2
  // While it loads, a soft disc in the agent's colours holds the place.
  const placeholder = <span className="inline-block shrink-0 rounded-full opacity-40" style={{ width: size, height: size, background: orbBackground(agentKey) }} />
  return (
    // The layout box stays size×size; the larger canvas is centred on it
    // and bleeds past its edges without moving anything.
    <span className="relative inline-block shrink-0" style={{ width: size, height: size }}>
      <Suspense fallback={placeholder}>
        <BotAvatar
          type={b.type}
          face={b.face}
          seed={b.seed}
          size={drawn}
          state={dim ? "sleeping" : working ? "working" : "default"}
          {...LOOK}
          brightness={b.brightness}
          interactive={size >= 40}
          turn={size >= 40 ? 0.7 : 0}
          jumpEvery={size >= 40 ? 16 : 0}
          style={{ position: "absolute", left: -bleed, top: -bleed, maxWidth: "none" }}
        />
      </Suspense>
    </span>
  )
}
