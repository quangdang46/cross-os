// The keymap editor (bead w7-frontend-remap).
//
// The spec's §3 is four steps and no JSON: "Search shortcut · Nhấn tổ hợp phím ·
// Chọn action · Resolve conflict. Mọi thứ sinh ra rule phía dưới" — every one
// of them is a step here, and none of them is typing a rule. The rule is
// assembled from the three things the daemon already serves (a chord, a key
// name, an action) and written with SetUserRule.
//
//	search      the list filter, over the served rules. A registry of twenty-odd
//	            shortcuts is short enough to read and long enough that the one
//	            you want is not on screen, so the filter is a real control rather
//	            than an optimisation.
//	capture     ChordRecorder. Pressing the chord is the input; see that file
//	            for why a typed chord cannot be trusted to be a real one.
//	action      a SELECT over the actions the daemon serves, never a text field.
//	            The action vocabulary is the router's (intent ids resolved
//	            through winlayout, which is what matrixAction hands back), so a
//	            picker built from the served rows cannot offer an action nothing
//	            can run, and a free-text field could.
//	resolve     a chord already claimed is a contest, shown with the winner and
//	            the losers the router decided, not a refusal. See
//	            ConflictResolver.
//
// THE SERVED ROWS ARE THE VOCABULARY. Both the action list and the conflict
// data are read from what the daemon already answered — GetMatrix for the rules
// and the actions they name, Traces for the verdicts — rather than from a
// second list this control keeps in step. A vocabulary the shell had to
// maintain separately would be a second source of truth for the same question,
// and it would be wrong the moment a rule was added.
//
// An empty registry reads as EMPTY, not broken: "No shortcuts are configured"
// is a sentence about the machine, and it is what a person with nothing bound
// should see. That is the never-a-blank-page rule, and it is why every
// resource here is read through useResource rather than awaited in an effect —
// a failed load keeps its own error row instead of leaving the pane empty.
//
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements), the
// simple-modifications editor this flow's shape comes from.
//
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:9-21    the
//     list of rules beside the editor for the one being read
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:47-72   a
//     row carrying one end of the remapping and the other, each independently
//     legible
//   src/apps/SettingsWindow/src/View/ComplexModificationsView.swift  the
//     filter above that list: a trimmed keyword matched against a per-rule
//     search text, not against an id
//   src/apps/SettingsWindow/src/View/SearchField.swift            the one text
//     field bound to it
//
// No code was copied: the reference is SwiftUI on macOS, its rules live in a
// JSON file this shell has no path to, and its device selector and JSON editor
// are not part of a settings window that renders daemon rows. The searchable
// list, the two-ended row and the press-to-capture input transfer; the drawing
// does not. Tracked in third_party/Karabiner-Elements/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ChordCapture, MatrixRow, TraceRow } from '../types/controls'
import { asText, failedTo } from '../lib/wire'
import { formatChord, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ChordRecorder, chordKey, renderChord } from './ChordRecorder'
import { ControlFrame, EmptyState, SelectField, TextField } from './common'
import type { ControlProps } from './common'

export function KeymapEditorControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const matrix = useResource(() => ctx.service.GetMatrix(), ctx.refreshToken, [], ctx.note)
  const decisions = useResource(() => ctx.service.Traces(), ctx.refreshToken, [], ctx.note)
  const [query, setQuery] = useState('')
  const [chord, setChord] = useState<ChordCapture | null>(null)
  const [action, setAction] = useState('')
  const [ruleID, setRuleID] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  const rows: MatrixRow[] = matrix.data ?? []
  const traces: TraceRow[] = decisions.data ?? []

  // The action vocabulary, read off the rules the daemon serves and de-duplicated
  // so one action claimed by six rules is one choice in the picker. A rule added
  // to the table today is a choice tomorrow, with no edit here.
  const actions: string[] = []
  for (const row of rows) {
    if (row.action !== '' && !actions.includes(row.action)) actions.push(row.action)
  }

  const needle = query.trim().toLowerCase()
  const visible = needle === ''
    ? rows
    : rows.filter((row) =>
        `${row.action} ${row.keys} ${row.plugin} ${row.contexts.join(' ')}`
          .toLowerCase()
          .includes(needle),
      )

  // The contest for the chord just captured, read out of the recorded verdicts.
  // Where the router has not decided this chord there is no verdict to show, and
  // the control says so rather than ranking the rules itself.
  // The lookup is on the daemon's own spelling of the chord, not the rendered
  // one: formatChord rewrites "Ctrl" to "ctrl", so a rendered chord would never
  // equal the served "Ctrl+C" and the contest would silently never be found.
  const captured = renderChord(chord)
  const capturedKey = chordKey(chord)
  const contest =
    capturedKey === ''
      ? undefined
      : traces.filter((trace) => trace.event.keys === capturedKey && trace.losers.length > 0).pop()
  // Assembled as one string, so the sentence is read and announced as a
  // sentence rather than as a run of fragments around two interpolations.
  const contestNote = contest
    ? `${capturedKey} is already claimed by ${contest.winner}, which wins over ${contest.losers.join(', ')}. Saving yours adds a rule to that contest — whichever rule outranks the others is the one that fires.`
    : ''

  function edit(chordDraft: ChordCapture): void {
    setChord(chordDraft)
    setError('')
    setOutcome('')
  }

  async function save(): Promise<void> {
    if (chord === null || chord.key === '') {
      setError('Record the shortcut first — press the keys you want this rule to answer to.')
      return
    }
    if (action === '') {
      setError('Choose an action from the list. It is what the shortcut will run.')
      return
    }
    setBusy(true)
    setError('')
    setOutcome('')
    try {
      // The stored dimensions only. Chord, Action, Priority, Specificity and
      // Scope are DERIVED by the daemon for display — userrules.Store.Create
      // recompiles them from the dimensions and ignores whatever arrived — so
      // the zeros below are placeholders the daemon overwrites, not values the
      // person set. The row type carries them because the daemon serves them on
      // the way back out.
      const id = await ctx.service.SetUserRule({
        id: ruleID,
        key: chord.key,
        modifiers: chord.modifiers,
        app_modes: [],
        app_ids: [],
        device_id: '',
        capability: action,
        emit: true,
        chord: '',
        action: '',
        priority: 0,
        specificity: 0,
        scope: '',
      })
      setRuleID(id)
      const said = `Saved the rule for ${captured}.`
      setOutcome(said)
      ctx.note(said)
      matrix.reload()
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`Saving the rule for ${captured}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || matrix.error || decisions.error}
    >
      <div className="ctl-actions">
        <TextField
          label="Search shortcuts"
          value={query}
          onChange={setQuery}
          placeholder="action, key or extension"
          disabled={busy}
        />
        <ChordRecorder
          value={chord}
          onCapture={edit}
          onReject={(reason) => {
            setError(reason)
            ctx.note(reason)
          }}
          disabled={busy}
        />
        <SelectField
          label="Action"
          value={action}
          options={[
            { value: '', text: actions.length === 0 ? 'no actions served' : 'choose an action' },
            ...actions.map((name) => ({ value: name, text: name })),
          ]}
          onChange={setAction}
          disabled={busy}
        />
      </div>

      {contestNote ? <p className="ctl-value">{contestNote}</p> : null}

      {rows.length === 0 ? (
        <EmptyState>No shortcuts are configured. The daemon serves this list.</EmptyState>
      ) : visible.length === 0 ? (
        <EmptyState>No shortcut matches “{query}”.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {visible.map((row) => (
            <li className="ctl-item" key={row.rule_id}>
              <span className="ctl-chip">{humanize(row.plugin)}</span>
              <span className="ctl-label" title={row.keys}>
                {formatChord(row.keys) || row.rule_id}
              </span>
              <span className="ctl-value">{row.action || 'no action'}</span>
              <span className="ctl-value">
                {row.enabled ? 'on' : 'off'}
                {row.contexts.length > 0 ? ` · ${row.contexts.join(', ')}` : ''}
              </span>
            </li>
          ))}
        </ul>
      )}

      <div className="ctl-actions">
        <button className="ctl-input" type="button" disabled={busy} onClick={() => void save()}>
          {busy ? 'Saving…' : 'Save rule'}
        </button>
        <button
          className="ctl-input"
          type="button"
          disabled={busy}
          onClick={() => {
            setChord(null)
            setRuleID('')
            setError('')
            setOutcome('Cleared.')
          }}
        >
          Clear
        </button>
      </div>
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
      {ruleID ? <p className="ctl-value">Editing rule {asText(ruleID)}.</p> : null}
    </ControlFrame>
  )
}
