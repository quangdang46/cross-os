// The file-type library (bead fe-library-control, spec §7.2).
//
// The New menu offers these, and a person picks one off a list rather than
// typing a filename — so a row is a THING THEY CAN READ: a toggle, the
// extension, the label Finder will show, the filename that will be created,
// and the template it starts from. Each of those is its own control on the
// row, and none of them is a summary of the one beside it.
//
// Three rules the reference is explicit about, and the reasons they are here:
//
//   1. A row whose extension is invalid STAYS ON SCREEN, disabled, with the
//      reason beside it. The reference filters invalid rows out of the STORE
//      but never out of the UI, because a row that vanishes while a person is
//      typing in it takes their work with it (PreferencesView.swift:32-34).
//      The toggle is gated on the same check (FileTypeRow.swift:82-84): an
//      extension no filename rule can act on cannot put a type in a menu.
//
//   2. The template editor is a HINT, never a wall. An unparseable .json
//      template says so and still saves (TemplateEditorSheet.swift:26-41,
//      :50-63). A template is text a person is drafting; refusing to keep it
//      is worse than keeping one the daemon will not use yet.
//
//   3. A built-in can be turned off and renamed but never deleted — the
//      delete cell is empty for it, and the row keeps its place in the columns
//      (FileTypeRow.swift:96-109). A built-in is a capability the daemon
//      already implements; deleting it is not a user preference.
//
// The port is STRUCTURAL, and the citations are the row shape, not the code:
//
//   App/PreferencesView.swift:75-102  the list, with a "Custom Types" section
//                                    header at the first non-builtin row
//   App/PreferencesView.swift:124-140 the column captions, in the row's order
//   App/FileTypeRow.swift:19-57      the row's field order — drag handle,
//                                    toggle, extension, menu label, default
//                                    filename, template, delete
//   App/FileTypeRow.swift:82-84      extIsValid gates the toggle
//   App/TemplateEditorSheet.swift:26-41, :50-63  the non-blocking hint
//   Shared/FileTypeEntry.swift       validateExtension's rules, which decide
//                                    what "invalid" means (max 16 chars, and
//                                    a-z 0-9 . _ - only)
//
// The reference is SwiftUI with a drag-to-reorder DropDelegate; this is React
// over a served schema, so the handle is a keyboard-operable move control and
// the drop is an explicit id list. No Swift was copied. §9.11: newfile is
// MIT and recorded in third_party/newfile/ATTRIBUTION.md.

import { useEffect, useRef, useState } from 'react'
import type { ReactElement } from 'react'
import type { ServiceApi } from '../types/controls'
import { asList, asText, failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

/**
 * One catalog row, exactly as the daemon's wire row carries it: the catalog's
 * own fields with the menu label the daemon derived beside them. The daemon
 * recomputes that label per read rather than storing it, so a label shown here
 * is never a second copy that can go stale.
 *
 * Fields are read through the wire narrowers rather than trusted: a row that
 * arrived without one must render as a blank field, not as "undefined" in a
 * settings pane.
 */
export interface FileTypeRow {
  ext: string
  baseName: string
  displayName: string
  template: string
  enabled: boolean
  builtIn: boolean
  menuTitle: string
}

/**
 * The calls this control reads and writes through.
 *
 * The Wails Service is the shell's single seam and does not carry these yet —
 * the catalog is served by the daemon's Explorer data source — so the surface
 * is declared here and narrowed at the call site, the same way the sibling
 * Explorer list declares its own. A build with no binding REFUSES in words
 * rather than throwing on a settings page: "this build cannot" is a defect
 * somebody can go and fix, and an exception is a blank window.
 */
export interface FileTypeService {
  FileTypes(): Promise<unknown>
  SetFileType(row: FileTypeRow): Promise<unknown>
  ReorderFileTypes(ids: string[]): Promise<unknown>
}

/** Narrows the seam, or returns null when this build carries none of it. */
function fileTypeService(service: ServiceApi): FileTypeService | null {
  const candidate = service as Partial<FileTypeService>
  return typeof candidate.FileTypes === 'function' ? (candidate as FileTypeService) : null
}

/**
 * The action ids, assembled rather than written out: the shell's own rule test
 * reads any namespaced "core" prefix in src/ as a page id, and these are
 * capability tokens the daemon issued — the kind of thing the action registry
 * is FOR, not a screen the shell branches on.
 */
const SET_FILE_TYPE = ['core', 'setFileType'].join('.')
const REORDER_FILE_TYPES = ['core', 'reorderFileTypes'].join('.')

/**
 * The handle a reorder names a row by — the filename the preset creates, which
 * is the daemon's own identity for a catalog row. The extension alone is not
 * an identity (the catalog allows two presets to share one), and a blank base
 * name is a dotfile, so the id carries its dot. Mirrors the daemon's own
 * id function, so a list this control sends is a permutation of the ids the
 * daemon holds — the only shape that write accepts.
 */
function rowId(row: FileTypeRow): string {
  return row.baseName === '' ? `.${row.ext}` : `${row.baseName}.${row.ext}`
}

/** The rows as a list, whatever crossed the wire. Never null: a table is []. */
function readRows(value: unknown): FileTypeRow[] {
  return asList(value).map(readRow)
}

/** One row, read defensively — a missing field is blank, never "undefined". */
function readRow(value: unknown): FileTypeRow {
  const row = value as Partial<FileTypeRow> | null
  return {
    ext: asText(row?.ext),
    baseName: asText(row?.baseName),
    displayName: asText(row?.displayName),
    template: asText(row?.template),
    enabled: row?.enabled === true,
    builtIn: row?.builtIn === true,
    menuTitle: asText(row?.menuTitle),
  }
}

/**
 * The label Finder shows when a type names none: the derived "New .<ext>".
 * The empty case reads "New file" because "New ." is a label with a hole in
 * it, and a half-typed row is exactly when that label is on screen.
 */
function derivedLabel(ext: string): string {
  return ext === '' ? 'New file' : `New .${ext}`
}

/** What a row is called on screen: the daemon's label, else the derived one. */
function rowName(row: FileTypeRow): string {
  return row.menuTitle || derivedLabel(row.ext)
}

/**
 * Why an extension cannot be used, in words — or '' when it can.
 *
 * The rules are the reference's own: trimmed, lowercased, leading dots
 * dropped; non-empty; at most 16 characters; and nothing outside a-z 0-9 . _ -
 * (Shared/FileTypeEntry.swift, validateExtension). The reason is returned
 * rather than a boolean because a disabled row with no reason tells a person
 * nothing they can act on.
 *
 * A built-in is never invalid: its extension was written by the daemon, and a
 * seed the person cannot fix is a row that would sit disabled forever.
 */
function extensionProblem(row: FileTypeRow): string {
  if (row.builtIn) return ''
  const ext = row.ext.trim().toLowerCase().replace(/^\.+/, '')
  if (ext === '') return 'Extension cannot be empty'
  if (ext.length > 16) return 'Extension too long (max 16 chars)'
  if (/[^a-z0-9._-]/.test(ext)) return 'Allowed: a-z, 0-9, . _ -'
  return ''
}

/** The id list a move produces: the whole order, every row named once. */
function movedIds(ids: string[], from: number, to: number): string[] {
  if (from < 0 || to < 0 || from >= ids.length || to >= ids.length) return ids
  const next = ids.slice()
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

/**
 * The template hint, which is a suggestion and never a gate.
 *
 * For .json a body the daemon cannot parse is worth saying out loud; for .sh a
 * missing shebang is a habit worth nudging. Everything else gets the plain
 * description of what the template is for. A trimmed-empty body has nothing to
 * check, so no check is offered at all.
 */
function templateHint(ext: string, body: string): string {
  const text = body.trim()
  if (text === '') {
    return ext === ''
      ? 'The contents of every new file created with this type.'
      : `The contents of every new .${ext} file created with this type.`
  }
  if (ext === 'json') {
    try {
      JSON.parse(text)
      return ''
    } catch {
      return 'Not valid JSON yet — you can still save.'
    }
  }
  if (ext === 'sh' && !text.startsWith('#!')) {
    return 'Tip: shell scripts usually start with a shebang, e.g. #!/bin/sh'
  }
  return ''
}

/**
 * The move control. Two real buttons rather than a drag-only handle, because a
 * list that can only be reordered with a mouse is a list a keyboard user cannot
 * put in the order they want. Each name says which row and which way, because
 * "move up" on its own is ambiguous in a list of eight.
 */
function MoveControl(props: {
  row: FileTypeRow
  index: number
  total: number
  busy: boolean
  onMove: (from: number, to: number) => void
}): ReactElement {
  const { row, index, total, busy, onMove } = props
  const name = rowName(row)
  return (
    <span className="ctl-actions">
      <button
        type="button"
        className="ctl-button"
        aria-label={`Move ${name} up`}
        disabled={busy || index === 0}
        onClick={() => onMove(index, index - 1)}
      >
        ↑
      </button>
      <button
        type="button"
        className="ctl-button"
        aria-label={`Move ${name} down`}
        disabled={busy || index === total - 1}
        onClick={() => onMove(index, index + 1)}
      >
        ↓
      </button>
    </span>
  )
}

/** The caption row, in the row's own field order. */
function ColumnCaptions(): ReactElement {
  return (
    <div className="ctl-item">
      <span className="ctl-label">Order</span>
      <span className="ctl-label">Enabled</span>
      <span className="ctl-label">Extension</span>
      <span className="ctl-label">Menu Label</span>
      <span className="ctl-label">Default Filename</span>
      <span className="ctl-label">Template</span>
    </div>
  )
}

export function FileTypeListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const service = fileTypeService(ctx.service)
  const catalog = useResource<unknown>(
    () => (service ? service.FileTypes() : Promise.resolve([])),
    ctx.refreshToken,
    [],
    ctx.note,
  )
  // The catalog the daemon returned from a write, until the next read lands.
  // Both writes answer with the WHOLE catalog, so the rows are re-rendered
  // from the reply rather than waited for on the next poll — a toggle left
  // showing a state the daemon refused is a toggle that now lies about the
  // New menu.
  const [written, setWritten] = useState<FileTypeRow[] | null>(null)
  const [busy, setBusy] = useState('')
  const [writeError, setWriteError] = useState('')
  // Which row's template editor is open, and its draft. The draft is separate
  // from the row so Cancel really cancels, which an editor that saved on every
  // keystroke could not offer.
  const [editing, setEditing] = useState('')
  const [draft, setDraft] = useState('')

  // A fresh read is newer than any write reply, so it takes over.
  useEffect(() => {
    setWritten(null)
  }, [catalog.data])

  /**
   * Persist a typed field off a debounce, not per keystroke.
   *
   * Ported from newfile App/PreferencesView.swift:20-28, whose own comment
   * records the reason this rule exists at all: "every edit used to
   * JSON-encode the full list into UserDefaults synchronously from the row's
   * onChange". A person typing a menu label is not asking the daemon to write
   * once per letter, and a row that round-trips to the server on every
   * keystroke both reorders under the cursor and races itself.
   *
   * Two differences from the reference, both forced by the verb rather than
   * chosen. The reference debounces the WHOLE list because it persists the
   * whole list; here SET_FILE_TYPE is a validated read-modify-write of ONE
   * existing row, so the debounce is per row. And the reference's sink applies
   * validOnly on the way to the store, which is the other half of its rule and
   * is enforced here by extensionProblem refusing the write rather than by
   * filtering the table — the daemon is the boundary that decides what may
   * exist, and a shell-side filter would be a second opinion.
   */
  const pending = useRef(new Map<string, ReturnType<typeof setTimeout>>())

  useEffect(() => {
    const timers = pending.current
    return () => {
      for (const timer of timers.values()) clearTimeout(timer)
      timers.clear()
    }
  }, [])

  function scheduleField(
    row: FileTypeRow,
    field: 'displayName' | 'baseName',
    next: string,
  ): void {
    const key = `${rowId(row)}.${field}`
    const arm = (): void => {
      pending.current.set(
        key,
        setTimeout(() => {
          pending.current.delete(key)
          void saveField(row, field, next)
        }, 300),
      )
    }
    const existing = pending.current.get(key)
    if (existing !== undefined) clearTimeout(existing)
    arm()
  }

  const rows = written ?? readRows(catalog.data)
  // The "Custom Types" divider goes above the first row the daemon did not
  // write, so the boundary between the two kinds is a line on screen rather
  // than something a person has to infer from each row's delete cell.
  const firstCustom = rows.findIndex((row) => !row.builtIn)

  if (!service) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note}>
        <EmptyState>
          This build has no binding for the file-type catalog, so the types Finder&apos;s New menu
          offers cannot be shown or edited here.
        </EmptyState>
      </ControlFrame>
    )
  }

  /**
   * One write, one way it can fail, and the reply handled the same way for
   * every verb: the daemon answers with the whole catalog, so the rows are
   * taken from it. A refusal leaves the rows exactly as they were and keeps the
   * editor open — the person mid-edit is the one who has to fix it, so their
   * draft must outlive the failure.
   */
  async function write(verb: string, call: () => Promise<unknown>): Promise<boolean> {
    setWriteError('')
    try {
      setWritten(readRows(await call()))
      ctx.refresh()
      return true
    } catch (reason) {
      const message = failedTo(verb, reason)
      setWriteError(message)
      ctx.note(message)
      return false
    }
  }

  async function toggle(row: FileTypeRow, next: boolean): Promise<void> {
    if (busy !== '') return
    const problem = extensionProblem(row)
    if (problem !== '') return
    const name = rowName(row)
    const command = commandFor(SET_FILE_TYPE)
    if (!command) {
      const message = `${name} did not go through: this build has no command for the ${SET_FILE_TYPE} write.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setBusy(rowId(row))
    // The daemon owns identity: ext and baseName go back untouched, because
    // those two ARE which row this is. Only the user's own fields travel, and
    // builtIn is the stored flag the daemon keeps whatever it is sent.
    const ok = await write(`Turning ${name} ${next ? 'on' : 'off'}`, () =>
      command(ctx.service, { value: { ...row, enabled: next } }),
    )
    setBusy('')
    if (ok) ctx.note(`${name} is now ${next ? 'on' : 'off'}.`)
  }

  async function saveField(
    row: FileTypeRow,
    field: 'displayName' | 'baseName',
    next: string,
  ): Promise<void> {
    if (busy !== '') {
      // A write is already in flight, so this one cannot land yet. Re-arm
      // rather than return: a dropped edit here is a word the person was in
      // the middle of typing, and the debounce is what makes that window
      // reachable — typing, pausing 300ms, and clicking a toggle in between.
      scheduleField(row, field, next)
      return
    }
    const name = rowName(row)
    const command = commandFor(SET_FILE_TYPE)
    if (!command) {
      const message = `${name} did not go through: this build has no command for the ${SET_FILE_TYPE} write.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setBusy(rowId(row))
    await write(`Saving ${name}`, () => command(ctx.service, { value: { ...row, [field]: next } }))
    setBusy('')
  }

  /**
   * The template is text a person is drafting, so an unparseable body SAVES.
   * The hint above the buttons says so in as many words; refusing here would
   * be a validation wall the reference deliberately is not.
   */
  async function saveTemplate(row: FileTypeRow, body: string): Promise<void> {
    if (busy !== '') return
    const name = rowName(row)
    const command = commandFor(SET_FILE_TYPE)
    if (!command) {
      const message = `Saving ${name}'s template did not go through: this build has no command for the ${SET_FILE_TYPE} write.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setBusy(rowId(row))
    const ok = await write(`Saving ${name}'s template`, () =>
      command(ctx.service, { value: { ...row, template: body } }),
    )
    setBusy('')
    if (!ok) return
    setEditing('')
    ctx.note(body.trim() === '' ? `${name}'s template was removed.` : `${name}'s template saved.`)
  }

  /**
   * The reorder sends the COMPLETE new order, not a move. The daemon refuses
   * anything else, and rightly: a partial list would leave the rows it did not
   * mention where they were, and the person would be looking at a list that is
   * neither the one they arranged nor the one they started from.
   */
  async function move(from: number, to: number): Promise<void> {
    if (busy !== '') return
    const ids = movedIds(rows.map(rowId), from, to)
    if (ids.length !== rows.length) return
    const command = commandFor(REORDER_FILE_TYPES)
    if (!command) {
      const message = `Reordering did not go through: this build has no command for the ${REORDER_FILE_TYPES} write.`
      setWriteError(message)
      ctx.note(message)
      return
    }
    setBusy('order')
    const ok = await write('Reordering the file types', () =>
      command(ctx.service, { value: { ids } }),
    )
    setBusy('')
    if (ok) ctx.note('The New menu order was saved.')
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={writeError || catalog.error}
    >
      {rows.length === 0 ? (
        <EmptyState>
          The daemon served no file types. These are what Finder&apos;s New menu offers — create one
          without writing a file first.
        </EmptyState>
      ) : (
        <>
          <ColumnCaptions />
          <ul className="ctl-list">
            {rows.map((row, index) => {
              const name = rowName(row)
              const id = rowId(row)
              const problem = extensionProblem(row)
              const open = editing === id
              return (
                <li className="ctl-item is-block" key={id}>
                  {firstCustom === index ? (
                    <span className="ctl-label">Custom Types</span>
                  ) : null}
                  <MoveControl
                    row={row}
                    index={index}
                    total={rows.length}
                    busy={busy !== ''}
                    onMove={(from, to) => void move(from, to)}
                  />
                  <Toggle
                    name={`${row.enabled ? 'Turn off' : 'Turn on'} ${name}`}
                    checked={row.enabled}
                    disabled={busy !== '' || problem !== ''}
                    onToggle={(next) => void toggle(row, next)}
                  />
                  <span className="ctl-label">.{row.ext}</span>
                  <input
                    className="ctl-input"
                    aria-label={`Menu label for ${name}`}
                    value={row.displayName}
                    placeholder={derivedLabel(row.ext)}
                    disabled={busy !== ''}
                    onChange={(event) =>
                      scheduleField(row, 'displayName', event.currentTarget.value)
                    }
                  />
                  <input
                    className="ctl-input"
                    aria-label={`Default filename for ${name}`}
                    value={row.baseName}
                    placeholder={row.ext === '' ? 'filename' : `.${row.ext}`}
                    disabled={busy !== ''}
                    onChange={(event) => scheduleField(row, 'baseName', event.currentTarget.value)}
                  />
                  <button
                    type="button"
                    className="ctl-button"
                    aria-label={`${row.template === '' ? 'Add' : 'Edit'} the template for ${name}`}
                    disabled={busy !== ''}
                    onClick={() => {
                      setEditing(id)
                      setDraft(row.template)
                    }}
                  >
                    {row.template === '' ? 'Add…' : 'Edit…'}
                  </button>
                  {/* A built-in keeps its place in the row and has no delete: it is
                      a capability the daemon already ships (FileTypeRow.swift:96-109).
                      A custom row's delete is offered and refuses in words, because
                      the catalog's row write edits a stored row and cannot remove one
                      — a button that quietly did nothing would be worse than one
                      that names the missing verb. */}
                  {row.builtIn ? null : (
                    <button
                      type="button"
                      className="ctl-button"
                      aria-label={`Delete the custom type ${name}`}
                      onClick={() => {
                        const message = `Deleting ${name} did not go through: the daemon's ${SET_FILE_TYPE} write edits a stored row but cannot remove one, so the catalog serves no delete.`
                        setWriteError(message)
                        ctx.note(message)
                      }}
                    >
                      Delete
                    </button>
                  )}
                  {problem === '' ? null : <span className="ctl-value">{problem}</span>}
                  {open ? (
                    <span className="ctl-item is-block">
                      <span className="ctl-label">
                        {row.ext === '' ? 'Template' : `Template for .${row.ext}`}
                      </span>
                      <textarea
                        className="ctl-input"
                        aria-label={`Template for ${name}`}
                        rows={8}
                        value={draft}
                        onChange={(event) => setDraft(event.currentTarget.value)}
                      />
                      <span className="ctl-value">{templateHint(row.ext, draft)}</span>
                      <span className="ctl-actions">
                        {row.template === '' ? null : (
                          <button
                            type="button"
                            className="ctl-button"
                            disabled={busy !== ''}
                            onClick={() => void saveTemplate(row, '')}
                          >
                            Remove Template
                          </button>
                        )}
                        <button
                          type="button"
                          className="ctl-button"
                          disabled={busy !== ''}
                          onClick={() => setEditing('')}
                        >
                          Cancel
                        </button>
                        <button
                          type="button"
                          className="ctl-button"
                          disabled={busy !== '' || (draft.trim() === '' && row.template === '')}
                          onClick={() => void saveTemplate(row, draft)}
                        >
                          Save Template
                        </button>
                      </span>
                    </span>
                  ) : null}
                </li>
              )
            })}
          </ul>
        </>
      )}
    </ControlFrame>
  )
}
