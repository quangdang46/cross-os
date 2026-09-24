// A control kind this build has no renderer for (bead cross-os-itq).
//
// The three Finder-page kinds (packList, actionSettings, gateBadge) land here
// today, and so would any kind a future page invents. That is acceptable only
// because this row NAMES the kind and says why it is empty — the rule is that
// a missing renderer is a defect someone can find, not a page that renders
// blank and leaves the user wondering whether CrossOS is broken.
//
// It is deliberately not styled as an error. Nothing failed at runtime; the
// shell simply cannot draw what the page declared, and a row in the error style
// would send users looking for a permission problem that does not exist.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function UnsupportedControl(props: ControlProps): ReactElement {
  const { control } = props
  const label = control.label ?? control.id

  return (
    <ControlFrame label={label} note={control.note}>
      <p className="ctl-value">
        Declared kind: <span className="ctl-chip">{control.kind || '(none)'}</span>
      </p>
      <EmptyState>
        {label} is a “{control.kind || 'unnamed'}” control, and this build has no renderer for
        that kind, so there is nothing to draw here.
      </EmptyState>
    </ControlFrame>
  )
}
