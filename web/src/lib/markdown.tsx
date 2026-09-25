// A small renderer for the markdown agents write to each other: paragraphs
// (single newlines kept), lists, headings, inline code, bold, italics,
// links, and fenced code, which goes to the CodeBlock (diffs as diffs).
import { Fragment, lazy, Suspense, type ReactNode } from "react"

import { looksLikeDiff, parseDiff } from "./diff"

const CodeBlock = lazy(() => import("@/components/primitives/CodeBlock"))

// No underscore emphasis: agents write snake_case far more often than _italics_.
const INLINE = /(`[^`]+`|\*\*[^*\n]+\*\*|(?<![\w*])\*[^*\s][^*\n]*\*(?![\w*])|\[[^\]]+\]\(https?:\/\/[^)\s]+\))/g

function inline(text: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let k = 0
  for (const m of text.matchAll(INLINE)) {
    const i = m.index ?? 0
    if (i > last) out.push(text.slice(last, i))
    const t = m[0]
    if (t.startsWith("`")) {
      out.push(
        <code key={k++} className="rounded-[5px] bg-field px-1 py-px font-mono text-[0.88em] text-ink shadow-hairline">
          {t.slice(1, -1)}
        </code>,
      )
    } else if (t.startsWith("**")) {
      out.push(<strong key={k++} className="font-semibold text-ink">{t.slice(2, -2)}</strong>)
    } else if (t.startsWith("[")) {
      const [, label, href] = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(t) ?? []
      out.push(
        <a key={k++} href={href} target="_blank" rel="noreferrer" className="text-accent-ink underline decoration-accent/30 underline-offset-2 hover:decoration-accent">
          {label}
        </a>,
      )
    } else {
      out.push(<em key={k++}>{t.slice(1, -1)}</em>)
    }
    last = i + t.length
  }
  if (last < text.length) out.push(text.slice(last))
  return out
}

function lines(ls: string[]): ReactNode {
  return ls.map((l, i) => (
    <Fragment key={i}>
      {i > 0 && <br />}
      {inline(l)}
    </Fragment>
  ))
}

const LIST = /^\s*(?:[-*•]|\d+[.)])\s+/

function prose(text: string, key: string): ReactNode[] {
  const blocks = text.split(/\n\s*\n/)
  return blocks.map((b, bi) => {
    const ls = b.replace(/^\n+|\n+$/g, "").split("\n")
    if (!ls.join("").trim()) return null
    const k = `${key}-${bi}`
    if (ls.every((l) => LIST.test(l))) {
      const ordered = /^\s*\d/.test(ls[0])
      const Tag = ordered ? "ol" : "ul"
      return (
        <Tag key={k} className={`my-1.5 space-y-0.5 pl-5 ${ordered ? "list-decimal" : "list-disc"} marker:text-ink-3`}>
          {ls.map((l, i) => (
            <li key={i}>{inline(l.replace(LIST, ""))}</li>
          ))}
        </Tag>
      )
    }
    const h = /^(#{1,4})\s+(.*)$/.exec(ls[0])
    if (h && ls.length === 1) {
      return (
        <p key={k} className="mt-2 mb-1 font-semibold text-ink">
          {inline(h[2])}
        </p>
      )
    }
    if (ls.every((l) => l.startsWith(">"))) {
      return (
        <blockquote key={k} className="my-1.5 border-l-2 border-line-strong pl-3 text-ink-2">
          {lines(ls.map((l) => l.replace(/^>\s?/, "")))}
        </blockquote>
      )
    }
    return (
      <p key={k} className="my-1 first:mt-0 last:mb-0">
        {lines(ls)}
      </p>
    )
  })
}

export function Code({ text, lang, name }: { text: string; lang?: string; name?: string }) {
  const diff = looksLikeDiff(text, lang)
  const label = name || (diff ? "diff" : lang || "code")
  return (
    <Suspense fallback={<pre className="rounded-card bg-surface p-3 font-mono text-[12.5px] shadow-card">{text}</pre>}>
      <div className="my-2 min-w-0">
        {diff ? (
          <CodeBlock variant="Diff" diff={parseDiff(text)} filename={label} code={text} />
        ) : (
          <CodeBlock variant="Code" lines={text.replace(/\n$/, "").split("\n")} filename={label} code={text} />
        )}
      </div>
    </Suspense>
  )
}

const FENCE = /```([\w+-]*)[^\n]*\n([\s\S]*?)(?:```|$)/g

export function Markdown({ text }: { text: string }) {
  const out: ReactNode[] = []
  let last = 0
  let k = 0
  for (const m of text.matchAll(FENCE)) {
    const i = m.index ?? 0
    if (i > last) out.push(...prose(text.slice(last, i), `p${k++}`))
    out.push(<Code key={`c${k++}`} text={m[2]} lang={m[1] || undefined} />)
    last = i + m[0].length
  }
  if (last < text.length) out.push(...prose(text.slice(last), `p${k}`))
  return <>{out}</>
}
