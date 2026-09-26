// A page's own prose (bead cross-os-itq).
//
// The Plugins page ships two of these ("New installs enter trial — confirm on
// the Safety page"). There is no data source and nothing to fetch; the text IS
// the control. An empty note still renders its frame and says so, because a
// declared note with no text is a page-authoring slip the user should be able
// to see rather than a silently blank row.
//
// THE EMPTY NOTE IS TWO SENTENCES, which is the reference's empty state split in
// two. menumate's ScreenPacksEmpty (App/UI/PacksScreen.swift:372-405) draws a
// title and a body in different voices — `packs.emptyTitle` "No packs yet" at
// 17pt semibold, `packs.emptyBody` "Browse community packs or import a Git
// repository." at 12.5pt in the second tone (:386-395) — because the reader has
// two different questions and they have two different answers: what is this, and
// what do I do about it. "This note has no text." answered only the first, which
// is why a page that declared an empty note left whoever found it with a
// statement and no way forward. The second sentence here names the second
// question's answer in the same terms the reference's body uses: what the person
// can do, not what the author forgot.

import type { ReactElement } from 'react'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function NoteControl(props: ControlProps): ReactElement {
  const { control } = props
  const text = control.text ?? control.note ?? ''

  return (
    <ControlFrame label={control.label ?? control.id}>
      {text ? (
        <p className="ctl-value">{text}</p>
      ) : (
        <EmptyState>
          This note has no text. The page that declared it left the text out, so there is nothing to
          read here.
        </EmptyState>
      )}
    </ControlFrame>
  )
}
