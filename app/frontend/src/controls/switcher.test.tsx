// The switcher panel, its grid arithmetic, and the overlay that draws it.
//
// Three things are asserted, and the first is the one the rest depend on:
//
//   1. THE GRID ARITHMETIC, in the reference's own six groups — A one row,
//      B wrapping, C right-to-left, D tile size, E centering, F the auto size.
//      Numbers in, numbers out: no screen, no DOM. This is the split the
//      reference's own header claims for TileGridLayout
//      (TileGridLayoutSpecs.md:17-18), and the reason a wrap bug can be pinned
//      here at all — the panel below lays the tiles out through this function,
//      so a wrong wrap is a wrong panel.
//
//   2. THE PANEL, against the rows the daemon serves: one tile per window row,
//      each named in words, the daemon's own selection marked IN WORDS beside
//      the tile rather than by colour alone (common.tsx:60-68), a click raising
//      the window it names, and a refused Windows() saying so in an EmptyState
//      instead of leaving a blank section.
//
//   3. THE OVERLAY ENTRY (src/switcher.tsx): a wait that answers triggered
//      draws the same panel and shows the window, and a wait that answers
//      triggered=false does not show it. A spent budget is the daemon's normal
//      answer on an idle machine, so treating it as a failure would break the
//      switcher on every machine nobody is switching on.
//
// Fixtures use invented window ids on purpose: a page id is daemon vocabulary
// (§3.6c), and pasting a real one into a fixture would put a page id back into
// src/ — the exact thing test/shell.test.tsx forbids.

import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ServiceApi } from '../types/controls'
import type { Control } from '../types/controls'
import { renderControl, type ControlContext } from './index'
import { SWITCHER_FOCUS, SWITCHER_WAIT } from './actions'
import { centeringOffsets, firstSizeThatFits, tileGrid } from './SwitcherPanelControl'

afterEach(() => cleanup())

// --- the grid, pinned at any width, any tile size, either direction ----------

/** The reference's own test constants: a 5pt gap and a 100pt tile. */
const PADDING = 5
const TILE_HEIGHT = 100

function layout(
  widths: number[],
  widthMax: number,
  over: { tileHeight?: number; isLeftToRight?: boolean } = {},
) {
  return tileGrid({
    widths,
    tileHeight: over.tileHeight ?? TILE_HEIGHT,
    widthMax,
    padding: PADDING,
    isLeftToRight: over.isLeftToRight ?? true,
  })
}

describe('A. one row', () => {
  it('advances each tile by its own width plus the padding', () => {
    // TileGridLayoutTests.testTilesAdvanceByTheirOwnWidthPlusPadding
    const result = layout([40, 40], 100)
    expect(result.origins).toEqual([
      { x: 5, y: 5 },
      { x: 50, y: 5 },
    ])
    expect(result.rows).toEqual([[0, 1]])
    expect(result.maxX).toBe(95)
    expect(result.maxY).toBe(110)
  })

  it('reserves one row for an empty grid, because the panel does', () => {
    // testEmptyGridStillReservesOneRow
    const result = layout([], 100)
    expect(result.origins).toEqual([])
    expect(result.rows).toEqual([[]])
    expect(result.maxX).toBe(0)
    expect(result.maxY).toBe(110)
  })

  it('does not wrap a tile whose far edge lands exactly on the max width', () => {
    // testSingleTileFillsTheGridExactly
    const result = layout([90], 100)
    expect(result.rows).toEqual([[0]])
    expect(result.maxX).toBe(100)
  })
})

describe('B. wrapping', () => {
  it('starts a new row when the next tile would cross the edge', () => {
    // testATileThatWouldCrossTheEdgeStartsANewRow
    const result = layout([40, 40, 40], 100)
    expect(result.origins).toEqual([
      { x: 5, y: 5 },
      { x: 50, y: 5 },
      { x: 5, y: 110 },
    ])
    expect(result.rows).toEqual([[0, 1], [2]])
    expect(result.maxX).toBe(95)
    expect(result.maxY).toBe(215)
  })

  it('spaces every row one tile height plus the padding below the last', () => {
    // testEachRowIsOneTileHeightPlusPaddingBelowTheLast
    const result = layout([90, 90, 90, 90], 100)
    expect(result.origins.map((origin) => origin.y)).toEqual([5, 110, 215, 320])
    expect(result.rows).toEqual([[0], [1], [2], [3]])
    expect(result.maxY).toBe(425)
  })

  it('leaves row 0 empty and maxX at 0 for a tile wider than the grid', () => {
    // testSingleTileWiderThanTheGrid. The pinned exception to "a tile that opens
    // a row does not widen the grid": a first tile wider than the whole grid
    // wraps on the first step and the panel keeps its own minimum width.
    const result = layout([200], 100)
    expect(result.rows).toEqual([[], [0]])
    expect(result.origins).toEqual([{ x: 5, y: 110 }])
    expect(result.maxX).toBe(0)
  })
})

describe('C. right to left', () => {
  it('places tiles from the right edge, with the same rows and totals', () => {
    // testRtlPlacesTilesFromTheRightEdge
    const result = layout([40, 40, 40], 100, { isLeftToRight: false })
    expect(result.origins).toEqual([
      { x: 55, y: 5 },
      { x: 10, y: 5 },
      { x: 55, y: 110 },
    ])
    expect(result.rows).toEqual([[0, 1], [2]])
    expect(result.maxX).toBe(95)
    expect(result.maxY).toBe(215)
  })

  it('wraps on the leading edge, and a tile ending exactly at zero fits', () => {
    // testRtlWrapsWhenTheLeadingEdgeWouldPassZero — the wrap edge is the LEFT
    // one going right to left, so the boundary is 0 rather than widthMax.
    expect(layout([90], 100, { isLeftToRight: false }).rows).toEqual([[0]])
    expect(layout([90], 100, { isLeftToRight: false }).origins).toEqual([{ x: 5, y: 5 }])
    expect(layout([110], 100, { isLeftToRight: false }).rows).toEqual([[], [0]])
  })
})

describe('D. tile size', () => {
  it('moves every row down by the difference a taller tile makes', () => {
    // testATallerTileMovesEveryRowDownByTheDifference — the shape of alt-tab
    // #6010, where a three-line window title inflated every tile's height.
    const normal = layout([40, 40, 40], 100)
    const inflated = layout([40, 40, 40], 100, { tileHeight: TILE_HEIGHT + 38 })
    expect(inflated.origins[2].y - normal.origins[2].y).toBe(38)
    expect(inflated.maxY - normal.maxY).toBe(76)
    expect(inflated.rows).toEqual(normal.rows)
  })

  it('wraps sooner in a narrower grid, and fewer-per-row with wider tiles', () => {
    // testANarrowerGridWrapsSooner · testWiderTilesFitFewerPerRow
    expect(layout([40, 40, 40], 140).rows.length).toBe(1)
    expect(layout([40, 40, 40], 100).rows.length).toBe(2)
    expect(layout([40, 40, 40], 50).rows.length).toBe(3)
    expect(layout([20, 20, 20, 20], 120).rows).toEqual([[0, 1, 2, 3]])
    expect(layout([45, 45, 45, 45], 120).rows).toEqual([[0, 1], [2, 3]])
  })
})

describe('E. centering', () => {
  it('does not move a full row', () => {
    // testAFullRowIsNotMoved
    expect(centeringOffsets([[40, 40]], PADDING, 95)).toEqual([0])
  })

  it('centers a short row, with the half pixel rounding away from zero', () => {
    // testAShortRowIsCentered — (95 - 50) / 2 is 22.5, and the reference rounds
    // it to 23 rather than to 22 or to banker's 22.
    expect(centeringOffsets([[40, 40], [40]], PADDING, 95)).toEqual([0, 23])
  })

  it('gives the empty row no offset at all', () => {
    // testAnEmptyRowGetsNoOffset
    expect(centeringOffsets([[], [40]], PADDING, 95)).toEqual([0, 23])
  })

  it('never pulls a row wider than the space back off the edge', () => {
    // testARowWiderThanTheSpaceIsNotPulledBack
    expect(centeringOffsets([[400]], PADDING, 95)).toEqual([0])
  })
})

describe('F. the auto size', () => {
  const SIZES = [
    { name: 'large', height: 200 },
    { name: 'medium', height: 100 },
    { name: 'small', height: 50 },
  ]

  it('takes the first size that fits, and measures nothing after it', () => {
    // testAutoTakesTheFirstSizeThatFits
    const measured: string[] = []
    const picked = firstSizeThatFits(SIZES, 300, (size) => {
      measured.push(size.name)
      return size.height
    })
    expect(picked?.name).toBe('large')
    expect(measured).toEqual(['large'])
  })

  it('falls through to the next size, measuring only up to the one it keeps', () => {
    // testAutoFallsThroughToTheNextSize
    const measured: string[] = []
    const picked = firstSizeThatFits(SIZES, 150, (size) => {
      measured.push(size.name)
      return size.height
    })
    expect(picked?.name).toBe('medium')
    expect(measured).toEqual(['large', 'medium'])
  })

  it('keeps the smallest when nothing fits, having measured every candidate', () => {
    // testAutoKeepsTheSmallestWhenNothingFits · testAutoMeasuresEachCandidateExactlyOnce.
    // The order and the count are part of the contract: in the reference the
    // measure closure applies the size to the appearance, so the appearance is
    // left on whatever was measured last and that must be the size returned.
    const measured: string[] = []
    const picked = firstSizeThatFits(SIZES, 10, (size) => {
      measured.push(size.name)
      return size.height
    })
    expect(picked?.name).toBe('small')
    expect(measured).toEqual(['large', 'medium', 'small'])
  })
})

// --- the panel ---------------------------------------------------------------

interface Row {
  window_id: string
  app_id: string
  title: string
  index: number
  selected: boolean
  skippable: boolean
}

/** One window as the daemon's switcher row carries it, with every field named. */
function row(fields: Partial<Row> = {}): Row {
  return {
    window_id: '412',
    app_id: 'com.apple.finder',
    title: 'Downloads',
    index: 0,
    selected: false,
    skippable: false,
    ...fields,
  }
}

/** The stubbed daemon. A call is recorded by name AND its payload kept, so a
 *  test can assert which write ran and WHICH window it named. */
function daemon(over: { rows?: Row[]; failOn?: string; why?: string } = {}): ServiceApi & {
  calls: string[]
  focused: string[]
} {
  const calls: string[] = []
  const focused: string[] = []
  const rows = over.rows ?? []
  const record = (name: string, answer: () => unknown) => (...args: unknown[]) => {
    calls.push(name)
    if (name === 'SwitcherFocus') focused.push(String(args[0]))
    if (name === over.failOn) return Promise.reject(new Error(over.why ?? 'refused'))
    return Promise.resolve(answer())
  }
  return {
    calls,
    focused,
    Windows: record('Windows', () => rows),
    SwitcherWait: record('SwitcherWait', () => ({ triggered: false })),
    SwitcherFocus: record('SwitcherFocus', () => ({ focused: true })),
  } as unknown as ServiceApi & { calls: string[]; focused: string[] }
}

function context(service: ServiceApi, over: Partial<ControlContext> = {}): ControlContext {
  return {
    service,
    status: null,
    logs: [],
    refreshToken: 0,
    note: () => {},
    refresh: () => {},
    pageId: '',
    ...over,
  }
}

/** The control as the page declares it: the kind, the label, and the row write
 *  as the action id the daemon checks. */
const CONTROL: Control = {
  kind: 'switcherPanel',
  id: 'probe',
  label: 'Open windows',
  rowAction: SWITCHER_FOCUS,
  actions: [SWITCHER_WAIT],
}

/** One act() turn, enough for a useResource load to land. */
async function oneTurn(): Promise<void> {
  await act(async () => {
    await Promise.resolve()
  })
}

function panel(control: Control, service: ServiceApi, over: Partial<ControlContext> = {}) {
  return render(<>{renderControl(control, context(service, over))}</>)
}

describe('the panel', () => {
  it('draws one tile per window row, named in words', async () => {
    const { container } = panel(
      CONTROL,
      daemon({
        rows: [
          row({ window_id: '412', title: 'Downloads' }),
          row({ window_id: '881', app_id: 'com.apple.Terminal', title: 'crossos - zsh' }),
          // The row a machine that cannot name a window sends: the tile falls
          // back to the app rather than to the id, so no tile is identified by
          // its key alone.
          row({ window_id: '207', app_id: 'com.apple.Safari', title: '' }),
        ],
      }),
    )
    await oneTurn()

    const tiles = container.querySelectorAll('.ctl-tile')
    expect(tiles.length).toBe(3)
    expect(Array.from(tiles).map((tile) => tile.querySelector('.ctl-tile-name')?.textContent)).toEqual([
      'Downloads',
      'crossos - zsh',
      'com.apple.Safari',
    ])
    // Three names and no key on screen: a window whose only label were its id
    // would tell a person nothing they can act on.
    expect(container.textContent).not.toContain('412')
  })

  it("marks the daemon's selected row in words beside the tile", async () => {
    const { container } = panel(
      CONTROL,
      daemon({
        rows: [
          row({ window_id: '412', selected: false }),
          row({ window_id: '881', selected: true }),
          row({ window_id: '207', selected: false }),
        ],
      }),
    )
    await oneTurn()

    // Exactly one tile carries the word, and it is the tile the daemon picked.
    const marks = screen.getAllByText('Selected')
    expect(marks.length).toBe(1)
    const marked = marks[0].closest('.ctl-tile')
    expect(marked?.querySelector('.ctl-tile-name')?.textContent).toBe('Downloads')
    expect(marked?.getAttribute('aria-current')).toBe('true')
    // The word is inside the tile, not only a border colour: a colour-blind
    // reader and a screen reader both get the highlight (common.tsx:60-68).
    expect(container.querySelectorAll('.ctl-tile.is-selected').length).toBe(1)
  })

  it('says Skippable in words too, because it is a fact about the pick', async () => {
    panel(CONTROL, daemon({ rows: [row({ skippable: true })] }))
    await oneTurn()
    expect(screen.getByText('Skippable')).toBeTruthy()
  })

  it('raises the window a tile names, through the write the page declared', async () => {
    const service = daemon({ rows: [row({ window_id: '881' })] })
    const { container } = panel(CONTROL, service)
    await oneTurn()

    fireEvent.click(container.querySelector('.ctl-tile') as HTMLElement)
    await waitFor(() => expect(service.focused).toEqual(['881']))
    expect(service.calls).toContain('SwitcherFocus')
  })

  it('names a refused read in an EmptyState rather than leaving a blank', async () => {
    // "The daemon has nothing" and "the daemon is gone" must never look alike:
    // the empty state below says which one this is, and the error row carries
    // the daemon's own words.
    const { container } = panel(CONTROL, daemon({ failOn: 'Windows', why: 'no window server' }))
    await oneTurn()

    const empty = container.querySelector('.ctl-empty')
    expect(empty?.textContent).toContain('no window server')
    expect(container.querySelector('.ctl-error')?.textContent).toContain('no window server')
    expect(container.querySelectorAll('.ctl-tile').length).toBe(0)
  })

  it('says there is nothing to switch to when the list is simply empty', async () => {
    const { container } = panel(CONTROL, daemon({ rows: [] }))
    await oneTurn()
    expect(container.querySelector('.ctl-empty')?.textContent).toMatch(/no windows are open/i)
    expect(container.querySelector('.ctl-error')).toBeNull()
  })

  it('reports a write the daemon refused instead of leaving the tile looking raised', async () => {
    const noted: string[] = []
    const service = daemon({ rows: [row()], failOn: 'SwitcherFocus', why: 'window is gone' })
    const { container } = panel(CONTROL, service, { note: (message) => noted.push(message) })
    await oneTurn()

    fireEvent.click(container.querySelector('.ctl-tile') as HTMLElement)
    await waitFor(() => expect(container.querySelector('.ctl-error')?.textContent).toContain('window is gone'))
    expect(noted.join(' ')).toContain('window is gone')
  })

  it('lays the tiles out at the widths the browser reports, not at a guess', async () => {
    // The shell never invents a tile's width: the grid is computed over what
    // the DOM measured, and a first render that pinned a width would make the
    // measurement read that width back. jsdom lays nothing out, so offsetWidth
    // is stubbed here — which is also the only way this two-pass path gets
    // exercised at all.
    const widths: Record<string, number> = { Downloads: 300, Notes: 300, Mail: 300 }
    const own = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetWidth')
    Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
      configurable: true,
      get(this: HTMLElement) {
        if (!this.hasAttribute('data-tile')) return 0
        return widths[this.querySelector('.ctl-tile-name')?.textContent ?? ''] ?? 0
      },
    })
    try {
      const { container } = panel(
        CONTROL,
        daemon({
          rows: [
            row({ window_id: '412', title: 'Downloads' }),
            row({ window_id: '881', title: 'Notes' }),
            row({ window_id: '207', title: 'Mail' }),
          ],
        }),
      )
      await oneTurn()

      const tiles = Array.from(container.querySelectorAll<HTMLElement>('.ctl-tile'))
      // 8 + 300 + 8 + 300 fits inside the 720px panel, and a third 300pt tile
      // would cross it — so the third wraps, at the starting x of its row.
      expect(tiles.map((tile) => tile.style.width)).toEqual(['300px', '300px', '300px'])
      expect(tiles.map((tile) => tile.style.left)).toEqual(['8px', '316px', '8px'])
      const first = Number.parseFloat(tiles[0].style.top)
      const third = Number.parseFloat(tiles[2].style.top)
      expect(third).toBeGreaterThan(first)
    } finally {
      if (own) Object.defineProperty(HTMLElement.prototype, 'offsetWidth', own)
      else delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetWidth
    }
  })

  it('refuses a tile the page declared no write for, naming the gap', async () => {
    const noted: string[] = []
    const { container } = panel({ ...CONTROL, rowAction: '' }, daemon({ rows: [row()] }), {
      note: (message) => noted.push(message),
    })
    await oneTurn()

    fireEvent.click(container.querySelector('.ctl-tile') as HTMLElement)
    await waitFor(() => expect(container.querySelector('.ctl-error')?.textContent).toMatch(/no command for/))
    expect(noted.length).toBe(1)
  })
})

// --- the overlay entry -------------------------------------------------------

/**
 * The overlay's bridge, built here rather than in the shared fixtures because
 * the question this asks is the WAIT's answer — a sequence of them, one per
 * poll — and the fixture's own machine is a settings window, not a switcher.
 *
 * The answers are SCRIPTED rather than a rule, because the two things worth
 * pinning are order-dependent: that a spent budget shows nothing and keeps the
 * loop alive, and that the chord which does arrive shows the window and leaves
 * the loop alive too.
 */
const bridge = vi.hoisted(() => ({
  /** One answer per poll, in order. An exhausted script answers untriggered. */
  answers: [] as { triggered: boolean; action?: string; window_id?: string }[],
  asked: 0,
  budgets: [] as unknown[],
  shown: 0,
  windows: [] as unknown[],
}))

vi.mock('../lib/service', () => {
  const fake = {
    Windows: () => Promise.resolve(bridge.windows),
    SwitcherWait: (timeoutMs: number) => {
      bridge.asked += 1
      bridge.budgets.push(timeoutMs)
      return Promise.resolve(bridge.answers[bridge.asked - 1] ?? { triggered: false })
    },
    SwitcherFocus: () => Promise.resolve({ focused: true }),
  }
  return { Service: fake, service: fake }
})

vi.mock('@wailsio/runtime', () => ({
  Window: {
    Show: () => {
      bridge.shown += 1
      return Promise.resolve()
    },
  },
}))

function script(answers: { triggered: boolean; action?: string; window_id?: string }[]): void {
  bridge.answers = answers
  bridge.asked = 0
  bridge.budgets.length = 0
  bridge.shown = 0
}

async function overlay() {
  const { SwitcherOverlay } = await import('../switcher')
  return render(<SwitcherOverlay />)
}

describe('the overlay entry', () => {
  it('shows the window and draws the panel when the chord arrives', async () => {
    script([{ triggered: false }, { triggered: true, action: 'summon' }])
    bridge.windows = [row({ window_id: '412', title: 'Downloads' })]

    const { container } = await overlay()
    // The first wait is the idle machine's answer: nothing happened, so the
    // window is NOT shown and the panel is not drawn.
    await waitFor(() => expect(bridge.asked).toBe(1))
    expect(bridge.shown).toBe(0)
    expect(container.querySelectorAll('.ctl-tile').length).toBe(0)

    await waitFor(() => expect(bridge.shown).toBe(1))
    await waitFor(() => expect(container.querySelectorAll('.ctl-tile').length).toBe(1))
    // The SAME panel the page draws — the overlay declares a control, it does
    // not carry a second tile renderer.
    expect(container.querySelector('section.ctl')?.getAttribute('aria-labelledby')).toBeTruthy()
    expect(container.textContent).toContain('Downloads')
  })

  it('keeps waiting after the chord, because the next chord is the next answer', async () => {
    script([{ triggered: false }, { triggered: true, action: 'summon' }])
    await overlay()
    await waitFor(() => expect(bridge.shown).toBe(1))
    // An overlay that stopped polling after the first trigger would open once
    // per window rather than once per chord.
    await waitFor(() => expect(bridge.asked).toBeGreaterThan(2))
    expect(bridge.shown).toBe(1)
  })

  it('shows nothing while the waits keep saying nothing happened', async () => {
    script([{ triggered: false }, { triggered: false }])
    await overlay()
    await waitFor(() => expect(bridge.asked).toBeGreaterThan(1))
    // A spent budget is the daemon's normal answer on an idle machine, so
    // treating it as a failure — or as a summon — would break the switcher on
    // every machine nobody is switching on.
    expect(bridge.shown).toBe(0)
  })

  it('asks for the daemon default budget, so it carries no copy of the cap', async () => {
    script([{ triggered: false }])
    await overlay()
    await waitFor(() => expect(bridge.asked).toBeGreaterThan(0))
    // Zero is the daemon's own "use the default" (the daemon's waitBudget):
    // the cap and the default are its numbers, and a second copy here would be
    // one more place they could drift.
    expect(bridge.budgets.every((budget) => budget === 0)).toBe(true)
  })

  it('stops waiting when it is torn down', async () => {
    script([{ triggered: false }])
    const { unmount } = await overlay()
    await waitFor(() => expect(bridge.asked).toBeGreaterThan(0))
    const asked = bridge.asked
    unmount()
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 120))
    })
    // A loop that outlives its component would go on asking a daemon about a
    // window nobody is looking at.
    expect(bridge.asked).toBe(asked)
  })
})
