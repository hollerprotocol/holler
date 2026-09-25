// Dev-only: the same agents rendered across Bot avatar prop variations, to
// choose the dashboard's look. Open /?botlab in the dev server.
import { BotAvatar, type BotAvatarProps } from "bot-avatars"

import { botFor } from "@/lib/avatar"

const KEYS = ["ed25519:worker", "ed25519:boss", "ed25519:builder", "ed25519:research", "ed25519:frontend", "ed25519:nightly", "ed25519:ops"]

const VARIANTS: { name: string; props: Partial<BotAvatarProps> }[] = [
  { name: "library default (plastic)", props: {} },
  { name: "plastic, calmer: sat 0.8, highlight 1, shadow 0.25", props: { saturation: 0.8, highlight: 1, shadow: 0.25 } },
  { name: "plastic, soft: sat 0.85, rim 0.25, spread 2, depth 0.5", props: { saturation: 0.85, rim: 0.25, spread: 2, depth: 0.5 } },
  { name: "smooth", props: { shading: "smooth" } },
  { name: "smooth, sat 0.8, shadow 0.25", props: { shading: "smooth", saturation: 0.8, shadow: 0.25 } },
  { name: "crisp", props: { shading: "crisp" } },
  { name: "crisp, sat 0.8, rim 0.35", props: { shading: "crisp", saturation: 0.8, rim: 0.35 } },
  { name: "flat", props: { shading: "flat" } },
  { name: "flat, sat 0.75, brightness 1.08", props: { shading: "flat", saturation: 0.75, brightness: 1.08 } },
]

export function BotLab() {
  return (
    <div className="min-h-svh bg-page p-6 text-ink">
      {VARIANTS.map((v) => (
        <div key={v.name} className="mb-3 flex items-center gap-4">
          <div className="w-80 shrink-0 font-mono text-[12px] text-ink-2">{v.name}</div>
          {[24, 36].map((size) =>
            KEYS.map((k) => {
              const b = botFor(k)
              return (
                <BotAvatar key={`${size}${k}`} type={b.type} face={b.face} seed={b.seed} size={size} state={k.endsWith("nightly") ? "sleeping" : "default"} jumpEvery={0} turn={0} paused {...v.props} />
              )
            }),
          )}
        </div>
      ))}
    </div>
  )
}
