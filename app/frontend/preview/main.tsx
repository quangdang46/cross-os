import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from '../src/App'
import { grantAccessibility, machine, press, reset } from '../src/test/fixtures'

// ?flat=1 drops the @supports (color: AccentColor) block, which Chrome reports
// as SUPPORTED while resolving none of its keywords — the whole palette
// collapses to the initial value there. Turning it off is how the designed hex
// palette is seen from a browser that cannot honour the hand-off.
// ?dark=1 forces the dark media query to match, so the two appearances can be
// compared from one machine that is set to light.
const q = new URLSearchParams(location.search)
if (q.get('flat') || q.get('dark')) {
  let css = await fetch('/style.css').then((r) => r.text())
  if (q.get('flat')) {
    css = css.replace(/@supports \(color: AccentColor\)/g, '@supports (color: NoSuchKeyword)')
  }
  if (q.get('dark')) {
    css = css.replace(/@media \(prefers-color-scheme: dark\)/g, '@media (min-width: 1px)')
  }
  if (q.get('comfortable')) {
    css += '\n:root { }\n:root, :root[data-density] { }\n'
    css = css.replace(':root[data-density=\'comfortable\']', ':root')
  }
  if (q.get('contrast')) {
    css = css.replace(/@media \(prefers-contrast: more\)/g, '@media (min-width: 1px)')
  }
  const tag = document.createElement('style')
  tag.textContent = css
  document.head.appendChild(tag)
}

// ?state=ready  a fully set-up machine (daemon ok, tap in, no callout)
// ?state=fresh  the first-run machine (tap refused -> permission callout)
// ?state=partial  the tap is in but no extension is on (wizard mid-flight)
const state = new URLSearchParams(location.search).get('state') ?? 'ready'

if (state === 'ready') {
  machine.interception = true
  machine.profile = 'windows11'
  machine.extensions['window-keys'] = true
  machine.extensions['finder-actions'] = true
  machine.onboarded = true
} else if (state === 'partial') {
  machine.profile = 'windows11'
  machine.extensions['window-keys'] = true
  machine.onboarded = true
} else {
  reset()
}

if (state === 'ready') grantAccessibility(0)
press('Ctrl+Tab')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

// ?page=<id> lands on a page other than the first, so a screenshot of any one
// of the served pages is one URL away. The click is the same one a person makes
// on the rail — the shell exposes no route for a preview to jump through.
const params = new URLSearchParams(location.search)
const want = params.get('page')
if (want) {
  setTimeout(() => {
    const rail = Array.from(document.querySelectorAll<HTMLButtonElement>('.section'))
    const hit = rail.find(
      (b) => b.querySelector('.section-label')?.textContent?.trim() === want,
    )
    if (!hit) console.warn('preview: no nav page named', want)
    hit?.click()
  }, 400)
}

// ?focus=<selector> puts the keyboard where a keyboard user's would be, so the
// focus ring can be looked at rather than assumed.
const focusTarget = params.get('focus')
if (focusTarget) {
  setTimeout(() => {
    const el = document.querySelector<HTMLElement>(focusTarget)
    el?.focus()
  }, 600)
}

// ?measure=1 reports the geometry of every control row, so "the edges do not
// line up" is a number rather than an impression. The report is written into
// the document so a headless --dump-dom run can read it back.
if (params.get('measure')) {
  setTimeout(() => {
    const round = (n: number) => Math.round(n * 10) / 10
    const lines: string[] = []
    for (const row of Array.from(document.querySelectorAll<HTMLElement>('.ctl'))) {
      const box = row.getBoundingClientRect()
      const style = getComputedStyle(row)
      const head = row.querySelector('.ctl-head')
      const label = row.querySelector('.ctl-label')
      const name = (label?.textContent ?? head?.textContent ?? '?').trim().slice(0, 22)
      const kids = Array.from(row.children)
        .map((k) => {
          const r = k.getBoundingClientRect()
          return `${k.className || k.tagName}@${round(r.left - box.left)}w${round(r.width)}`
        })
        .join('  ')
      lines.push(
        `row h=${round(box.height)} cols=${style.gridTemplateColumns} | ${name} | ${kids}`,
      )
    }
    // ?kind-measure: the same read for a control's own children, which is
    // where the rhythm inside one row actually lives.
    if (params.get('kind')) {
      const sel = params.get('kind') as string
      for (const el of Array.from(document.querySelectorAll<HTMLElement>(sel))) {
        const r = el.getBoundingClientRect()
        const cs = getComputedStyle(el)
        lines.push(
          `${sel} h=${round(r.height)} top=${round(r.top)} lh=${cs.lineHeight} ` +
          `mt=${cs.marginTop} mb=${cs.marginBottom} "${(el.textContent ?? '').trim().slice(0, 28)}"`,
        )
      }
    }
    const pre = document.createElement('pre')
    pre.id = 'measure'
    pre.textContent = lines.join('\n')
    document.body.appendChild(pre)
  }, 900)
}

// ?audit=1 reports the palette actually in force — every distinct background,
// ink, radius and font-size the page paints with. A design system that has not
// been applied shows up here as a long tail of one-off values.
if (params.get('audit')) {
  setTimeout(() => {
    const tally = (pick: (s: CSSStyleDeclaration) => string) => {
      const counts = new Map<string, Map<string, number>>()
      for (const el of Array.from(document.querySelectorAll<HTMLElement>('.window *'))) {
        const s = getComputedStyle(el)
        const v = pick(s)
        if (!v || v === 'none' || v === 'rgba(0, 0, 0, 0)' || v === 'normal') continue
        const who = counts.get(v) ?? new Map<string, number>()
        who.set(el.className || el.tagName, (who.get(el.className || el.tagName) ?? 0) + 1)
        counts.set(v, who)
      }
      return [...counts.entries()]
        .sort((a, b) => [...b[1].values()].reduce((x, y) => x + y, 0) - [...a[1].values()].reduce((x, y) => x + y, 0))
        .map(([v, who]) => `${v}  <-  ${[...who].map(([k, n]) => `${k}(${n})`).join(', ')}`)
    }
    const out = [
      '== background ==',
      ...tally((s) => s.backgroundColor),
      '== color (ink) ==',
      ...tally((s) => s.color).map(([v, n]) => `${v}\t${n}`),
      '== border-color ==',
      ...tally((s) => s.borderTopColor).map(([v, n]) => `${v}\t${n}`),
      '== border-radius ==',
      ...tally((s) => s.borderTopLeftRadius).map(([v, n]) => `${v}\t${n}`),
      '== font-size ==',
      ...tally((s) => s.fontSize).map(([v, n]) => `${v}\t${n}`),
      '== font-weight ==',
      ...tally((s) => s.fontWeight).map(([v, n]) => `${v}\t${n}`),
    ].join('\n')
    const pre = document.createElement('pre')
    pre.id = 'audit'
    pre.textContent = out
    document.body.appendChild(pre)
  }, 900)
}
