// The dashboard's views live in the URL hash (#/network), so a reload or a
// shared link keeps the view, and the server needs no routes.
import { useEffect, useState } from "react"

export const VIEWS = ["overview", "network", "threads", "agents", "activity"] as const
export type View = (typeof VIEWS)[number]

function read(): View {
  const v = window.location.hash.replace(/^#\/?/, "")
  return (VIEWS as readonly string[]).includes(v) ? (v as View) : "overview"
}

export function useView(): [View, (v: View) => void] {
  const [view, setView] = useState<View>(read)
  useEffect(() => {
    const on = () => setView(read())
    window.addEventListener("hashchange", on)
    return () => window.removeEventListener("hashchange", on)
  }, [])
  const go = (v: View) => {
    const hash = v === "overview" ? "" : `#/${v}`
    if (window.location.hash !== hash) history.pushState(null, "", hash || window.location.pathname + window.location.search)
    setView(v)
    window.scrollTo({ top: 0 })
  }
  return [view, go]
}
