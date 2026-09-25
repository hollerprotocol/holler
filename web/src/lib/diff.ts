import type { DiffRow } from "@/components/primitives/CodeBlock"

/** Unified diff text to CodeBlock rows. */
export function parseDiff(text: string): DiffRow[] {
  const rows: DiffRow[] = []
  let o = 1
  let c = 1
  for (const l of text.replace(/\n$/, "").split("\n")) {
    const hunk = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(l)
    if (hunk) {
      o = +hunk[1]
      c = +hunk[2]
      continue
    }
    if (l.startsWith("---") || l.startsWith("+++") || l.startsWith("diff ") || l.startsWith("index ")) continue
    if (l.startsWith("+")) rows.push({ old: null, cur: c++, type: "add", pieces: [{ text: l.slice(1) }] })
    else if (l.startsWith("-")) rows.push({ old: o++, cur: null, type: "del", pieces: [{ text: l.slice(1) }] })
    else rows.push({ old: o++, cur: c++, type: "ctx", pieces: [{ text: l.startsWith(" ") ? l.slice(1) : l }] })
  }
  return rows
}

export function looksLikeDiff(text: string, lang?: string): boolean {
  if (lang === "diff" || lang === "patch") return true
  const ls = text.split("\n")
  return ls.some((l) => l.startsWith("@@ ")) && ls.some((l) => /^[+-]/.test(l))
}
