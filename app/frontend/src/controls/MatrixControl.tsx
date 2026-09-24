// The behaviour matrix (bead cross-os-itq).
//
// One row per rule: the chord, what it does, and where it applies. The toggle
// goes through config.setRuleEnabled and the row shows the state the DAEMON
// stored, not the one that was asked for: SetRuleEnabled hands back the stored
// bool, so a refused edit is an error to report rather than a second verdict to
// reconcile with the row.
//
// A disabled rule is shown as disabled, never as an error. Disabling a rule is
// what the control is for; dressing it in the same red as a failed write would
// teach users to ignore the row that actually matters.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { failedTo } from '../lib/wire'
import { formatChord, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

export function MatrixControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const matrix = useResource(() => ctx.service.GetMatrix(), ctx.refreshToken, [], ctx.note)
  const [confirmed, setConfirmed] = useState<Record<string, boolean>>({})
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const rows = matrix.data ?? []

  async function toggle(ruleID: string, keys: string, next: boolean): Promise<void> {
    setPending(ruleID)
    setError('')
    try {
      // Stored state, not a success flag. config.setRuleEnabled answers
      // {ruleId, enabled:<what was asked for>} and IPCCore decodes that
      // enabled straight into the bool return, so `stored` is always the new
      // state: reading it as "did it stick?" printed "The daemon kept Ctrl+C
      // off." the moment the user turned the rule OFF, on the one control whose
      // whole job is making the machine stop remapping that chord.
      const stored = await ctx.service.SetRuleEnabled(ruleID, next)
      setConfirmed((prev) => ({ ...prev, [ruleID]: stored }))
      ctx.note(`${keys || ruleID} is now ${stored ? 'on' : 'off'}.`)
    } catch (reason) {
      const message = failedTo(`${next ? 'Turning on' : 'Turning off'} ${keys || ruleID}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setPending('')
    }
  }

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error || matrix.error}>
      {rows.length === 0 ? (
        <EmptyState>No shortcuts are configured yet.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row) => {
            const enabled = confirmed[row.rule_id] ?? row.enabled
            const chord = formatChord(row.keys)
            const contexts = Array.isArray(row.contexts) ? row.contexts : []
            return (
              <li className="ctl-item" key={row.rule_id}>
                <span className="ctl-label" title={row.keys}>
                  {chord || row.rule_id}
                </span>
                <span className="ctl-value">{row.action || 'no action'}</span>
                <span className="ctl-chip">{humanize(row.plugin)}</span>
                {contexts.length === 0 ? (
                  <span className="ctl-value">everywhere</span>
                ) : (
                  contexts.map((context) => (
                    <span className="ctl-chip" key={context}>
                      {humanize(context)}
                    </span>
                  ))
                )}
                <span className="ctl-value">{enabled ? 'Enabled' : 'Disabled'}</span>
                <Toggle
                  name={`${enabled ? 'Disable' : 'Enable'} ${chord || row.rule_id}`}
                  checked={enabled}
                  disabled={pending === row.rule_id}
                  onToggle={(next) => void toggle(row.rule_id, chord, next)}
                />
              </li>
            )
          })}
        </ul>
      )}
      {matrix.loading ? <p className="ctl-value">Loading…</p> : null}
    </ControlFrame>
  )
}
