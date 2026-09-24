// Resolving a chord two rules both claim (bead w7-frontend-remap).
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
// Reading the verdict from the decision trace is deliberate. Traces() is the
// bound surface, and each row carries the winner and the losers exactly as the
// router recorded them — the same rule.Resolve call the conflicts source serves
// for the live table. Where the trace has not seen the chord yet there is no
// verdict to show, and the control says exactly that rather than picking a
// winner itself; a resolver that invented its own ranking would be a second,
// stale copy of the decision path, which is the failure this whole design
// exists to prevent.
//
// No port is claimed for this control. The ranked-conflict idea is the daemon's
// own, and the reference apps that were checked for it (Karabiner-Elements'
// settings window) carry no conflict surface at all — a claim of a port here
// would be a reference to something that does not exist.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ChordContest, TraceRow } from '../types/controls'
import { failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * The verdict for one chord, read out of the recorded decisions. The newest row
 * wins because a chord's ranking can change — a rule switched on, a scope
 * narrowed — and the most recent decision is the one the router would make now.
 */
function verdictFor(traces: TraceRow[], keys: string): ChordContest | null {
  const seen = traces.filter((trace) => trace.event.keys === keys && trace.winner !== '')
  const latest = seen[seen.length - 1]
  if (!latest) return null
  return { keys, winner: latest.winner, losers: latest.losers }
}

export function ConflictResolver(props: ControlProps): ReactElement {
  const { control, ctx } = props
  // The same bound source the pipeline control reads, through the shared hook,
  // so this control adds no call of its own and a page showing both draws one
  // set of decisions on one refresh cadence.
  const decisions = useResource(() => ctx.service.Traces(), ctx.refreshToken, [], ctx.note)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const traces = decisions.data ?? []

  // One row per contested chord. The recorder serves oldest-first, so the first
  // row for a chord decides where it sits in this list, while verdictFor reads
  // the LAST row for it — the ranking a chord's verdict can change as rules
  // are switched on and off, and the one the router would reach now.
  const contested: ChordContest[] = []
  for (const trace of traces) {
    if (trace.losers.length === 0) continue
    if (contested.some((row) => row.keys === trace.event.keys)) continue
    const verdict = verdictFor(traces, trace.event.keys)
    if (verdict) contested.push(verdict)
  }

  async function switchOff(ruleID: string, keys: string): Promise<void> {
    setBusy(ruleID)
    setError('')
    try {
      await ctx.service.SetRuleEnabled(ruleID, false)
      ctx.note(`${ruleID} is off, so ${keys} resolves to the rule that outranks it.`)
      decisions.reload()
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
      error={error || decisions.error}
    >
      {contested.length === 0 ? (
        <EmptyState>No shortcut is claimed by two rules. Nothing to resolve.</EmptyState>
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
                <span className="ctl-label">won by {row.winner}</span>
                <span className="ctl-value">lost to it: {row.losers.join(', ')}</span>
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
