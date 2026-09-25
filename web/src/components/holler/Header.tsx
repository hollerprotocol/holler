import { Arc, Pulse, Ring } from "loading-dev"

import { useTheme } from "@/components/theme-provider"
import { setSound, useSound } from "@/lib/sound"
import { getStore, type Conn } from "@/lib/store"

export function Wordmark() {
  return (
    <span className="flex items-center gap-2">
      <span
        aria-hidden
        className="orb size-[22px]"
        style={{ "--o1": "oklch(0.92 0.07 60)", "--o2": "oklch(0.7 0.15 290)", "--o3": "oklch(0.84 0.1 220)" } as React.CSSProperties}
      />
      <span className="text-[17px] font-semibold tracking-[-0.03em] text-ink">holler</span>
    </span>
  )
}

export function Connection({ conn }: { conn: Conn }) {
  if (conn === "live") {
    return (
      <span className="inline-flex h-8 items-center gap-2 rounded-full bg-surface pr-3 pl-2 text-[12.5px] font-medium text-ink-2 shadow-hairline" title="Streaming live from the host">
        <Pulse size={16} color="var(--green)" duration={1600} />
        Live
      </span>
    )
  }
  if (conn === "connecting") {
    return (
      <span className="inline-flex h-8 items-center gap-2 rounded-full bg-surface pr-3 pl-2 text-[12.5px] font-medium text-ink-3 shadow-hairline">
        <Arc size={14} color="var(--ink-3)" />
        Connecting
      </span>
    )
  }
  return (
    <button
      type="button"
      onClick={() => getStore().reconnect()}
      title="The stream dropped; retrying with backoff. Click to retry now."
      className="inline-flex h-8 items-center gap-2 rounded-full bg-orange-tint pr-3 pl-2 text-[12.5px] font-medium text-orange shadow-[0_0_0_1px_color-mix(in_oklch,var(--orange)_28%,transparent)] transition-colors hover:brightness-95"
    >
      <Ring size={14} color="var(--orange)" />
      Reconnecting
    </button>
  )
}

function IconButton({ label, onClick, children, pressed }: { label: string; onClick: () => void; children: React.ReactNode; pressed?: boolean }) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={pressed}
      title={label}
      onClick={onClick}
      className="flex size-8 items-center justify-center rounded-full bg-surface text-ink-2 shadow-hairline transition-colors hover:bg-hover hover:text-ink"
    >
      {children}
    </button>
  )
}

export function SoundToggle() {
  const on = useSound()
  return (
    <IconButton label={on ? "Mute sounds" : "Play sounds for live activity"} pressed={on} onClick={() => setSound(!on)}>
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M11 5 6 9H3v6h3l5 4z" />
        {on ? (
          <>
            <path d="M15.5 8.5a5 5 0 0 1 0 7" />
            <path d="M18.5 5.5a9 9 0 0 1 0 13" />
          </>
        ) : (
          <path d="m22 9-6 6M16 9l6 6" />
        )}
      </svg>
    </IconButton>
  )
}

export function ThemeToggle() {
  const { setTheme } = useTheme()
  const dark = typeof document !== "undefined" && document.documentElement.classList.contains("dark")
  return (
    <IconButton label={dark ? "Light mode" : "Dark mode"} onClick={() => setTheme(dark ? "light" : "dark")}>
      {dark ? (
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
      ) : (
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M20.5 14.5A8.5 8.5 0 0 1 9.5 3.5a8.5 8.5 0 1 0 11 11z" /></svg>
      )}
    </IconButton>
  )
}

export function SearchButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="hidden h-8 w-56 items-center gap-2 rounded-full bg-surface pr-1.5 pl-3 text-[12.5px] text-ink-3 shadow-hairline transition-colors hover:bg-hover md:flex"
    >
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4.3-4.3" /></svg>
      Search agents and threads
      <kbd className="ml-auto rounded-[6px] bg-field px-1.5 font-mono text-[11px] text-ink-2 shadow-hairline">/</kbd>
    </button>
  )
}
