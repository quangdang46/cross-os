// The readiness checklist (bead cross-os-itq; ordered by
// w6-frontend-setup-renderers).
//
// Rows come from the readiness source, which is the daemon's own answer about
// what is ready and — the part that matters to a person — what to do about the
// parts that are not. A page also declares the ids it expects (`items`), and
// those lead: a checklist that quietly drops an item the page asked for reads
// as "done" when the truth is "never reported", which on a first-run page is
// the difference between fixing a permission and believing you fixed one.
//
// The declared ids lead for a second reason. The daemon serves more rows than a
// page declares — the core daemon itself, and one row per installed plugin —
// and a first-run page that asked for three items would otherwise open on a
// list of five in which the three it cares about are wherever the daemon's own
// order put them. Declared first, answered second, unreported last: the page
// says which checks it wanted, the daemon says what they are, and anything the
// page wanted and the daemon did not answer is named as unreported rather than
// quietly missing.

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
  const rowFor = (id: string) => reported.find((row) => row.id === id)
  const answered = declared.filter((id) => rowFor(id) !== undefined)
  const unreported = declared.filter((id) => rowFor(id) === undefined)
  // Everything the daemon added beyond what the page asked for: the core row
  // and one row per installed plugin.
  const extra = reported.filter((row) => !declared.includes(row.id))

  const line = (row: (typeof reported)[number]) => (
    <li className="ctl-item" key={row.id}>
      <span className="ctl-chip">{row.ready ? 'Ready' : 'Not ready'}</span>
      <span className="ctl-label">{row.label || humanize(row.id)}</span>
      {row.detail ? <span className="ctl-value">{row.detail}</span> : null}
    </li>
  )

  if (reported.length === 0 && declared.length === 0) {
    return (
      <ControlFrame
        label={control.label ?? control.id}
        note={control.note}
        error={readiness.error}
      >
        <EmptyState>The daemon reported no readiness checks.</EmptyState>
        {readiness.loading ? <p className="ctl-value">Checking…</p> : null}
      </ControlFrame>
    )
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={readiness.error}
    >
      <ul className="ctl-list">
        {answered.map((id) => line(rowFor(id)!))}
        {unreported.map((id) => (
          <li className="ctl-item" key={`unreported-${id}`}>
            <span className="ctl-chip">Not checked</span>
            <span className="ctl-label">{humanize(id)}</span>
            <span className="ctl-value">The daemon did not report on this one yet.</span>
          </li>
        ))}
        {extra.map((row) => line(row))}
      </ul>
      {readiness.loading ? <p className="ctl-value">Checking…</p> : null}
    </ControlFrame>
  )
}
