// The version row (bead cross-os-itq).
//
// The value is status.Version, which the bridge reads from the daemon
// (core.CurrentVersion) — never a literal in this file. A settings window that
// reports a version the running daemon does not have is worse than one that
// says it does not know yet, so an empty status renders the empty state.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function VersionControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const version = ctx.status?.Version ?? ''

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note}>
      {version ? (
        <p className="ctl-value">CrossOS {version}</p>
      ) : (
        <EmptyState>The daemon has not reported a version yet.</EmptyState>
      )}
    </ControlFrame>
  )
}
