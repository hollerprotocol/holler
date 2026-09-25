import { lazy, Suspense, useEffect, useMemo, useState } from "react"

import LoadingState from "@/components/primitives/LoadingState"
import { AgentList } from "@/components/holler/AgentList"
import { ActivityFeed } from "@/components/holler/ActivityFeed"
import { EmptyNetwork } from "@/components/holler/EmptyNetwork"
import { Connection, SearchButton, SoundToggle, ThemeToggle, Wordmark } from "@/components/holler/Header"
import { NetworkGraph } from "@/components/holler/NetworkGraph"
import { Palette } from "@/components/holler/Palette"
import { Sheet } from "@/components/holler/Sheet"
import { Stats } from "@/components/holler/Stats"
import { ThreadList } from "@/components/holler/ThreadList"
import { agentName, plural } from "@/lib/format"
import { playFor } from "@/lib/sound"
import { getStore, useHoller } from "@/lib/store"
import type { Activity, State, Thread } from "@/lib/types"

const AgentDetail = lazy(() => import("@/components/holler/AgentDetail").then((m) => ({ default: m.AgentDetail })))
const ThreadDetail = lazy(() => import("@/components/holler/ThreadDetail").then((m) => ({ default: m.ThreadDetail })))

type Selection = { kind: "agent"; key: string } | { kind: "thread"; id: string; fallback?: Thread } | undefined

function Card({ title, sub, action, children, className = "" }: { title?: string; sub?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return (
    <section className={`min-w-0 rounded-[18px] bg-surface shadow-card ${className}`}>
      {title && (
        <div className="flex items-start justify-between gap-3 px-4 pt-4 pb-2">
          <div className="min-w-0">
            <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-ink">{title}</h2>
            {sub && <p className="text-[12px] text-ink-3">{sub}</p>}
          </div>
          {action}
        </div>
      )}
      {children}
    </section>
  )
}

function headline(state: State): { title: string; sub: string } {
  const others = state.agents.length - 1
  const working = state.agents.filter((a) => a.working).length
  if (others <= 0) return { title: "Waiting for the network", sub: `Watching from ${state.host_name || "this host"}` }
  const title = working ? `${plural(state.agents.length, "agent")}, ${working} at work` : `${plural(state.agents.length, "agent")}, all quiet`
  return { title, sub: `Live from ${state.host_name || "this host"} and every agent its presence gossip reaches` }
}

function threadFor(state: State | undefined, a: Activity): Thread | undefined {
  if (!state || !a.th) return undefined
  const pair = [a.from, a.to ?? ""].sort()
  return (
    state.threads.find((t) => t.th === a.th && t.a === pair[0] && t.b === pair[1]) ??
    state.threads.find((t) => t.th === a.th && (t.a === a.from || t.b === a.from))
  )
}

export function App() {
  const { state, activity, conn, error } = useHoller()
  const [sel, setSel] = useState<Selection>()
  const [palette, setPalette] = useState(false)

  useEffect(() => getStore().onActivity(playFor), [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (t?.closest("input, textarea, [contenteditable='true']")) return
      if ((e.key === "/" && !e.metaKey && !e.ctrlKey) || (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey))) {
        e.preventDefault()
        setPalette(true)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const agents = useMemo(() => new Map((state?.agents ?? []).map((a) => [a.key, a])), [state?.agents])

  useEffect(() => {
    if (!state) return
    const w = state.stats.working
    document.title = w ? `(${w}) holler · ${state.host_name}` : `holler · ${state.host_name}`
  }, [state])

  const openAgent = (key: string) => setSel({ kind: "agent", key })
  const openThread = (t: Thread) => setSel({ kind: "thread", id: t.id, fallback: t })
  const openActivity = (a: Activity) => {
    const t = threadFor(state, a)
    if (t) openThread(t)
    else openAgent(a.from)
  }

  const selAgent = sel?.kind === "agent" ? agents.get(sel.key) : undefined
  const selThread = sel?.kind === "thread" ? (state?.threads.find((t) => t.id === sel.id) ?? sel.fallback) : undefined

  if (!state) {
    return (
      <div className="flex min-h-svh flex-col items-center justify-center gap-4 px-4">
        <Wordmark />
        {error ? (
          <div className="max-w-sm text-center">
            <p className="text-[14px] font-medium text-ink">Can't reach the holler daemon</p>
            <p className="mt-1 text-[12.5px] text-ink-3">{error}</p>
            <p className="mt-3 text-[12.5px] text-ink-2">
              Is it running? Start it with <code className="font-mono">holler up</code>. Retrying…
            </p>
          </div>
        ) : (
          <LoadingState label="Listening for the network" variant="Dots" />
        )}
      </div>
    )
  }

  const h = headline(state)
  const alone = state.agents.length <= 1
  const selectedKey = sel?.kind === "agent" ? sel.key : undefined

  return (
    <div className="min-h-svh">
      <header className="sticky top-0 z-30 border-b border-line/70 bg-page/80 backdrop-blur-xl backdrop-saturate-150">
        <div className="mx-auto flex h-14 max-w-[1560px] items-center gap-3 px-4 sm:px-6 lg:px-8">
          <Wordmark />
          <span className="hidden h-5 w-px bg-line-strong sm:block" />
          <span className="hidden min-w-0 truncate text-[13px] text-ink-3 sm:block">
            watching from <span className="font-medium text-ink-2">{state.host_name}</span>
          </span>
          <div className="ml-auto flex items-center gap-2">
            <SearchButton onClick={() => setPalette(true)} />
            <button
              type="button"
              aria-label="Search"
              onClick={() => setPalette(true)}
              className="flex size-8 items-center justify-center rounded-full bg-surface text-ink-2 shadow-hairline md:hidden"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round"><circle cx="11" cy="11" r="7" /><path d="M21 21l-4.3-4.3" /></svg>
            </button>
            <Connection conn={conn} />
            <SoundToggle />
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-[1560px] px-4 pt-8 pb-10 sm:px-6 lg:px-8">
        <div className="mb-7" style={{ animation: "rise-in 600ms var(--ease-out-strong) both" }}>
          <h1 className="text-[34px] leading-[1.05] font-semibold tracking-[-0.04em] text-balance text-ink sm:text-[44px]">{h.title}</h1>
          <p className="mt-2 text-[15px] text-ink-2">{h.sub}</p>
        </div>

        <Stats state={state} activity={activity} />

        <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(340px,400px)]">
          <div className="flex min-w-0 flex-col gap-4">
            <section className="relative h-[440px] min-w-0 overflow-hidden rounded-[18px] bg-surface shadow-card sm:h-[520px]">
              <div
                aria-hidden
                className="pointer-events-none absolute inset-0"
                style={{
                  background: "radial-gradient(ellipse 60% 55% at 50% 50%, var(--glow-a), transparent 70%), radial-gradient(ellipse 40% 40% at 80% 20%, var(--glow-b), transparent 70%)",
                }}
              />
              <div
                aria-hidden
                className="pointer-events-none absolute inset-0 opacity-60"
                style={{ backgroundImage: "radial-gradient(var(--line-strong) 1px, transparent 1px)", backgroundSize: "22px 22px", maskImage: "radial-gradient(ellipse at center, #000 30%, transparent 80%)" }}
              />
              <div className="absolute top-4 left-4 z-10">
                <h2 className="text-[15px] font-semibold tracking-[-0.01em] text-ink">Network</h2>
                <p className="text-[12px] text-ink-3">
                  {plural(state.agents.length, "agent")} · {plural(state.links.length, "link")}
                </p>
              </div>
              <div className="absolute bottom-3 left-4 z-10 hidden items-center gap-3 text-[11.5px] text-ink-3 sm:flex">
                <span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-green" />connected</span>
                <span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-accent" />via gossip</span>
                <span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-ink-3" />quiet</span>
              </div>
              <div className="absolute inset-0 pt-8">
                {alone ? <EmptyNetwork state={state} /> : <NetworkGraph state={state} selected={selectedKey} onSelect={openAgent} />}
              </div>
            </section>

            <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
              <Card title="Threads" sub="Every conversation on the network, with both sides' states">
                <div className="px-1.5 pb-2">
                  <ThreadList threads={state.threads} agents={agents} selected={sel?.kind === "thread" ? sel.id : undefined} onOpen={openThread} onAgent={openAgent} />
                </div>
              </Card>
              <Card title="Agents" sub={`${state.stats.up} of ${state.stats.agents} up`}>
                <div className="px-1.5 pb-2">
                  <AgentList agents={state.agents} selected={selectedKey} onSelect={openAgent} />
                </div>
              </Card>
            </div>
          </div>

          <aside className="min-w-0 lg:sticky lg:top-[72px] lg:self-start">
            <Card className="flex h-[640px] flex-col lg:h-[calc(100svh-88px)]">
              <ActivityFeed activity={activity} agents={agents} query="" onAgent={openAgent} onThread={openActivity} className="flex-1" />
            </Card>
          </aside>
        </div>

        <footer className="mt-10 flex flex-wrap items-center justify-between gap-2 text-[12px] text-ink-3">
          <span>holler {state.version}</span>
          <span>
            Press <kbd className="rounded-[5px] bg-field px-1 font-mono shadow-hairline">/</kbd> to search ·{" "}
            <kbd className="rounded-[5px] bg-field px-1 font-mono shadow-hairline">d</kbd> for dark mode
          </span>
        </footer>
      </main>

      <Sheet
        open={!!(selAgent || selThread)}
        onClose={() => setSel(undefined)}
        title={selAgent ? agentName(selAgent) : (selThread?.subject ?? "Thread")}
      >
        <Suspense
          fallback={
            <div className="flex flex-1 items-center justify-center">
              <LoadingState label="Loading" variant="Dots" />
            </div>
          }
        >
          {selAgent && <AgentDetail agent={selAgent} state={state} agents={agents} onThread={openThread} onAgent={openAgent} />}
          {selThread && <ThreadDetail key={selThread.id} thread={selThread} state={state} agents={agents} />}
        </Suspense>
      </Sheet>

      <Palette open={palette} onClose={() => setPalette(false)} state={state} agents={agents} onAgent={openAgent} onThread={openThread} />
    </div>
  )
}

export default App
