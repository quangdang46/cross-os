// The per-app override table (bead cross-os-itq).
//
// An override shadows one matrix rule inside one app, which is why the row
// repeats the action and chord instead of pointing at a rule: what the app will
// actually do is the thing a user is checking, not which rule produced it.
//
// The daemon's SetOverride echoes only {app, rule_id, enabled} — the wire
// contract deliberately omits action and keys — so the answer is merged onto the
// existing row rather than replacing it. Replacing it would blank the two
// columns the user is reading, which is the same "empty value" trap the wire
// contract's "collections are [], never null" rule exists to prevent.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { failedTo } from '../lib/wire'
import { formatChord, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

function rowKey(app: string, ruleID: string): string {
  return `${app}/${ruleID}`
}

export function OverridesControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const overrides = useResource(() => ctx.service.GetOverrides(), ctx.refreshToken, [], ctx.note)
  const [confirmed, setConfirmed] = useState<Record<string, boolean>>({})
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const rows = overrides.data ?? []

  async function toggle(app: string, ruleID: string, keys: string, next: boolean): Promise<void> {
    setPending(rowKey(app, ruleID))
    setError('')
    try {
      const answer = await ctx.service.SetOverride(app, ruleID, next)
      setConfirmed((prev) => ({ ...prev, [rowKey(app, ruleID)]: answer.enabled }))
      ctx.note(`${app} · ${keys || ruleID} is now ${answer.enabled ? 'on' : 'off'}.`)
    } catch (reason) {
      const message = failedTo(`${next ? 'Turning on' : 'Turning off'} ${keys || ruleID} in ${app}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setPending('')
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || overrides.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No app overrides are set. A rule applies everywhere until one is.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row) => {
            const key = rowKey(row.app, row.rule_id)
            const enabled = confirmed[key] ?? row.enabled
            const chord = formatChord(row.keys)
            // The same greying as the matrix beside it, and from the same
            // reference (ComplexModificationsView.swift:175-177). `enabled` is
            // the daemon's echoed verdict, so this row, its word and its
            // toggle cannot disagree.
            return (
              <li className={enabled ? 'ctl-item' : 'ctl-item ctl-off'} key={key}>
                <span className="ctl-chip">{humanize(row.app)}</span>
                <span className="ctl-label" title={row.keys}>
                  {chord || row.rule_id}
                </span>
                <span className="ctl-value">{row.action || 'no action'}</span>
                <span className="ctl-value">{enabled ? 'On for this app' : 'Off for this app'}</span>
                <Toggle
                  name={`${enabled ? 'Stop' : 'Apply'} ${chord || row.rule_id} in ${row.app}`}
                  checked={enabled}
                  disabled={pending === key}
                  onToggle={(next) => void toggle(row.app, row.rule_id, chord, next)}
                />
              </li>
            )
          })}
        </ul>
      )}
      {overrides.loading ? <p className="ctl-value">Loading…</p> : null}
    </ControlFrame>
  )
}
