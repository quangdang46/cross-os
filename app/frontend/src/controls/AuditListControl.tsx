// "What CrossOS created" — the ownership audit (bead cross-os-itq).
//
// Every row names the resource, its owner and when it appeared, because the
// Reset Everything plan is scoped by exactly this list (§8.2): a user deciding
// whether to let CrossOS clean up needs to see what it is about to touch before
// it touches it.
//
// The owning page declares the per-row action and
// this deliberately draws no button for it. The daemon's method table has no
// rollback handler and the frozen bridge exposes no command for one, so there
// is nothing to call — and the tempting shortcut, wiring the row to
// ResetEverything because that one does exist, would put a "roll back this
// single resource" label on a control that wipes everything CrossOS owns. The
// row says the affordance is missing instead, which is a bug someone can fix;
// the shortcut would have been a bug someone would trust.

import type { ReactElement } from 'react'
import { asText } from '../lib/wire'
import { relativeTime } from '../lib/format'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function AuditListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const audit = useResource(() => ctx.service.OwnershipAudit(), ctx.refreshToken, [], ctx.note)
  const rowAction = asText(control.rowAction)
  const canRollback = rowAction ? commandFor(rowAction) !== undefined : false
  const rows = audit.data ?? []

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={audit.error}
    >
      {rows.length === 0 ? (
        <EmptyState>CrossOS has not created anything on this machine yet.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {rows.map((row) => (
            <li className="ctl-item" key={`${row.resource}-${row.id}`}>
              <span className="ctl-chip">{row.resource || 'resource'}</span>
              <span className="ctl-label">{row.id || 'unnamed'}</span>
              <span className="ctl-value">owner: {row.owner || 'not reported'}</span>
              <span className="ctl-value" title={row.created_at}>
                created: {relativeTime(row.created_at)}
              </span>
            </li>
          ))}
        </ul>
      )}
      {rowAction && !canRollback ? (
        <p className="ctl-empty">
          This page offers “{rowAction}” on each row. Nothing in this build can carry that out
          yet, so the rows are listed without it.
        </p>
      ) : null}
    </ControlFrame>
  )
}
