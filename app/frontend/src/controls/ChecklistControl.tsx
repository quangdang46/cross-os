// The readiness checklist (bead cross-os-itq).
//
// Rows come from core.readiness, which is the daemon's own answer about what is
// ready and — the part that matters to a person — what to do about the parts
// that are not. A page also declares the ids it expects (`items`), and those
// are shown too: a checklist that quietly drops an item the page asked for
// reads as "done" when the truth is "never reported", which on a first-run page
// is the difference between fixing a permission and believing you fixed one.

import type { ReactElement } from 'react'
import { useResource } from '../lib/useResource'
import { humanize } from '../lib/format'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function ChecklistControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const readiness = useResource(() => ctx.service.Readiness(), ctx.refreshToken, [], ctx.note)
  const declared = Array.isArray(control.items) ? control.items : []
  const reported = readiness.data ?? []
  const unreported = declared.filter((id) => !reported.some((row) => row.id === id))

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={readiness.error}
    >
      {reported.length === 0 && unreported.length === 0 ? (
        <EmptyState>The daemon reported no readiness checks.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {reported.map((row) => (
            <li className="ctl-item" key={row.id}>
              <span className="ctl-chip">{row.ready ? 'Ready' : 'Not ready'}</span>
              <span className="ctl-label">{row.label || humanize(row.id)}</span>
              {row.detail ? <span className="ctl-value">{row.detail}</span> : null}
            </li>
          ))}
          {unreported.map((id) => (
            <li className="ctl-item" key={`unreported-${id}`}>
              <span className="ctl-chip">Not checked</span>
              <span className="ctl-label">{humanize(id)}</span>
              <span className="ctl-value">The daemon did not report on this one yet.</span>
            </li>
          ))}
        </ul>
      )}
      {readiness.loading ? <p className="ctl-value">Checking…</p> : null}
    </ControlFrame>
  )
}
