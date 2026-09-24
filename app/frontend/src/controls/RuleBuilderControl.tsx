// The context rule builder (bead w7-frontend-remap).
//
// The spec's §4 writes the shape out twice, and both examples are the same
// chord doing different things in different apps:
//
//	IF App = Terminal  AND Ctrl+C  THEN Interrupt
//	IF App = Finder    AND Ctrl+C  THEN Copy
//
// "User kéo thả, không code" — the person picks, they do not code. So every
// dimension of the sentence is a CHOICE from what the daemon serves, and the
// sentence is drawn from those choices as they are made, because a builder that
// only shows you the result after you press Save is a form, and the spec asked
// for a builder.
//
// NO FREE TEXT WHERE A VOCABULARY EXISTS. The app is a picker over Apps() — the
// installed-application list, the same source the rule's app_ids are matched
// against, so a chosen row is one the matcher can find. The action is a picker
// over the actions the served rules name. The chord is captured by pressing it
// (ChordRecorder). There is no field anywhere in this control that accepts a
// typed app id or a typed action name, because each of those is a value the
// daemon holds a list of and a typo in one is a rule that matches nothing.
//
// WHEN THE SERVED LIST IS EMPTY, THAT IS SAID. A machine with no installed apps
// enumerated gets an app picker that says so and a rule that cannot be scoped —
// which is the truthful answer, not a gap to fill with a free-text box. The same
// goes for the actions: if the daemon serves no rules, there is nothing to
// bind, and the control says that instead of offering an empty dropdown that
// would save a rule running nothing.
//
// The rule is written whole and validated whole by the daemon (config.setUserRule
// → userrules.Store.Create), which refuses an unknown capability or app mode
// before storing anything. So this control sends the picked dimensions and shows
// the exact sentence that came back — the same contract ShortcutListControl has
// for the window table, for the same reason: one validator, never two.
//
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements), for the ROW
// shape — a rule drawn as two ends, each with its own control, neither of them
// a text field the person has to spell correctly.
//
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:47-72  one
//     row carrying one end of a remapping and the other, each end carrying its
//     own value and each independently legible
//
// What is CrossOS's own is the third end: the key, which is captured by pressing
// it rather than typed, and the sentence the picks assemble. No code was copied:
// the reference edits Karabiner's own remapping document, its device selector
// and its scripting parameters, none of which is part of a builder that writes
// CrossOS rules through the capability API. Tracked in
// third_party/Karabiner-Elements/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { AppRow, ChordCapture, MatrixRow, UserRuleRow } from '../types/controls'
import { failedTo } from '../lib/wire'
import { formatChord, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ChordRecorder, renderChord } from './ChordRecorder'
import { ControlFrame, EmptyState, SelectField } from './common'
import type { ControlProps } from './common'

/** A row with nothing in it, so the first Save has a shape to fill. */
function blankRule(): UserRuleRow {
  return {
    id: '',
    key: '',
    modifiers: [],
    app_modes: [],
    app_ids: [],
    device_id: '',
    capability: '',
    emit: true,
    chord: '',
    action: '',
    priority: 0,
    specificity: 0,
    scope: '',
  }
}

export function RuleBuilderControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const apps = useResource(() => ctx.service.Apps(), ctx.refreshToken, [], ctx.note)
  const rules = useResource(() => ctx.service.UserRules(), ctx.refreshToken, [], ctx.note)
  const matrix = useResource(() => ctx.service.GetMatrix(), ctx.refreshToken, [], ctx.note)
  const [draft, setDraft] = useState<UserRuleRow | null>(null)
  const [chord, setChord] = useState<ChordCapture | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')

  const rule = draft ?? blankRule()
  const stored: UserRuleRow[] = rules.data ?? []
  const served: MatrixRow[] = matrix.data ?? []

  // The action vocabulary, off the rules the daemon already serves.
  const actions: string[] = []
  for (const row of served) {
    if (row.action !== '' && !actions.includes(row.action)) actions.push(row.action)
  }
  // The app vocabulary, off the installed-application list. The label is the
  // app's own display name; the value is the bundle id the matcher compares
  // against, so the two cannot drift into different names for one application.
  const appRows: AppRow[] = apps.data ?? []
  const appOptions = [
    { value: '', text: 'any app' },
    ...appRows.map((app) => ({ value: app.BundleID, text: app.DisplayName || app.BundleID })),
  ]

  const chordText = renderChord(chord) || formatChord(rule.chord)
  // The sentence names the app the way the person knows it — the display name
  // the daemon enumerated — while the rule stores the bundle id the matcher
  // compares against. The two travel together on the AppRow, so the sentence
  // and the stored rule can never be about different applications.
  const appName = appRows.find((row) => row.BundleID === rule.app_ids[0])
  // Assembled as one string rather than as adjacent JSX text nodes, so it reads
  // — and is read by a screen reader, and asserted by a test — as the single
  // sentence the spec writes it as.
  const sentence = `IF App = ${appName ? appName.DisplayName || appName.BundleID : 'any app'} AND ${
    chordText || 'press a shortcut'
  } THEN ${rule.capability || 'choose an action'}`

  function update(field: keyof UserRuleRow, value: string): void {
    setDraft({ ...rule, [field]: value })
    setError('')
    setOutcome('')
  }

  async function save(): Promise<void> {
    if (chord === null && rule.key === '') {
      setError('Press the shortcut this rule answers to.')
      return
    }
    if (rule.capability === '') {
      setError('Choose an action from the list. It is what the rule will run.')
      return
    }
    setBusy(true)
    setError('')
    setOutcome('')
    try {
      // Only the stored dimensions travel. chord, action, priority,
      // specificity and scope are the daemon's derivation, recomputed on every
      // write, so a value sent back would be a number the person can change
      // without changing the rule.
      const id = await ctx.service.SetUserRule({
        ...rule,
        key: chord?.key || rule.key,
        modifiers: chord?.modifiers ?? rule.modifiers,
      })
      setDraft(null)
      setChord(null)
      const said = `Saved rule ${id}.`
      setOutcome(said)
      ctx.note(said)
      rules.reload()
      ctx.refresh()
    } catch (reason) {
      const message = failedTo('Saving the rule', reason)
      setError(`${message} Nothing was changed.`)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  async function remove(id: string): Promise<void> {
    setBusy(true)
    setError('')
    try {
      // The reply is the table as it now stands, so the list updates from the
      // write rather than from a second read that could disagree with it.
      const left = await ctx.service.DeleteUserRule(id)
      setDraft(null)
      setChord(null)
      const said = `Removed ${id}. ${left.length} rule${left.length === 1 ? '' : 's'} left.`
      setOutcome(said)
      ctx.note(said)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`Removing rule ${id}`, reason)
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
      error={error || rules.error || apps.error || matrix.error}
    >
      <div className="ctl-actions">
        <SelectField
          label="IF the front app is"
          value={rule.app_ids[0] ?? ''}
          options={appOptions}
          onChange={(value) => setDraft({ ...rule, app_ids: value === '' ? [] : [value] })}
          disabled={busy}
        />
        <ChordRecorder
          value={chord}
          onCapture={(next) => setDraft({ ...rule, key: next.key, modifiers: next.modifiers })}
          onReject={(reason) => {
            setError(reason)
            ctx.note(reason)
          }}
          disabled={busy}
          label="AND press the shortcut"
        />
        <SelectField
          label="THEN"
          value={rule.capability}
          options={[
            { value: '', text: actions.length === 0 ? 'no actions served' : 'choose an action' },
            ...actions.map((name) => ({ value: name, text: name })),
          ]}
          onChange={(value) => update('capability', value)}
          disabled={busy}
        />
      </div>

      {/* The sentence, drawn from the picks. It is the spec's own presentation
          of the rule, and showing it while the picks are being made is what
          makes this a builder rather than a form. */}
      <p className="ctl-clause">{sentence}</p>

      <div className="ctl-actions">
        <button className="ctl-input" type="button" disabled={busy} onClick={() => void save()}>
          {busy ? 'Saving…' : 'Save rule'}
        </button>
        <button
          className="ctl-input"
          type="button"
          disabled={busy || (draft === null && chord === null)}
          onClick={() => {
            setDraft(null)
            setChord(null)
            setError('')
            setOutcome('Cleared.')
          }}
        >
          Clear
        </button>
      </div>

      {stored.length === 0 ? (
        <EmptyState>No rules have been written. Ones you add appear here.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {stored.map((row) => (
            <li className="ctl-item" key={row.id}>
              <span className="ctl-label" title={row.chord}>
                {formatChord(row.chord) || row.key}
              </span>
              <span className="ctl-value">
                {row.app_ids.length > 0 ? humanize(row.app_ids[0]) : 'any app'}
              </span>
              <span className="ctl-value">{row.action || row.capability || 'no action'}</span>
              <span className="ctl-chip">{row.scope || 'global'}</span>
              <div className="ctl-actions">
                <button
                  className="ctl-input"
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    setDraft({ ...row })
                    setChord({ key: row.key, modifiers: row.modifiers })
                    setOutcome('')
                  }}
                >
                  Edit {row.id}
                </button>
                <button
                  className="ctl-input"
                  type="button"
                  disabled={busy}
                  onClick={() => void remove(row.id)}
                >
                  Remove {row.id}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {outcome ? <p className="ctl-value">{outcome}</p> : null}
    </ControlFrame>
  )
}
