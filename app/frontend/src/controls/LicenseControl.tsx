// The license row (bead cross-os-itq).
//
// The About page does not inline "MIT" as a control field — it names where the
// value comes from (`core:licenseMIT`), and the name after that marker IS the
// license. Deriving it keeps the shell off the license-gate's back: the
// license-gate script (§9.11) fails the job on a copyleft or missing license in
// third_party/, so the answer here moves when the gate says it must, and a
// hand-typed MIT in a React file would not.
//
// A page that declares some other source still renders — it just says what the
// source was instead of guessing a license from the page it sits on.

import type { ReactElement } from 'react'
import { asText } from '../lib/wire'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

const LICENSE_SOURCE = 'core:license'

export function LicenseControl(props: ControlProps): ReactElement {
  const { control } = props
  const source = asText(control.source)
  const name = source.startsWith(LICENSE_SOURCE) ? source.slice(LICENSE_SOURCE.length) : ''

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note}>
      {name ? (
        <p className="ctl-value" title={source}>
          Released under the {name} license.
        </p>
      ) : (
        <EmptyState>
          {source
            ? `This page points at “${source}”, which names no license.`
            : 'This page did not say which license applies.'}
        </EmptyState>
      )}
    </ControlFrame>
  )
}
