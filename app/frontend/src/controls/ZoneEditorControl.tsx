// The snap-zone editor (bead cross-os-itq).
//
// Four number fields per zone, not a JSON textarea. The window page exists so
// somebody can move "left half" a few pixels because it clips a menu bar, and a
// textarea asks that user to know the shape of the record, spell it, and not
// fat-finger a brace — while a number field asks for the one number they
// actually changed.
//
// The zone NAMES are the daemon's to choose, but their IDs and NAMES are
// editable: winlayout.DefaultZones is empty by design (the daemon holds no
// display geometry outside the adapter), so without an Add control this editor
// could only ever show "No snap zones are configured" and a user who wanted a
// snap rectangle would have nowhere to put one. A new row starts with no id, no
// name and no area, and the daemon's own rules — a unique non-blank id, a
// name, a rectangle with extent — are checked here first so the refusal names
// the row instead of arriving as a raw RPC error.
//
// A cleared field becomes NaN rather than 0, so "I typed nothing" cannot be
// mistaken for "I typed zero" — a silently-zeroed zone is a window that snaps
// to a degenerate rectangle, and Save refuses to send it.
//
// Save is armed on a draft, never on the fetch. config.setZones is
// FULL-REPLACE (winlayout.ValidateZones accepts an empty slice), so a Save
// fired before the first load answered — or after one failed — would send
// {"zones":[]} and delete every rectangle the user had. The control cannot
// tell "no zones exist" from "this shell does not know what zones exist", and
// that is the distinction the never-a-blank-page rule turns on; refusing to
// write until there is something the user changed is the honest side of it.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ZoneRow } from '../types/controls'
import { asText, failedTo } from '../lib/wire'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, SelectField, TextField } from './common'
import type { ControlProps } from './common'

// The four geometry fields, named as the wire names them and labelled the way a
// person reads a rectangle. The wire spelling is the key; the label is ours.
/**
 * The eight named places, ported from Rectangle's snap-area rows
 * (SnapAreaViewController.swift:18-25: topLeft, top, topRight, left, right,
 * bottomLeft, bottom, bottomRight).
 *
 * The reference asks WHICH PLACE and CrossOS asked for four numbers, and that
 * is the whole difference between a person naming where a window should go and
 * a person working out what 0.5 means before they can start. The numbers stay
 * on the row for anyone placing an exact rectangle — this is an addition to the
 * editor, not a replacement of it, and a rectangle that is none of the eight
 * is a legitimate thing to want.
 *
 * Coordinates are the editor's own normalised [0,1] screen box, the same
 * convention the served rows already use (x 0.5, w 0.5 is the right half).
 */
const PLACES: readonly (readonly [string, number, number, number, number])[] = [
  ['Top left', 0, 0, 0.5, 0.5],
  ['Top', 0, 0, 1, 0.5],
  ['Top right', 0.5, 0, 0.5, 0.5],
  ['Left', 0, 0, 0.5, 1],
  ['Right', 0.5, 0, 0.5, 1],
  ['Bottom left', 0, 0.5, 0.5, 0.5],
  ['Bottom', 0, 0.5, 1, 0.5],
  ['Bottom right', 0.5, 0.5, 0.5, 0.5],
]

/** The place a row already sits in, or '' when it is an exact rectangle. */
function placeFor(row: ZoneRow): string {
  for (const [name, x, y, w, h] of PLACES) {
    if (row.x === x && row.y === y && row.w === w && row.h === h) return name
  }
  return ''
}

const FIELDS = [
  ['x', 'X'],
  ['y', 'Y'],
  ['w', 'Width'],
  ['h', 'Height'],
] as const
type Field = (typeof FIELDS)[number][0]

// The two text fields the daemon validates per zone, on the same footing as the
// geometry: a blank or duplicated id and a nameless row are both refused.
const TEXT = [
  ['id', 'Zone ID'],
  ['name', 'Name'],
] as const
type Text = (typeof TEXT)[number][0]

// blankZone is a row that cannot be saved yet, on purpose: NaN geometry renders
// as an empty field, and a missing id and name are the two things the daemon
// refuses first. Seeding plausible numbers would hand the user a rectangle they
// never placed.
function blankZone(): ZoneRow {
  return {
    id: '',
    name: '',
    x: Number.NaN,
    y: Number.NaN,
    w: Number.NaN,
    h: Number.NaN,
  }
}

export function ZoneEditorControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const zones = useResource(() => ctx.service.GetZones(), ctx.refreshToken, [], ctx.note)
  const [draft, setDraft] = useState<ZoneRow[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  const rows = draft ?? zones.data ?? []
  const dirty = draft !== null

  function update(index: number, row: ZoneRow): void {
    setDraft((draft ?? [...(zones.data ?? [])]).map((r, at) => (at === index ? row : r)))
  }

  /**
   * Choosing a place writes the rectangle, and the id and name when they are
   * still blank. It does NOT overwrite an id or a name a person already typed:
   * naming a zone is theirs, and the place is geometry.
   */
  function choosePlace(index: number, name: string): void {
    const place = PLACES.find(([label]) => label === name)
    if (!place) return
    const [, x, y, w, h] = place
    const row = rows[index]
    update(index, {
      ...row,
      id: row.id === '' ? name.toLowerCase().replace(/\s+/g, '-') : row.id,
      name: row.name === '' ? name : row.name,
      x,
      y,
      w,
      h,
    })
  }

  function editNumber(index: number, field: Field, text: string): void {
    const parsed = text.trim() === '' ? Number.NaN : Number(text)
    update(index, { ...rows[index], [field]: parsed })
  }

  function editText(index: number, field: Text, text: string): void {
    update(index, { ...rows[index], [field]: text })
  }

  function remove(index: number): void {
    setDraft(rows.filter((_, at) => at !== index))
  }

  function add(): void {
    setDraft([...rows, blankZone()])
  }

  // complain runs the daemon's own rules over the whole list before Save
  // sends it, so one bad row is named in a sentence the user can act on instead
  // of arriving as "winlayout: duplicate zone id". It never touches the stored
  // set: the write is the only thing that does, and it is not reached.
  function complain(list: ZoneRow[]): string {
    const seen = new Set<string>()
    for (const [index, row] of list.entries()) {
      const id = asText(row.id).trim()
      const where = asText(row.name).trim() || `row ${index + 1}`
      if (id === '') {
        return `Row ${index + 1} needs a zone ID. Nothing was saved.`
      }
      if (seen.has(id)) {
        return `Two rows use the zone ID "${id}". Nothing was saved.`
      }
      seen.add(id)
      if (where === '') {
        return `Zone ${id} needs a name. Nothing was saved.`
      }
      for (const [field, label] of FIELDS) {
        if (!Number.isFinite(row[field])) {
          return `${where} needs a number for ${label}. Nothing was saved.`
        }
      }
      if (row.w <= 0 || row.h <= 0) {
        return `${where} needs a width and a height above zero. Nothing was saved.`
      }
    }
    return ''
  }

  async function save(): Promise<void> {
    if (draft === null) return
    const problem = complain(draft)
    if (problem) {
      setError(problem)
      ctx.note(problem)
      return
    }
    setBusy(true)
    setError('')
    setOutcome('')
    try {
      const count = (await ctx.service.SetZones(draft)) ?? draft.length
      setDraft(null)
      const said = `Saved ${plural(count, 'zone')}.`
      setOutcome(said)
      ctx.note(said)
      ctx.refresh()
    } catch (reason) {
      setDraft(null)
      zones.reload()
      const message = failedTo('Saving the zones', reason)
      setError(`${message} Nothing was changed.`)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error || zones.error}>
      {rows.length === 0 ? (
        <EmptyState>No snap zones are configured.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row, index) => (
            // Positional key: the id is editable, and keying on it would
            // remount the inputs mid-typing and drop the caret.
            <li className="ctl-item" key={index}>
              <span className="ctl-label">
                {humanize(asText(row.name) || asText(row.id)) || `Zone ${index + 1}`}
              </span>
              {TEXT.map(([field, label]) => (
                <TextField
                  key={field}
                  label={`${label}, row ${index + 1}`}
                  value={asText(row[field])}
                  onChange={(text) => editText(index, field, text)}
                  disabled={busy}
                />
              ))}
              <SelectField
                label={`Place, row ${index + 1}`}
                value={placeFor(row)}
                onChange={(value) => choosePlace(index, value)}
                disabled={busy}
                options={[
                  { value: '', text: 'An exact rectangle' },
                  ...PLACES.map(([label]) => ({ value: label, text: label })),
                ]}
              />
              {FIELDS.map(([field, label]) => (
                <TextField
                  key={field}
                  label={`${label}, row ${index + 1}`}
                  type="number"
                  step="any"
                  value={Number.isFinite(row[field]) ? String(row[field]) : ''}
                  onChange={(text) => editNumber(index, field, text)}
                  disabled={busy}
                />
              ))}
              <div className="ctl-actions">
                <button className="ctl-input" type="button" disabled={busy} onClick={() => remove(index)}>
                  Remove zone {index + 1}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <div className="ctl-actions">
        {/* Save is live only for a list the user has actually changed AND that
            still holds a zone. A save of nothing is either a no-op or the
            destructive clear this editor has no way to distinguish from one,
            so the empty list is not a state this control offers to write. */}
        <button
          className="ctl-input"
          type="button"
          disabled={busy || !dirty || rows.length === 0}
          onClick={() => void save()}
        >
          {busy ? 'Saving…' : 'Save zones'}
        </button>
        <button className="ctl-input" type="button" disabled={busy} onClick={add}>
          Add a zone
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
    </ControlFrame>
  )
}
