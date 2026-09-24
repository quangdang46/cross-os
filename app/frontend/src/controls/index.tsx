// The control registry and the one function App.tsx calls (bead cross-os-itq).
//
// THE EXPORT CONTRACT. Another agent's App.tsx imports renderControl and
// ControlContext from here and nothing else out of this directory. Everything a
// control can reach — the service, the status, the refresh cadence, the note
// sink — arrives in ControlContext, so a control is a function of what the
// daemon said and never of module-level state.
//
// ReactElement rather than the JSX.Element spelling: those are the same type,
// and this checkout resolves React's JSX namespace only when @types/react is
// installed, which it is not on a machine that has never run npm install. The
// signature App.tsx codes against is unchanged.

import type { ReactElement } from 'react'
import type { Control, ServiceApi, Status } from '../types/controls'
import { AuditListControl } from './AuditListControl'
import { ButtonControl } from './ButtonControl'
import { ChecklistControl } from './ChecklistControl'
import { CreditsControl } from './CreditsControl'
import { EnableFlowControl } from './EnableFlowControl'
import { LicenseControl } from './LicenseControl'
import { MatrixControl } from './MatrixControl'
import { NoteControl } from './NoteControl'
import { OverridesControl } from './OverridesControl'
import { PaletteControl } from './PaletteControl'
import { PluginListControl } from './PluginListControl'
import { SchemaFormControl } from './SchemaFormControl'
import { ShortcutListControl } from './ShortcutListControl'
import { TraceListControl } from './TraceListControl'
import { TrialControl } from './TrialControl'
import { UnsupportedControl } from './UnsupportedControl'
import { VersionControl } from './VersionControl'
import { ZoneEditorControl } from './ZoneEditorControl'
import type { ControlProps } from './common'

export interface ControlContext {
  service: ServiceApi
  status: Status | null
  logs: string[]
  /** Bumped on every refresh; a control re-reads when it changes. */
  refreshToken: number
  note: (message: string) => void
  refresh: () => void
  pageId: string
}

type ControlRenderer = (props: ControlProps) => ReactElement

/**
 * The kind registry — one entry per control KIND.
 *
 * Keys are kinds, never page ids, control ids or plugin ids. Those are data:
 * a plugin shipping a new page must get working UI from this file unchanged
 * (§3.6c's acceptance test is literally "deleting a plugin from the source tree
 * entirely → the shell UI still runs"). A registry keyed on anything but the
 * kind would turn the next page into a code change and quietly break that rule.
 *
 * A Map rather than an object literal because a plain lookup would answer
 * "constructor" or "toString" with something inherited from Object.prototype,
 * and renderControl would then try to draw it.
 */
const RENDERERS = new Map<string, ControlRenderer>([
  ['auditList', AuditListControl],
  ['button', ButtonControl],
  ['checklist', ChecklistControl],
  ['credits', CreditsControl],
  ['enableFlow', EnableFlowControl],
  ['license', LicenseControl],
  ['matrix', MatrixControl],
  ['note', NoteControl],
  ['overrides', OverridesControl],
  ['palette', PaletteControl],
  ['pluginList', PluginListControl],
  ['schemaForm', SchemaFormControl],
  ['shortcutList', ShortcutListControl],
  ['traceList', TraceListControl],
  ['trial', TrialControl],
  ['version', VersionControl],
  ['zoneEditor', ZoneEditorControl],
])

export function renderControl(control: Control, ctx: ControlContext): ReactElement {
  const Renderer = RENDERERS.get(control.kind)
  // An unknown kind is named rather than skipped. Rendering nothing would leave
  // a hole in the page and no way to tell whether the page or the shell is at
  // fault; naming it makes the gap self-describing.
  if (!Renderer) return <UnsupportedControl control={control} ctx={ctx} />
  return <Renderer control={control} ctx={ctx} />
}
