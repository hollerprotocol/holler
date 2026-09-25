import type { CSSProperties } from "react"

import { orbColors } from "@/lib/orb"

import { useAvatarStyle } from "@/lib/avatar"

import { Bot } from "./Bot"
import { HarnessBadge } from "./HarnessLogo"

/** An agent's gradient orb. Working agents breathe and swirl faster; quiet
 *  ones fade to grey. With a harness, its logo sits on the orb's edge. */
export function Orb({
  agentKey,
  size = 32,
  working = false,
  dim = false,
  harness,
  className = "",
  style,
}: {
  agentKey: string
  size?: number
  working?: boolean
  dim?: boolean
  harness?: string
  className?: string
  style?: CSSProperties
}) {
  const c = orbColors(agentKey)
  const avatars = useAvatarStyle()
  const orb =
    avatars === "bots" ? (
      <span className={`inline-flex shrink-0 ${className}`} style={style}>
        <Bot agentKey={agentKey} size={size} working={working} dim={dim} />
      </span>
    ) : (
    <span
      aria-hidden
      className={`orb inline-block shrink-0 ${working ? "is-working" : ""} ${dim ? "is-dim" : ""} ${className}`}
      style={
        {
          width: size,
          height: size,
          "--o1": c.o1,
          "--o2": c.o2,
          "--o3": c.o3,
          ...style,
        } as CSSProperties
      }
    />
  )
  if (!harness) return orb
  const badge = Math.max(13, Math.round(size * 0.46))
  return (
    <span className="relative inline-flex shrink-0">
      {orb}
      <HarnessBadge harness={harness} size={badge} className="absolute" style={{ right: -badge * 0.12, bottom: -badge * 0.12 }} />
    </span>
  )
}
