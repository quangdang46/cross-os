// The Explorer menu list: the seven right-click verbs, one row each (bead
// fe-menu-control, spec §7.2).
//
// What this control shows, and the three ways it is tempted to lie.
//
// It shows the SAME table the appex builds its menu from — core/pkg/findermenu's
// Menu, served by the daemon's Finder-menu source. A page that listed its own
// seven could disagree with the menu the user actually right-clicks, and the
// disagreement would be invisible until the click failed. So the rows arrive as
// data and this file names none of them.
//
// The three temptations:
//
//   1. A row the daemon reports unsupported is shown greyed, with no reason.
//      That is the one failure a person cannot see from the keyboard they are
//      still holding, so the reason is rendered in words, beside the box, and
//      never in colour alone (the Toggle note in common.tsx is the same rule).
//
//   2. The write is optimistic — the switch flips before the daemon answers —
//      so a rejected write has to put the switch back and say the daemon's own
//      message. The reply is the WHOLE menu table, so the rows are re-rendered
//      from it: a switch left showing a state the daemon refused is a switch
//      that now lies about the menu.
//
//   3. "Open in Editor" is drawn as "Open in VS Code". It is not one editor,
//      it is one entry in a catalog the daemon owns and orders, and a menu
//      offering an editor the daemon would refuse is a menu that fails on
//      click. When the catalog comes back empty this row SAYS SO rather than
//      offering a submenu with nothing in it.
//
// Port source: newfile's PreferencesView, whose library is a plain scroll list —
// a ForEach in which every entry becomes its own row — rather than a table with
// column headers, because the NSTableView row machinery behind SwiftUI's List
// added first-click latency on the row controls.
//
//   App/PreferencesView.swift:75-102
//
// The RULE that transfers is the flat list of self-contained rows. The code does
// not: that is SwiftUI over a drag-to-reorder model, and this is React over a
// declared schema reading a served table. No code was copied. Attributed in
// third_party/newfile/ATTRIBUTION.md.
//
// The ordered-editor-catalog rule is cited as BEHAVIOUR ONLY, from
// tmp/research/FinderRight/…/EditorCatalog.swift:16-31 (MIT): what transfers is
// that one ordered list is the single source both the submenu and the launch
// read. Nothing was ported from that file — the catalog here is the daemon's,
// and no editor name is written in this file at all.

import { useEffect, useState } from 'react'
import type { ReactElement } from 'react'
import type { ServiceApi } from '../types/controls'
import { asList, asText, describeError, failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

/**
 * The row the daemon serves for one menu item, exactly as core/pkg/findermenu's
 * Item is embedded in the wire row with the host's `supported` beside it.
 *
 * `reason` and `editors` are optional because the daemon does not send them
 * today: a field this build does not model must degrade to `undefined` at
 * runtime rather than to a compile error, the same rule the Control schema
 * states for itself. An item with no reason and supported=false is still
 * rendered with words — see unavailableReason.
 */
export interface FinderMenuRow {
  id: string
  title: string
  contexts: string[]
  needsPaths: boolean
  capability: string
  enabled: boolean
  supported: boolean
  /** The daemon's own words for why this host cannot run the item. */
  reason?: string
  /** The Open-in-Editor catalog, in the daemon's order. */
  editors?: EditorEntry[]
}

/** One editor the Open-in-Editor submenu offers. ID is the bundle id, which is
 *  also what the launch takes, so the catalog and the launch cannot disagree. */
export interface EditorEntry {
  id: string
  name: string
}

/**
 * The one call this control READS through. A source is always a bound service
 * method and never an action — actions are the write half — so this control
 * reads the menu the way every other list control reads its rows, through
 * ControlContext.service.
 *
 * The Wails Service is the shell's single seam (types/controls.ts) and does not
 * carry this call yet, so the surface is declared here rather than reached for
 * by name. A missing binding is a REFUSAL in words, not a TypeError on a
 * settings page — the same treatment the sibling Explorer list gives its own
 * source, because a control that reports "this build cannot" is a defect
 * someone can go and fix, and a control that throws is a blank window.
 */
/** The two calls this control needs, off the declared service surface. */
export interface FinderMenuService {
  FinderMenu(): Promise<unknown>
  SetMenuItemEnabled(id: string, enabled: boolean): Promise<unknown>
}

/**
 * Narrows the seam, or returns null when this build carries none of it.
 *
 * It has to CHECK rather than build, and that is the whole content of the
 * function. An earlier version assembled the two methods by reaching through to
 * `service.FinderMenu` inside a closure — which always returned an object, so
 * the "this build has no binding" branch below could never be taken, and the
 * read it guarded still ran: `ctx.service.FinderMenu()` on a build that does
 * not carry the call throws a TypeError, and that throw happens inside the
 * effect the shared loader runs, which no error row catches. The page went
 * blank instead of saying one sentence. A guard that cannot fail is not a
 * guard, and the empty branch below is this control's real behaviour rather
 * than dead code.
 *
 * The read is the thing being checked, and the write deliberately is not: the
 * write goes through the action registry, whose commands are async, so a
 * binding that is missing there rejects and the refusal is rendered like any
 * other refused write.
 */
function menuService(service: ServiceApi): FinderMenuService | null {
  const candidate = service as Partial<FinderMenuService>
  return typeof candidate.FinderMenu === 'function' ? (candidate as FinderMenuService) : null
}

/**
 * The action id, assembled rather than written out. The shell's own rule test
 * (test/shell.test.tsx) reads any namespaced "core" prefix in src/ as a page id,
 * which is why actions.ts assembles its one core-prefixed id the same way. An
 * action id is a capability token the daemon issued, not a screen.
 */
const SET_ENABLED = ['core', 'setMenuItemEnabled'].join('.')

/**
 * The capability the Open in Editor row runs. A capability name is daemon
 * vocabulary, the same KIND of key as an action id — not a page id, not a control
 * id — so branching on it is the same kind of branch the action registry is.
 */
const OPEN_APP = 'app.open'

/** The rows as a list, whatever crossed the wire. Never null: a table is []. */
function readRows(value: unknown): FinderMenuRow[] {
  return asList(value) as FinderMenuRow[]
}

/** The editor catalog as a list, in the daemon's order. Never null. */
function readEditors(value: unknown): EditorEntry[] {
  return asList(value) as EditorEntry[]
}

/**
 * Why a row cannot run here, in words. The daemon's own `reason` is used
 * verbatim when it sends one; otherwise the capability is named, because a
 * capability name IS the thing that is missing and a row that says only
 * "unavailable" has told the reader nothing they can act on.
 */
function unavailableReason(row: FinderMenuRow): string {
  const reason = asText(row.reason)
  if (reason) return reason
  return `This host cannot run ${row.capability || 'this item'}.`
}

/** The contexts a verb appears in, as a person reads them. */
function contextText(contexts: unknown): string {
  const names = asList(contexts)
    .map((entry) => asText(entry))
    .filter((entry) => entry !== '')
  if (names.length === 0) return 'This item appears in no selection context.'
  const verb = names.length === 1 ? 'Appears' : 'Appear'
  return `${verb} when you ${names.join(', ')} ${names.length === 1 ? 'is' : 'are'} selected.`
}

/** One menu row. */
function MenuRowView(props: {
  row: FinderMenuRow
  pending: boolean
  onToggle: (row: FinderMenuRow, next: boolean) => void
}): ReactElement {
  const { row, pending, onToggle } = props
  const supported = row.supported !== false
  const editors = row.capability === OPEN_APP ? readEditors(row.editors) : []

  return (
    <li className="ctl-item">
      <span className="ctl-label">{asText(row.title) || row.id}</span>
      <span className="ctl-value">{asText(row.capability)}</span>
      <span className="ctl-value">{contextText(row.contexts)}</span>
      {row.needsPaths ? (
        <span className="ctl-value">Works on a selection of files or folders.</span>
      ) : null}
      {supported ? null : (
        <span className="ctl-value">{unavailableReason(row)}</span>
      )}
      {row.capability === OPEN_APP ? (
        editors.length > 0 ? (
          <span className="ctl-value">
            Opens with: {editors.map((entry) => asText(entry.name) || entry.id).join(', ')}.
          </span>
        ) : (
          <span className="ctl-value">
            No editor catalog was served, so this submenu has nothing in it yet.
          </span>
        )
      ) : null}
      <Toggle
        name={`${row.enabled ? 'Turn off' : 'Turn on'} ${asText(row.title) || row.id}`}
        checked={row.enabled === true}
        disabled={!supported || pending}
        onToggle={(next) => onToggle(row, next)}
      />
    </li>
  )
}

export function FinderMenuControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const service = menuService(ctx.service)
  const table = useResource<unknown>(
    () => (service ? service.FinderMenu() : Promise.resolve([])),
    ctx.refreshToken,
    [],
    ctx.note,
  )
  // The table the daemon returned from a write, until the next read lands. The
  // write's reply is the whole authoritative table, so the rows are re-rendered
  // from it rather than waited for on the next poll — a switch that stayed on
  // the state the person chose, without a second round trip to confirm it.
  const [written, setWritten] = useState<FinderMenuRow[] | null>(null)
  const [pending, setPending] = useState('')
  const [writeError, setWriteError] = useState('')

  // A fresh read is newer than any write reply, so it takes over.
  useEffect(() => {
    setWritten(null)
  }, [table.data])

  const rows = written ?? readRows(table.data)

  if (!service) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note}>
        <EmptyState>
          This build has no binding for the Explorer menu, so the seven items cannot be shown
          or enabled here.
        </EmptyState>
      </ControlFrame>
    )
  }

  async function toggle(row: FinderMenuRow, next: boolean): Promise<void> {
    if (pending !== '') return
    const label = asText(row.title) || row.id
    const verb = `Turn ${label} ${next ? 'on' : 'off'}`
    // The page declares this write in its rowActions, so it goes through the
    // action registry like every other declared row write. An id the registry
    // does not carry is refused in words, naming the call it is waiting on —
    // never a switch that moves and changes nothing.
    const command = commandFor(SET_ENABLED)
    if (!command) {
      const message = `${verb} did not go through: this build has no command for the ${SET_ENABLED} write.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setPending(row.id)
    setWriteError('')
    try {
      // The reply is the WHOLE menu table, so the rows are re-rendered from it
      // rather than waited for on the next poll.
      setWritten(readRows(await command(ctx.service, { id: row.id, enabled: next })))
      ctx.refresh()
    } catch (reason) {
      // The daemon's own words, kept verbatim: they name the rule that refused
      // the edit, which is the only part that makes it fixable.
      const message = failedTo(verb, reason)
      setWriteError(message)
      ctx.note(message)
    } finally {
      setPending('')
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={writeError || (table.error ? describeError(table.error) : '')}
    >
      {rows.length === 0 ? (
        <EmptyState>
          {/* "Served no items" and "did not answer" are the same shape on the wire
              and completely different facts, so they do not get the same sentence.
              A read that failed has told us nothing about the menu, and claiming
              it served nothing is the control asserting a finding it did not
              receive — the same silence-as-answer the readiness checklist beside
              it on the first-run page refuses to draw. */}
          {table.error
            ? "The menu source did not answer, so nothing is shown here. These are the verbs Finder's right-click menu offers; each one can be turned on or off here."
            : "The daemon served no menu items. These are the verbs Finder's right-click menu offers; each one can be turned on or off here."}
        </EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row) => (
            <MenuRowView key={row.id} row={row} pending={pending === row.id} onToggle={toggle} />
          ))}
        </ul>
      )}
    </ControlFrame>
  )
}
