// The shortcut table and its editor (bead cross-os-itq; remapped by
// w7-frontend-remap).
//
// TWO SOURCES, TWO TABLES, ONE RENDERER. Both WindowsPage and CommandsPage
// declare a `shortcutList`, and they do not mean the same thing:
//
//	core:windowShortcuts  the persisted window table (config.getShortcuts):
//	                      three columns — action, modifiers, key — editable in
//	                      place, because SetShortcuts REPLACES it and this is
//	                      the page that owns those chords.
//	core:allShortcuts     the whole registry: every rule the router compiles
//	                      (config.getMatrix), read-only, with per-row on/off
//	                      through SetRuleEnabled. CommandsPage is the "every
//	                      shortcut in one place" page and says so in its own
//	                      description, and it links per-rule editing back to the
//	                      owning page (editLinks:"owner") rather than offering a
//	                      second editor for the same rule.
//
// Before this dispatch both spellings rendered Shortcuts(), so the Shortcuts
// page showed the window table and called itself the global registry: a page
// that answered a different question than the one asked. Dispatching on
// `source` fixes that without a second component, because the difference is
// WHICH served rows a page wants, not how rows are drawn.
//
// The window editor's own rules are unchanged: a local draft, Save armed on the
// draft rather than on the fetch, because SetShortcuts is FULL-REPLACE and a
// save fired before the first load answered would persist an empty table.
//
// Validation is the daemon's, deliberately. winlayout rejects an unknown action
// name, a duplicate chord and a non-window capability before it stores anything
// (config.setShortcuts), so the honest editing loop is: send it, show the exact
// message that came back, and re-read the table to show that nothing moved. The
// shell duplicating those rules would give users two answers to the same
// question, one of which would be out of date the moment the action list grew.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { MatrixRow, ShortcutRow } from '../types/controls'
import { asText, failedTo, splitModifiers } from '../lib/wire'
import { formatChord, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, TextField, Toggle } from './common'
import type { ControlProps } from './common'

/**
 * The two sources this renderer answers, named as the pages declare them. A
 * source the page did not declare is treated as the window table rather than
 * refused: a page that omits `source` is asking for the table this component
 * has always drawn, and a hard error would break a page that works.
 */
const WINDOW_SOURCE = 'core:windowShortcuts'
const ALL_SOURCE = 'core:allShortcuts'

/** The conflict source both pages declare beside their table. */
const CONFLICTS_SOURCE = 'core:conflicts'

/** Which table a page's `source` asks for. */
type Table = 'all' | 'window'

function tableFor(source: unknown): Table {
  if (source === ALL_SOURCE) return 'all'
  if (source === WINDOW_SOURCE) return 'window'
  return 'window'
}

function blankShortcut(): ShortcutRow {
  return { action: '', modifiers: [], key: '' }
}

/** modifierText renders the two shapes a row may carry modifiers in. */
function modifierText(row: ShortcutRow): string {
  const raw = asText(row.modifiers)
  if (raw) return raw
  return Array.isArray(row.modifiers) ? row.modifiers.join(', ') : ''
}

/** The window table, editable in place. */
function WindowShortcutTable(props: ControlProps): ReactElement {
  const { ctx } = props
  const table = useResource(
    async () => readShortcutRows(await ctx.service.Shortcuts()),
    ctx.refreshToken,
    [] as ShortcutRow[],
    ctx.note,
  )
  const [draft, setDraft] = useState<ShortcutRow[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  const rows = draft ?? table.data ?? []
  const dirty = draft !== null

  function edit(index: number, field: string, value: string): void {
    const next = (draft ?? [...(table.data ?? [])]).map((row, at) =>
      at === index ? { ...row, [field]: value } : row,
    )
    setDraft(next)
  }

  function remove(index: number): void {
    const next = (draft ?? [...(table.data ?? [])]).filter((_, at) => at !== index)
    setDraft(next)
  }

  function add(): void {
    setDraft([...(draft ?? [...(table.data ?? [])]), blankShortcut()])
  }

  async function save(): Promise<void> {
    // No draft means no edit, and config.setShortcuts REPLACES the whole
    // table: winlayout.ValidateShortcuts returns nil for an empty slice, so a
    // save fired before the first load answered — or after one failed, since
    // the data is never replaced on error — would persist an empty shortcut
    // table and take the user's window chords with it. Removing every row
    // through the per-row Remove button still sets the draft, so the
    // deliberate clear of the table keeps working.
    if (draft === null) return
    const outgoing = draft.map((row) => ({
      action: asText(row.action).trim(),
      modifiers: splitModifiers(modifierText(row)),
      key: asText(row.key).trim(),
    }))
    setBusy(true)
    setError('')
    setOutcome('')
    try {
      const count = await ctx.service.SetShortcuts(outgoing)
      setDraft(null)
      const said = `Saved ${count} shortcut${count === 1 ? '' : 's'}.`
      setOutcome(said)
      ctx.note(said)
      ctx.refresh()
    } catch (reason) {
      // The rejected draft is dropped and the table re-read: the daemon
      // validates before it stores (config.setShortcuts), so what comes back
      // is proof the running set never moved. Keeping the draft on screen
      // would invite the user to press Save again against the same error.
      setDraft(null)
      table.reload()
      const message = failedTo('Saving the shortcut table', reason)
      setError(`${message} Nothing was changed.`)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      {rows.length === 0 ? (
        <EmptyState>No window shortcuts are configured.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row, index) => (
            <li className="ctl-item" key={index}>
              <TextField
                label={`Action, row ${index + 1}`}
                value={asText(row.action)}
                onChange={(value) => edit(index, 'action', value)}
                disabled={busy}
              />
              <TextField
                label={`Modifiers, row ${index + 1}`}
                value={modifierText(row)}
                onChange={(value) => edit(index, 'modifiers', value)}
                placeholder="ctrl, shift"
                disabled={busy}
              />
              <TextField
                label={`Key, row ${index + 1}`}
                value={asText(row.key)}
                onChange={(value) => edit(index, 'key', value)}
                disabled={busy}
              />
              <div className="ctl-actions">
                <button className="ctl-input" type="button" disabled={busy} onClick={() => remove(index)}>
                  Remove row {index + 1}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="ctl-actions">
        <button
          className="ctl-input"
          type="button"
          disabled={busy || !dirty}
          onClick={() => void save()}
        >
          {busy ? 'Saving…' : 'Save shortcuts'}
        </button>
        <button className="ctl-input" type="button" disabled={busy} onClick={add}>
          Add a shortcut
        </button>
        <button
          className="ctl-input"
          type="button"
          disabled={busy || !dirty}
          onClick={() => {
            setDraft(null)
            setError('')
            setOutcome('Edits discarded.')
          }}
        >
          Discard changes
        </button>
      </div>
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
      {/* The error row lives with the table that produced it. The frame is
          shared by both tables, so a save refusal that the window editor
          raised has to be printed here — a control that set an error and never
          rendered it is the silent failure the never-a-blank-page rule exists
          to prevent. */}
      {error || table.error ? <p className="ctl-error" role="alert">{error || table.error}</p> : null}
    </>
  )
}

/**
 * The whole registry, one row per rule the router compiles, with a per-row
 * switch. The toggle is a real enable/disable write (SetRuleEnabled), not a
 * link to the owning page: CommandsPage declares shortcut.setEnabled for
 * exactly this, and a registry a person can only read is a registry they cannot
 * use. What the page does NOT get is an editor — its own `editLinks:"owner"`
 * says per-rule content is edited where the rule is owned.
 */
function AllShortcutTable(props: ControlProps): ReactElement {
  const { ctx } = props
  const matrix = useResource(() => ctx.service.GetMatrix(), ctx.refreshToken, [], ctx.note)
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const rows: MatrixRow[] = matrix.data ?? []

  async function toggle(row: MatrixRow, next: boolean): Promise<void> {
    setPending(row.rule_id)
    setError('')
    try {
      await ctx.service.SetRuleEnabled(row.rule_id, next)
      ctx.note(`${row.action || row.rule_id} is now ${next ? 'on' : 'off'}.`)
      ctx.refresh()
    } catch (reason) {
      // SetRuleEnabled fails closed on a rule id the daemon does not have, so
      // the refusal is the answer and the switch is left where it was rather
      // than drawn in a state the daemon never accepted.
      const message = failedTo(`${next ? 'Turning on' : 'Turning off'} ${row.action || row.rule_id}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setPending('')
    }
  }

  if (rows.length === 0) {
    // A failed load is a sentence about the daemon; an empty registry is a
    // sentence about the machine. Collapsing the two is how a settings window
    // becomes a blank page, so the load error is printed and only then is the
    // list reported as empty.
    return matrix.error ? (
      <p className="ctl-error" role="alert">
        {matrix.error}
      </p>
    ) : (
      <EmptyState>No shortcuts are configured. The daemon serves this list.</EmptyState>
    )
  }

  return (
    <>
      <ul className="ctl-list">
        {rows.map((row) => (
          // Karabiner's row says the off state in a WORD beside the toggle and
          // shows it in the INK at the same time
          // (ComplexModificationsView.swift:175-182). Both halves, and they
          // agree: a row that is only greyed says nothing to a colour-blind
          // reader or to a screen reader, and a toggle that is only greyed says
          // nothing about what the rule is doing.
          <li className={row.enabled ? 'ctl-item' : 'ctl-item ctl-off'} key={row.rule_id}>
            <span className="ctl-chip">{humanize(row.plugin)}</span>
            <span className="ctl-label" title={row.keys}>
              {formatChord(row.keys) || row.rule_id}
            </span>
            <span className="ctl-value">{row.action || 'no action'}</span>
            {row.contexts.length > 0 ? (
              <span className="ctl-value">{row.contexts.join(', ')}</span>
            ) : (
              <span className="ctl-value">everywhere</span>
            )}
            <span className="ctl-value">{row.enabled ? 'on' : 'off'}</span>
            <Toggle
              name={`${row.enabled ? 'Turn off' : 'Turn on'} ${row.action || row.rule_id}`}
              checked={row.enabled}
              disabled={pending === row.rule_id}
              onToggle={(next) => void toggle(row, next)}
            />
          </li>
        ))}
      </ul>
      {/* The same sentence the window table has always shown, and for the same
          reason: the Shortcuts page hosts the registry, the Keyboard and
          Windows pages host the editing. */}
      <p className="ctl-empty">Edits to these shortcuts are made on the page that owns them.</p>
      {error || matrix.error ? <p className="ctl-error" role="alert">{error || matrix.error}</p> : null}
    </>
  )
}

export function ShortcutListControl(props: ControlProps): ReactElement {
  const { control } = props
  // The source decides the table, so the two pages that declare this kind get
  // the two different answers they asked for. Everything else about the
  // control — the frame, the note, the page's own conflicts field — is shared.
  const table = tableFor(control.source)

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note}>
      {control.conflicts === CONFLICTS_SOURCE ? (
        <p className="ctl-value">
          Conflicts resolve by rule priority — the winner and the rules that lost to it are shown
          on the Activity page.
        </p>
      ) : null}
      {table === 'all' ? <AllShortcutTable {...props} /> : <WindowShortcutTable {...props} />}
    </ControlFrame>
  )
}

/** The wire carries nullable maps; a row is what the editor can render. */
function readShortcutRows(value: unknown): ShortcutRow[] {
  if (!Array.isArray(value)) return []
  return value.filter(
    (row): row is ShortcutRow => row !== null && typeof row === 'object',
  )
}
