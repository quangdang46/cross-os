// The stylesheet's own contracts, asserted against public/style.css.
//
// jsdom does not load the stylesheet, so nothing in the render suite can see a
// broken grid or a token that lost its value — a card laid out with a 220px
// hole down the middle of it is green on CI and unusable on screen. These are
// the three defects that shipped that way, written down so the second one is a
// failing test rather than a screenshot somebody has to notice.

import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// vitest runs with cwd at the package root, and import.meta.url is not a file:
// URL under the jsdom environment, so the path is built from cwd rather than
// from the module.
const CSS = readFileSync(resolve(process.cwd(), 'public/style.css'), 'utf8')

/** The byte offset of a top-level at-rule or selector, for order assertions. */
function at(needle: string): number {
  const i = CSS.indexOf(needle)
  expect(i, `style.css no longer contains ${JSON.stringify(needle)}`).toBeGreaterThan(-1)
  return i
}

describe('the token cascade', () => {
  it('lets the OS answer last, in BOTH appearances', () => {
    // The defect: the @supports block sat ABOVE the dark block, and both target
    // :root at the same specificity, so source order decided. Dark mode took the
    // hardcoded --accent: #5b9bff and light mode took the user's AccentColor —
    // the app followed System Settings in one appearance and ignored it in the
    // other. Being last is the whole contract.
    expect(at('@supports (color: AccentColor)')).toBeGreaterThan(
      at('@media (prefers-color-scheme: dark)'),
    )
  })

  it('gives the card, the nested list and the rail three different surfaces', () => {
    // All three were ButtonFace, so one grey had to mean "rail", "card" and
    // "well" at once — and a card the same colour as the page behind it is not
    // a card. Checked in the bridged block, which is the one WKWebView uses.
    const bridged = CSS.slice(at('@supports (color: AccentColor)'), at('@media (prefers-contrast'))
    const surfaces = ['--bg-window', '--bg-sidebar', '--bg-raised', '--bg-sunken']
      .map((n) => bridged.match(new RegExp(`${n}:\\s*([^;]+);`))?.[1]?.trim())
    expect(surfaces.every(Boolean)).toBe(true)
    // Three DISTINCT values, not four names pointing at ButtonFace.
    expect(new Set(surfaces).size).toBeGreaterThanOrEqual(3)
  })

  it('bridges all three ink tiers to the OS', () => {
    // --fg-tertiary was never in the @supports block, so timestamps and caps
    // kept a hardcoded hex while the ink around them followed the appearance.
    const bridged = CSS.slice(at('@supports (color: AccentColor)'), at('@media (prefers-contrast'))
    for (const tier of ['--fg-primary', '--fg-secondary', '--fg-tertiary']) {
      expect(bridged, `${tier} is not bridged`).toContain(`${tier}:`)
    }
  })
})

describe('the control row', () => {
  it('has no label gutter', () => {
    // The defect: `minmax(0, 220px) minmax(0, 1fr)`, a fixed 220px label column
    // on every row. A label like "version" is ~50px, so ~170px of nothing sat
    // between the label and its value, and the value's left edge moved with the
    // longest label on the page.
    expect(CSS).not.toMatch(/grid-template-columns:[^;]*220px/)
    expect(CSS).toMatch(/\.ctl \{\s*display: grid;\s*grid-template-columns: minmax\(0, 1fr\) auto;/)
  })

  it('does not reshape the grid per row', () => {
    // The defect: `:has(> .ctl-toggle), :has(> .ctl-input), :has(> .ctl-select)`
    // inverted the WHOLE grid for a row that happened to hold a control, so a
    // card could hold a 220px-label row beside a 1fr-label row and the label
    // edge jumped between two rows meant to read as one list. Which CHILD a row
    // holds decides its column; the row's shape does not.
    expect(CSS).not.toMatch(/\.ctl:has\([^{]*\{\s*grid-template-columns/)
  })

  it('is nestable', () => {
    // SchemaFormControl emits a .ctl inside a ControlFrame. The inner one
    // re-entered the grid inside the parent's content column and came out as
    // `220px 192px` — a 220px label gutter in a 192px box.
    expect(CSS).toMatch(/\.ctl \.ctl \{/)
  })

  it('does not leave a paragraph on the UA default margin', () => {
    // .ctl-value is a bare <p>. At the UA's `1em 0` it carried 26px of margin
    // on top of the grid's row-gap, which is why a one-line "CrossOS 0.4.0"
    // measured 82.7px tall in a row declared at 34px.
    const value = CSS.match(/\.ctl-value \{[^}]*\}/)?.[0] ?? ''
    expect(value).toMatch(/margin:\s*0/)
  })
})

describe('the wizard step row', () => {
  it('gives the step name a floor it cannot be squeezed under', () => {
    // The defect: .ctl-step-button was `flex: 1` (basis 0), so it was the one
    // item on the line with nothing to lose by shrinking. The chips and the
    // reason paragraph took the space, the label was left a character or two
    // wide, and `overflow-wrap: anywhere` broke "Pick a Windows profile" into
    // one letter per line — a row 883.5px tall in a 720px window, on the first
    // screen a new user sees.
    const button = CSS.match(/\.ctl-step-button \{[^}]*\}/)?.[0] ?? ''
    expect(button).toMatch(/flex:\s*1 1 auto/)
    expect(button).not.toMatch(/flex:\s*1;/)
    expect(button).toMatch(/min-width:\s*var\(--step-name-min/)
  })
})

describe('spacing', () => {
  // The --sp-* scale plus the two half-steps 3.3 describes ("a 4px grid with
  // 4px half-steps"): 0/2/4/6/8/10/12/16/20/24/32/40/56. 6 and 10 were in the
  // file thirteen and ten times before they were named, which is a rhythm
  // nobody had written down rather than an accident.
  const onScale = new Set(['0', '2px', '4px', '8px', '12px', '16px', '20px', '24px', '32px', '40px', '56px'])

  // The ported measurements. These are numbers TAKEN from the reference
  // applications and cited in the rule they appear in, so rounding them to the
  // nearest rung would make those comments say something false about the app
  // they were measured from. An odd number that names its source is honest; an
  // odd number with no name is not. If one of these is ever changed, the
  // citation changes with it and this list loses its entry.
  const PORTED = [
    'padding: 1px var(--sp-2)',      // a chip's own vertical padding
    'padding: 1px 8px',              // ditto
    'padding: 3px 7px',              // MMField, DesignSystem.swift:451-482
    'padding: 3px 0',                // MMField, without its inline padding
    'padding: 7px var(--sp-5h)',     // PackImportSheet.swift:253-254, the step row
    'padding: var(--sp-4h) 9px',     // a list row
    'padding: 4px 9px',              // a list row, roomier
    'padding: 9px 11px',             // a list row with a hint under it
    'padding-right: 22px',           // room for a select's drawn chevron
    'row-gap: 13px',                 // the fact pane's own rhythm
    'column-gap: 11px',              // lockRow, MenuHubPanels.swift:620-633
    'gap: 11px',                     // ditto, between label, value and lock
    'margin-top: 3px',               // PvField's hint, :37-42
    'padding-left: 14px',            // --row-pad-inline, fixed by name in 3.3
    'padding-right: 14px',           // ditto
    'padding-top: 14px',             // ditto
  ]

  it('has no spacing value that is neither on the scale nor a cited measurement', () => {
    // A hairline is not spacing: the file draws control boundaries at 0.5px and
    // dividers at 1px throughout, and those are strokes rather than steps in a
    // rhythm. A declaration that names a line style is a border, whatever
    // property it is written under — `padding: 3px 7px` sitting beside
    // `border: 1px solid` in the same rule must not be read twice.
    const SPACING = /^(padding|margin|gap|row-gap|column-gap|top|left|right|bottom|inset)/
    const IS_BORDER = /(^|\s)(solid|dashed|dotted|double|none)(\s|$)/
    // `(?<![\d.])` so 0.5px is a hairline and not the number 5.
    const PX = /(?<![\d.])(\d+px)/g

    const offenders: string[] = []
    for (const decl of CSS.replace(/\/\*[\s\S]*?\*\//g, '').split(/[;{}]/)) {
      const colon = decl.indexOf(':')
      if (colon < 0) continue
      const prop = decl.slice(0, colon).trim()
      const value = decl.slice(colon + 1).trim()
      if (!SPACING.test(prop) || IS_BORDER.test(value)) continue
      if (value.includes('--sp-4h') || value.includes('--sp-5h')) continue
      if (PORTED.includes(`${prop}: ${value}`)) continue
      for (const m of value.matchAll(PX)) {
        if (!onScale.has(m[1])) offenders.push(`${prop}: ${value}`)
      }
    }
    // A value that has joined PORTED must have said where it came from, so the
    // list cannot grow into a dumping ground.
    for (const entry of PORTED) {
      expect(CSS, `${entry} is exempted but no longer in the file`).toContain(entry)
    }
    expect(offenders, `unnamed off-scale spacing:\n  ${[...new Set(offenders)].join('\n  ')}`).toEqual([])
  })
})
