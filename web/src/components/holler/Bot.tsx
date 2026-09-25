// An agent as a Bot avatar (libraries.dev/bots), loaded on first use so the
// orb style costs nothing.
import { lazy, Suspense } from "react"

import { botFor } from "@/lib/avatar"
import { orbBackground } from "@/lib/orb"

const BotAvatar = lazy(() => import("bot-avatars").then((m) => ({ default: m.BotAvatar })))

// The canvas keeps headroom for hops, so the body fills only part of it: draw
// it larger than its layout box, overflowing evenly, to match an orb's mass.
const SCALE = 1.3

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
          interactive={size >= 40}
          turn={size >= 40 ? 1 : 0.4}
          jumpEvery={size >= 40 ? 8 : 0}
          style={{ position: "absolute", left: -bleed, top: -bleed, maxWidth: "none" }}
        />
      </Suspense>
    </span>
  )
}
