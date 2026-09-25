import type { CSSProperties } from "react"

import { orbColors } from "@/lib/orb"

/** An agent's gradient orb. Working agents breathe and swirl faster; quiet
 *  ones fade to grey. */
export function Orb({
  agentKey,
  size = 32,
  working = false,
  dim = false,
  className = "",
  style,
}: {
  agentKey: string
  size?: number
  working?: boolean
  dim?: boolean
  className?: string
  style?: CSSProperties
}) {
  const c = orbColors(agentKey)
  return (
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
}
