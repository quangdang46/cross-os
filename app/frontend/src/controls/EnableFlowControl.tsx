// The four-step enablement flow (bead cross-os-itq).
//
// The steps are declared by the page, so this renders them in order and stops.
// Which step the user is ON is not in the schema, and inferring it from
// status.Running would be the shell inventing a state machine the daemon never
// described — the same reason §3.6c keeps the shell a renderer and not a
// participant. The links a step may carry (aboutLink, trialLink) are page ids
// and belong to App.tsx, which owns navigation; a control that navigated would
// need a hardcoded page id, which is precisely what the discovery rule bans.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function EnableFlowControl(props: ControlProps): ReactElement {
  const { control } = props
  const steps = Array.isArray(control.steps) ? control.steps : []

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note}>
      {steps.length === 0 ? (
        <EmptyState>This page declared an enablement flow with no steps in it.</EmptyState>
      ) : (
        <ol className="ctl-list">
          {steps.map((step, index) => (
            <li className="ctl-item" key={`${index}-${step}`}>
              <span className="ctl-value">
                {index + 1}. {step}
              </span>
            </li>
          ))}
        </ol>
      )}
    </ControlFrame>
  )
}
