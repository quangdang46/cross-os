// The switcher panel: the windows the switcher can raise, one tile each.
//
// A settings page is a scrolling column, so this is the first control whose
// layout is a GRID rather than a list — and the grid is why the panel is its
// own kind. The daemon decides the order and which row is highlighted; the
// panel lays the rows out and re-decides neither.
//
// Port source: alt-tab-macos (lwouis/alt-tab-macos), whose tile grid is the
// closest analog to a page of daemon-served windows. What transfers is the
// ARITHMETIC — where a tile goes, when a row wraps, how far a short row moves
// to be centered, and which tile height the auto setting settles on —
// reimplemented in TypeScript over the same five inputs:
//
//   tmp/research/alt-tab-macos/src/switcher/main-window/TileGridLayoutSpecs.md:5-49
//     :5-8    every tile the same height and its own width, placed along the
//             writing direction until the next would cross the panel's max width
//     :28-31  a tile wraps on its FAR edge, measured with the padding that
//             follows it; both the projection and the row's y are floored, so
//             half a pixel of accumulated rounding cannot push a fitting row over
//     :32-37  maxY always counts one row even with no tiles (the panel reserves
//             a row before it knows what is in it), and a tile that OPENS a row
//             does not widen the grid
//     :38-39  rows always holds at least one row, and a first tile wider than
//             the grid leaves row 0 empty
//     :40-46  a row is centered only when narrower than the space it is given;
//             offsets round away from zero and are never negative; centering is
//             measured against a width the CALLER chooses, not against maxX
//     :47-50  the auto size takes the first that fits and the last regardless,
//             measuring each candidate up to and including the chosen one
//   tmp/research/alt-tab-macos/src/switcher/main-window/TileGridLayoutTests.swift
//     the six groups — A one row, B wrapping, C right-to-left, D tile size,
//     E centering, F the auto size — mirrored one for one in switcher.test.tsx
//   tmp/research/alt-tab-macos/src/switcher/state/SelectionResolverSpecs.md:44-50
//     the decision priority order: which tile is highlighted is the daemon's
//     answer, arrived at over a refresh the shell does not see, so the panel
//     draws Selected where the row says so and resolves nothing itself
//
// No Swift was copied. The reference measures a tile by filling an NSView inside
// a flipped document view; here the arithmetic is a pure function over the widths
// the DOM reports, which is the split the reference's own header claims for it
// (TileGridLayoutSpecs.md:17-18, "the grid can now be pinned at any width, any
// tile size, either writing direction and any number of tiles, without a
// screen"). The reference is GPL-3.0, so the rule is cited and the code
// rewritten; §9.11's entry is in third_party/alt-tab-macos/ATTRIBUTION.md.

import { useLayoutEffect, useRef, useState } from 'react'
import type { ReactElement } from 'react'
import type { ServiceApi } from '../types/controls'
import { asList, asText, failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * One tile of the switcher (the windows source), carrying exactly the daemon's
 * row: window_id, app_id, title, index, selected, skippable.
 *
 * Index, Selected and Skippable are the daemon's own — it owns the MRU, so the
 * panel draws the position and the highlight it is handed rather than sorting a
 * second order of its own.
 */
export interface WindowRow {
  window_id: string
  app_id: string
  title: string
  index: number
  selected: boolean
  skippable: boolean
}

/**
 * The panel's own READ, narrowed.
 *
 * The Wails Service carries it (app/backend/service.go: Windows) but ServiceApi
 * does not model it yet, so the seam is narrowed here exactly as the file-type
 * catalog narrows its own. Only the read is narrowed here: the two WRITES belong
 * to the action registry, which is the one place a bound call is named, so
 * declaring them twice is the drift this file's own header warns about.
 *
 * A build with no binding REFUSES in words rather than throwing: "this build
 * cannot" is a defect somebody can go and fix, and an exception on a settings
 * page is a blank window.
 */
interface WindowList {
  Windows(): Promise<unknown>
}

/** Narrows the read, or returns null when this build carries none of it. */
function windowList(service: ServiceApi): WindowList | null {
  const candidate = service as Partial<WindowList>
  return typeof candidate.Windows === 'function' ? (candidate as WindowList) : null
}

/** The rows as a list, whatever crossed the wire. Never null: a table is []. */
function readRows(value: unknown): WindowRow[] {
  return asList(value).map(readRow)
}

/** One row, read defensively — a missing field is blank, never "undefined". */
function readRow(value: unknown): WindowRow {
  const row = value as Partial<WindowRow> | null
  return {
    window_id: asText(row?.window_id),
    app_id: asText(row?.app_id),
    title: asText(row?.title),
    index: typeof row?.index === 'number' ? row.index : -1,
    selected: row?.selected === true,
    skippable: row?.skippable === true,
  }
}

/**
 * What a tile is called on screen. The title first, the app second, and a last
 * resort that is still words. This ladder is CrossOS's own and is NOT a rectangle
 * port: rectangle has no title/app/key fallback to copy. Its snap-area menu
 * reads a localized name per action and simply omits an action that has none
 * (`WindowAction.displayName` is optional, `WindowAction.swift:364` declares it `String?`
 * by default; `SnapAreaViewController.swift:279` guards on it and returns). The
 * rule here is the opposite choice — never drop a tile, and never let one be
 * identified by its key alone, because a window whose only name is a number
 * tells a person nothing they can act on.
 */
function rowName(row: WindowRow): string {
  return row.title || row.app_id || 'Untitled window'
}

/**
 * The key a tile is rendered under. The window id is the row's identity; the
 * served position is the fallback for a row that arrived without one, so two
 * blank ids never collapse into a single tile.
 */
function rowKey(row: WindowRow, position: number): string {
  return row.window_id || `at ${position}`
}

// --- the grid, as arithmetic ------------------------------------------------

/** The five inputs the reference's TileGridLayout.Input carries. */
export interface TileGridInput {
  /** One per tile, in served order. A tile's own width is its own. */
  widths: number[]
  /** Every tile in a row is this tall; a row is one tile plus the padding. */
  tileHeight: number
  /** The panel's maximum width: the edge a tile wraps on, and the space rows
   *  are centered within. */
  widthMax: number
  padding: number
  /** false places tiles from the right edge and wraps at zero. */
  isLeftToRight: boolean
}

export interface TileOrigin {
  x: number
  y: number
}

export interface TileGrid {
  /** One per input width, in the same order. */
  origins: TileOrigin[]
  /** Input positions grouped by row. Always holds at least one row. */
  rows: number[][]
  /** What the panel is sized from: the far edge of the widest run of tiles
   *  (including the padding that follows the last) and the bottom of the last
   *  row. */
  maxX: number
  maxY: number
}

/** Swift's .rounded(.down): a projection is floored so accumulated rounding can
 *  never push a row that fits onto the next one. */
function floorTo(value: number): number {
  return Math.floor(value)
}

/** Swift's .rounded(): half away from zero, so 22.5 is 23 and -22.5 is -23. */
function rounded(value: number): number {
  return value < 0 ? -Math.round(-value) : Math.round(value)
}

/** In LTR currentX is the tile's leading edge; in RTL it is the trailing one. */
function projectedX(currentX: number, width: number, input: TileGridInput): number {
  return input.isLeftToRight
    ? currentX + width + input.padding
    : currentX - width - input.padding
}

/** A tile wraps when its far edge would pass the panel's edge: widthMax going
 *  left to right, zero going right to left. An edge that lands EXACTLY on it
 *  does not wrap. */
function needsNewRow(projected: number, input: TileGridInput): boolean {
  return input.isLeftToRight ? projected > input.widthMax : projected < 0
}

/** The frame's origin. In RTL currentX is the trailing edge, so the origin is
 *  one tile width before it. */
function originX(currentX: number, width: number, input: TileGridInput): number {
  return input.isLeftToRight ? currentX : currentX - width
}

/**
 * tileGrid places the tiles, as the reference does.
 *
 * The two subtleties it keeps are the ones that break a grid when they are
 * simplified: a tile that opens a row does NOT widen the grid (row 1 always
 * fills to within one tile of widthMax before it wraps, so no later row can be
 * wider than row 1 already recorded — and the one exception, a first tile wider
 * than the whole grid, is left at maxX 0), and maxY counts one row even with no
 * tiles at all, because the panel reserves a row before it knows what is in it.
 */
export function tileGrid(input: TileGridInput): TileGrid {
  const startingX = input.isLeftToRight ? input.padding : input.widthMax - input.padding
  let currentX = startingX
  let currentY = input.padding
  let maxX = 0
  let maxY = currentY + input.tileHeight + input.padding
  const origins: TileOrigin[] = []
  const rows: number[][] = [[]]

  for (const [position, width] of input.widths.entries()) {
    const nextX = floorTo(projectedX(currentX, width, input))
    if (needsNewRow(nextX, input)) {
      currentX = startingX
      currentY = floorTo(currentY + input.tileHeight + input.padding)
      origins.push({ x: originX(currentX, width, input), y: currentY })
      currentX = floorTo(projectedX(currentX, width, input))
      maxY = Math.max(currentY + input.tileHeight + input.padding, maxY)
      rows.push([])
    } else {
      origins.push({ x: originX(currentX, width, input), y: currentY })
      currentX = nextX
      maxX = Math.max(input.isLeftToRight ? currentX : input.widthMax - currentX, maxX)
    }
    rows[rows.length - 1].push(position)
  }
  return { origins, rows, maxX, maxY }
}

/**
 * How far each row has to move along the writing direction to sit centered in
 * `within`. Zero for an empty row and for a row already at least that wide, so
 * a full row is never pulled backwards off the edge.
 *
 * `within` is the width the CALLER chooses — the panel's own width here, not the
 * grid's maxX — which is why the last row of a short list sits under the middle
 * of the panel rather than under the middle of the tiles.
 */
export function centeringOffsets(rowWidths: number[][], padding: number, within: number): number[] {
  return rowWidths.map((widths) => {
    if (widths.length === 0) return 0
    const rowWidth = padding + widths.reduce((sum, width) => sum + width + padding, 0)
    return Math.max(0, rounded((within - rowWidth) / 2))
  })
}

/**
 * The auto size: the first candidate whose grid fits in heightMax, and the LAST
 * candidate when none do. `measure` is called once for every candidate up to and
 * including the chosen one, in order — the reference's measure closure applies
 * the size to the appearance before it can measure it, so the call order and
 * count are part of the contract rather than an accident.
 */
export function firstSizeThatFits<Size>(
  candidates: Size[],
  heightMax: number,
  measure: (candidate: Size) => number,
): Size | null {
  for (const [position, candidate] of candidates.entries()) {
    if (measure(candidate) <= heightMax || position === candidates.length - 1) return candidate
  }
  return null
}

// --- the panel's own geometry ------------------------------------------------

/**
 * The tile sizes, largest first: the order auto walks, and the one measured
 * first is the one a person sees when the space is generous.
 */
const TILE_SIZES = [
  { name: 'large', tileHeight: 132 },
  { name: 'medium', tileHeight: 104 },
  { name: 'small', tileHeight: 76 },
]

/** The panel's own max width, in CSS pixels. Both the grid's wrap edge and the
 *  space a short row is centered within are this. */
const PANEL_MAX_WIDTH = 720

/** The gap between tiles, which the reference measures the wrap WITH: a tile
 *  that would cross the edge counting the padding that follows it is the tile
 *  that wraps. */
const TILE_PADDING = 8

/**
 * The width a tile is given before the browser has measured one.
 *
 * Both moments below are real and both need a number rather than a blank grid:
 * the very first render, and the overlay window while it is still hidden (a
 * hidden window has no layout, so every tile measures zero). The first layout
 * is therefore the fallback one, and the measurement replaces it as soon as
 * there is a box to measure.
 */
const TILE_FALLBACK_WIDTH = 176

/** True when the widths the DOM reported are usable, i.e. something was laid
 *  out. An all-zero list is a window with no layout yet, not a grid of nothing. */
function measured(widths: number[]): boolean {
  return widths.length > 0 && widths.some((width) => width > 0)
}

/** The tile height the panel settles on, given the widths and the space. */
function settleTileHeight(widths: number[], heightMax: number, isLeftToRight: boolean): number {
  // Nothing has been laid out yet, so there is no space to fit into and the
  // declared default stands. An auto size asked against a zero-height panel
  // would otherwise fall through every candidate and settle on the smallest —
  // the reference's own answer for a panel genuinely too small, which is not the
  // question being asked before the first paint.
  if (heightMax <= 0) return TILE_SIZES[0].tileHeight
  const heightAt = (size: { tileHeight: number }): number =>
    tileGrid({
      widths,
      tileHeight: size.tileHeight,
      widthMax: PANEL_MAX_WIDTH,
      padding: TILE_PADDING,
      isLeftToRight,
    }).maxY
  const chosen = firstSizeThatFits(TILE_SIZES, heightMax, heightAt)
  return chosen ? chosen.tileHeight : TILE_SIZES[TILE_SIZES.length - 1].tileHeight
}

// --- the renderer -----------------------------------------------------------

export function SwitcherPanelControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const list = windowList(ctx.service)
  // The page declares the row write as an action id — the permission token the
  // daemon checks — so the panel resolves the write through the registry rather
  // than naming a bound method of its own.
  const rowAction = asText(control.rowAction)
  const windows = useResource<unknown>(
    () => (list ? list.Windows() : Promise.resolve([])),
    ctx.refreshToken,
    [],
    ctx.note,
  )
  const [writeError, setWriteError] = useState('')
  /** The window a focus is in flight for, so no tile is raised twice. */
  const [busy, setBusy] = useState('')

  const rows = readRows(windows.data)
  const grid = useRef<HTMLDivElement>(null)
  // What the browser can tell the grid: one width per tile, the writing
  // direction of the box they are in, and the height they have to fit into.
  // Read together and once per row set, because all three are the DOM's answer
  // and none of them may be read during a render.
  const [box, setBox] = useState({ widths: [] as number[], rtl: false, heightMax: 0 })
  // What is measured is the WHOLE row set, not just its keys: a daemon that
  // rewrites a window's title changes the width that title asks for, and a grid
  // left on the old width would place the next tile inside the one before it.
  const shape = JSON.stringify(rows)

  useLayoutEffect(() => {
    const measure = (): void => {
      const nodes = Array.from(grid.current?.querySelectorAll<HTMLElement>('[data-tile]') ?? [])
      const next = {
        widths: nodes.map((node) => node.offsetWidth),
        // The writing direction is the panel's own, read from the box the grid
        // is in: the reference's RTL half is a mirror of the same arithmetic, so
        // a page that set dir=rtl above gets the mirror for free.
        rtl: nodes.length > 0 && getComputedStyle(nodes[0]).direction === 'rtl',
        heightMax: document.documentElement.clientHeight,
      }
      // The same numbers keep the same state, so a re-measure that found
      // nothing new does not re-render the grid that was just drawn.
      setBox((previous) =>
        previous.widths.length === next.widths.length &&
        previous.widths.every((width, at) => width === next.widths[at]) &&
        previous.rtl === next.rtl &&
        previous.heightMax === next.heightMax
          ? previous
          : next,
      )
    }
    measure()
    // A window the person resizes has a different panel width, and the auto
    // tile size is measured against it — so a resize is a re-measure, not
    // something the next poll would happen to fix.
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [shape])

  // A window that has nothing to say about sizes leaves every tile at the
  // fallback width rather than at zero, so the grid still wraps and the rows
  // still center on the first layout.
  const own = measured(box.widths)
  const tileWidths = own ? box.widths : rows.map(() => TILE_FALLBACK_WIDTH)
  const isLeftToRight = !box.rtl
  const tileHeight = settleTileHeight(tileWidths, box.heightMax, isLeftToRight)

  const placed = tileGrid({
    widths: tileWidths,
    tileHeight,
    widthMax: PANEL_MAX_WIDTH,
    padding: TILE_PADDING,
    isLeftToRight,
  })
  const offsets = centeringOffsets(
    placed.rows.map((row) => row.map((position) => tileWidths[position])),
    TILE_PADDING,
    PANEL_MAX_WIDTH,
  )
  // Which row each tile landed on, so a tile is centered with its own row
  // instead of the rows being searched again for every tile.
  const rowOf = placed.rows.flatMap((row, at) => row.map(() => at))
  // The DOM's left edge. In RTL the reference's x is measured from widthMax, so
  // the origin CSS wants is its mirror across the panel.
  const leftOf = (origin: TileOrigin, width: number): number =>
    isLeftToRight ? origin.x : PANEL_MAX_WIDTH - origin.x - width

  /**
   * Raising a tile is the same write a release of the chord performs, so a
   * click never has to synthesize a keystroke. It fails closed like every other
   * write: a focus that did not happen must not leave the panel showing a
   * highlight the daemon never agreed to, so a refusal is printed and the rows
   * are re-read rather than assumed.
   */
  async function focus(row: WindowRow, key: string): Promise<void> {
    if (busy !== '') return
    const name = rowName(row)
    const command = commandFor(rowAction)
    if (!command) {
      const message = `${name} did not come to the front: this build has no command for ${
        rowAction || 'the write this page declares'
      }.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setBusy(key)
    try {
      await command(ctx.service, { id: row.window_id })
      setWriteError('')
      ctx.note(`${name} came to the front.`)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`Bringing ${name} to the front`, reason)
      setWriteError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  if (!list) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note}>
        <EmptyState>
          This build has no binding for the window list, so the windows the switcher can raise cannot
          be shown here.
        </EmptyState>
      </ControlFrame>
    )
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={writeError || windows.error}
    >
      {rows.length === 0 ? (
        <EmptyState>
          {windows.error
            ? `The window list did not arrive: ${windows.error}`
            : 'No windows are open. The switcher lists what is open right now, in the order it will raise them.'}
        </EmptyState>
      ) : (
        <div
          className="ctl-tile-grid"
          ref={grid}
          style={{ width: PANEL_MAX_WIDTH, height: placed.maxY }}
        >
          {rows.map((row, position) => {
            const key = rowKey(row, position)
            const name = rowName(row)
            const app = row.app_id
            const width = tileWidths[position]
            const at = rowOf[position]
            return (
              <button
                type="button"
                data-tile
                key={key}
                className={row.selected ? 'ctl-tile is-selected' : 'ctl-tile'}
                style={{
                  left: leftOf(placed.origins[position], width),
                  top: placed.origins[position].y + offsets[at],
                  // Until a tile has been measured it is left to its own
                  // content, because "a tile's own width is its own" is the
                  // width the CONTENT asks for — pinning it to the fallback
                  // first would make the measurement above read that same
                  // fallback back, and the grid would never learn a real width.
                  width: own ? width : 'auto',
                  height: tileHeight,
                }}
                // The highlight the daemon chose is announced as current, and
                // the tile SAYS it too: colour must never be the only signal
                // (common.tsx:60-68), so the word sits beside the name and not
                // only in the border.
                aria-current={row.selected ? 'true' : undefined}
                disabled={busy !== ''}
                onClick={() => void focus(row, key)}
              >
                <span className="ctl-tile-name">{name}</span>
                {app && app !== name ? <span className="ctl-tile-app">{app}</span> : null}
                {/* Skippable is a fact about the pick, not a fault in the
                    window: it is the row the switcher steps over. */}
                {row.skippable ? <span className="ctl-chip">Skippable</span> : null}
                {row.selected ? <span className="ctl-chip">Selected</span> : null}
              </button>
            )
          })}
        </div>
      )}
    </ControlFrame>
  )
}
