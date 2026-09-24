// The window-shortcut table and its editor (bead cross-os-itq).
//
// Reading it is three columns — action, modifiers, key — which is exactly what
// config.getShortcuts serves. Editing is a local draft: nothing is written
// until Save, because config.setShortcuts REPLACES the whole table, and a write
// per keystroke would push an unvalidated table at the rules engine twice a
// second. The replace is also why Save is armed on the draft rather than on the
// fetch: see save() below.
//
// Which pages get the editor is the page's call, not this file's. A page that
// declares editable:true gets it; a page that declares editLinks:"owner" stays
// read-only, because per-rule editing belongs to the owning page and a second
// editor would be a second source of truth for the same chords.
//
// Validation is the daemon's, deliberately. winlayout rejects an unknown action
// name, a duplicate chord and a non-window capability before it stores anything
// (config.setShortcuts), so the honest editing loop is: send it, show the exact
// message that came back, and re-read the table to show that nothing moved. The
// shell duplicating those rules would give users two answers to the same
// question, one of which would be out of date the moment the action list grew.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ShortcutRow } from '../types/controls'
import { asText, failedTo, splitModifiers } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, TextField } from './common'
import type { ControlProps } from './common'

function blankShortcut(): ShortcutRow {
  return { action: '', modifiers: [], key: '' }
}

export function ShortcutListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const table = useResource(() => ctx.service.Shortcuts(), ctx.refreshToken, [], ctx.note)
  const [draft, setDraft] = useState<ShortcutRow[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  const editable = control.editable === true
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
      modifiers: splitModifiers(asText(row.modifiers) || (Array.isArray(row.modifiers) ? row.modifiers.join(', ') : '')),
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
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || table.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No window shortcuts are configured.</EmptyState>
      ) : editable ? (
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
                value={asText(row.modifiers) || (Array.isArray(row.modifiers) ? row.modifiers.join(', ') : '')}
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
      ) : (
        <ul className="ctl-list">
          {rows.map((row, index) => (
            <li className="ctl-item" key={index}>
              <span className="ctl-label">{asText(row.action) || 'unnamed action'}</span>
              <span className="ctl-chip">{asText(row.key) || 'no key'}</span>
              <span className="ctl-value">
                {Array.isArray(row.modifiers) && row.modifiers.length > 0
                  ? row.modifiers.join(' + ')
                  : 'no modifiers'}
              </span>
            </li>
          ))}
        </ul>
      )}
      {editable ? (
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
      ) : (
        <p className="ctl-empty">Edits to these shortcuts are made on the page that owns them.</p>
      )}
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
    </ControlFrame>
  )
}
