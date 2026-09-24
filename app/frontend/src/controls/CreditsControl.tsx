// The credits row (bead cross-os-itq).
//
// app/backend/pages.go is explicit that these entries are "generated from
// third_party ATTRIBUTION.md entries (never hand-maintained)", and that rule
// outlives the wish for a prettier About page. Typing the four repository names
// into a React file would create a second, hand-maintained list that the
// license gate cannot see — the exact drift the gate exists to catch (§9.11) —
// so this control does not carry one.
//
// It also cannot read the files: the entries live in third_party/, outside the
// frontend's module, and nothing on the frozen bridge serves them. The control
// therefore renders the source the page declared and names what is missing,
// which is a defect someone can go and fix. Blanking the row, or inventing four
// entries that will be wrong the moment a fifth port lands, are the two
// alternatives and both are worse.

import type { ReactElement } from 'react'
import { asText } from '../lib/wire'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function CreditsControl(props: ControlProps): ReactElement {
  const { control } = props
  const source = asText(control.source)

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note}>
      <EmptyState>
        CrossOS reuses code from open-source projects. Each one's name, pinned commit and
        license live in its own third_party/ATTRIBUTION.md, and the license gate checks that
        list on every change.
      </EmptyState>
      {source ? <p className="ctl-value">This page draws them from {source}.</p> : null}
    </ControlFrame>
  )
}
