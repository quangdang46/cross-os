// Resolving a chord two rules both claim (bead w7-frontend-remap, cross-os-1uu).
//
// A DUPLICATE CHORD IS A STATE, NOT AN ERROR. The rule engine already has an
// answer for a chord two rules claim: rule.Resolve ranks the candidates and
// names one winner and the rest losers, and the router calls it on every
// keystroke (event.Router.Decide). So a keymap editor that greets a collision
// with "duplicate chord" is reporting a question the daemon has already
// answered, and refusing to save until the user guesses the answer.
//
// The winner and the losers here are the ENGINE'S, not this file's. The control
// shows what rule.Resolve would do and offers the one action that changes it —
// switching the losing rule off — rather than a save button that would fail.
// That is the honest loop for a ranked system: you do not "resolve" a conflict
// in a resolver, you change which rule is eligible and the ranking does the
// rest.
//
// This used to read the verdict out of the decision trace, because the daemon's
// own conflict source had no bridge. That was a workaround with two costs
// worth naming, both now gone. It could only find a collision the person had
// ALREADY pressed, because a trace row only exists after a decision — and the
// collision worth warning about is the one that has not fired yet. And it could
// only see the last 200 decisions, so a busy session lost the older ones. The
// daemon has served Conflicts() the whole time: it compiles the candidate
// group into a real event.Router and calls Decide in each declared app context
// (core/cmd/crossos/pagedata.go, handleConflicts), so its winner is exactly what
// the keyboard will pick, and it answers for a chord nobody has pressed. User-
// authored rules participate, so a collision created in the rule editor above is
// reported here too.
//
// It also carries what the trace path threw away: which plugin each claimant
// belongs to, and its action in the same human words the matrix above uses. A
// row here and a row there now call the same rule the same thing.
//
// No port is claimed for this control. The ranked-conflict idea is the daemon's
// own, and the reference apps that were checked for it (Karabiner-Elements'
// settings window) carry no conflict surface at all — a claim of a port here
// would be a reference to something that does not exist.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ConflictRow } from '../types/controls'
import { failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * One claimant in the words the matrix above already uses: its plugin, its
 * action, and the rule id — so a person reading a collision here and the row
 * for the same rule three controls up is reading the same words both times.
 * The daemon supplies all three; the trace path this replaced carried the id
 * alone, which is how "won by windows-keyboard.ctrl-c-copy" ended up being the
 * whole sentence.
 */
function claimText(row: ConflictRow, ruleID: string): string {
  const claim = row.rules.find((r) => r.rule_id === ruleID)
  if (!claim) return ruleID
  return `${claim.action} (${claim.rule_id}, ${claim.plugin})`
}

export function ConflictResolver(props: ControlProps): ReactElement {
  const { control, ctx } = props
  // The daemon's own verdict, through the shared hook so this control opens no
  // timer of its own and a page showing both draws on one refresh cadence.
  const conflicts = useResource(() => ctx.service.Conflicts(), ctx.refreshToken, [], ctx.note)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  // One row per contested chord, already ranked: the source is the decision
  // path's answer for each one, so there is nothing to re-derive and nothing
  // to sort. A chord nobody has pressed is in here too — which is the whole
  // reason the trace path was given up.
  const contested: ConflictRow[] = conflicts.data ?? []

  async function switchOff(ruleID: string, keys: string): Promise<void> {
    setBusy(ruleID)
    setError('')
    try {
      await ctx.service.SetRuleEnabled(ruleID, false)
      ctx.note(`${ruleID} is off, so ${keys} resolves to the rule that outranks it.`)
      conflicts.reload()
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`Turning off ${ruleID}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || conflicts.error}
    >
      {contested.length === 0 ? (
        conflicts.error !== '' ? (
          // "Nothing is contested" and "the daemon would not say" are different
          // answers, and only the first one is good news. An empty list drawn
          // over a failed read tells a person their collisions are resolved
          // when nobody checked — so the refusal is the whole body here, and
          // the error above it says which source.
          <EmptyState>
            The daemon did not answer, so this page cannot say whether anything is contested. Nothing
            has been changed.
          </EmptyState>
        ) : (
          <EmptyState>
            No chord is claimed by two rules that are both still on. A rule you switch off stops
            contesting anything, which is why this list can be empty while the matrix above still
            shows a row.
          </EmptyState>
        )
      ) : (
        <>
          <p className="ctl-value">
            When two rules claim one chord the higher-ranked rule wins. Turning a losing rule off
            changes which one that is.
          </p>
          <ul className="ctl-list">
            {contested.map((row) => (
              <li className="ctl-item" key={row.keys}>
                <span className="ctl-chip">{row.keys}</span>
                <span className="ctl-label">{claimText(row, row.winner)}</span>
                <span className="ctl-value">
                  {row.losers.length === 1
                    ? `loses to it: ${claimText(row, row.losers[0])}`
                    : `loses to it: ${row.losers.map((id) => claimText(row, id)).join(', ')}`}
                </span>
                <div className="ctl-actions">
                  {row.losers.map((loser) => (
                    // A button, not a switch. The action this view offers is
                    // one-way — the loser's on/off state lives on the registry
                    // row, and a checkbox that can only ever be unchecked would
                    // claim to be a control with two positions when it has one.
                    <button
                      key={loser}
                      type="button"
                      className="ctl-button"
                      disabled={busy === loser}
                      onClick={() => void switchOff(loser, row.keys)}
                    >
                      {busy === loser
                        ? 'Turning off…'
                        : `Turn off ${loser} so ${row.keys} resolves to ${row.winner}`}
                    </button>
                  ))}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </ControlFrame>
  )
}
