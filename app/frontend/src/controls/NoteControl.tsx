// A page's own prose (bead cross-os-itq).
//
// The Plugins page ships two of these ("New installs enter trial — confirm on
// the Safety page"). There is no data source and nothing to fetch; the text IS
// the control. An empty note still renders its frame and says so, because a
// declared note with no text is a page-authoring slip the user should be able
// to see rather than a silently blank row.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function NoteControl(props: ControlProps): ReactElement {
  const { control } = props
  const text = control.text ?? control.note ?? ''

  return (
    <ControlFrame label={control.label ?? control.id}>
      {text ? <p className="ctl-value">{text}</p> : <EmptyState>This note has no text.</EmptyState>}
    </ControlFrame>
  )
}
