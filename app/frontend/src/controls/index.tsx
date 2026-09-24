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
import { ActionSettingsControl } from './ActionSettingsControl'
import { AuditListControl } from './AuditListControl'
import { ButtonControl } from './ButtonControl'
import { ChecklistControl } from './ChecklistControl'
import { ConflictResolver } from './ConflictResolver'
import { FileTypeListControl } from './FileTypeListControl'
import { CreditsControl } from './CreditsControl'
import { GateBadgeControl } from './GateBadgeControl'
import { HomeSummaryControl } from './HomeSummaryControl'
import { KeymapEditorControl } from './KeymapEditorControl'
import { LicenseControl } from './LicenseControl'
import { MatrixControl } from './MatrixControl'
import { NoteControl } from './NoteControl'
import { OverridesControl } from './OverridesControl'
import { PackListControl } from './PackListControl'
import { PaletteControl } from './PaletteControl'
import { PipelineTraceControl } from './PipelineTraceControl'
import { PluginDetailControl } from './PluginDetailControl'
import { PluginListControl } from './PluginListControl'
import { ProfileListControl } from './ProfileListControl'
import { RuleBuilderControl } from './RuleBuilderControl'
import { SchemaFormControl } from './SchemaFormControl'
import { ShortcutListControl } from './ShortcutListControl'
import { SwitcherPanelControl } from './SwitcherPanelControl'
import { TrialControl } from './TrialControl'
import { UnsupportedControl } from './UnsupportedControl'
import { VersionControl } from './VersionControl'
import { WizardControl } from './WizardControl'
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
  ['actionSettings', ActionSettingsControl],
  ['auditList', AuditListControl],
  ['button', ButtonControl],
  ['checklist', ChecklistControl],
  ['conflictResolver', ConflictResolver],
  ['credits', CreditsControl],
  ['fileTypeList', FileTypeListControl],
  ['gateBadge', GateBadgeControl],
  ['homeSummary', HomeSummaryControl],
  ['keymapEditor', KeymapEditorControl],
  ['license', LicenseControl],
  ['matrix', MatrixControl],
  ['note', NoteControl],
  ['overrides', OverridesControl],
  ['packList', PackListControl],
  ['palette', PaletteControl],
  ['pipelineTrace', PipelineTraceControl],
  ['pluginDetail', PluginDetailControl],
  ['pluginList', PluginListControl],
  ['profileList', ProfileListControl],
  ['ruleBuilder', RuleBuilderControl],
  ['schemaForm', SchemaFormControl],
  ['shortcutList', ShortcutListControl],
  ['switcherPanel', SwitcherPanelControl],
  ['trial', TrialControl],
  ['version', VersionControl],
  ['wizard', WizardControl],
  ['zoneEditor', ZoneEditorControl],

  // The three spellings earlier pages declared, each mapping to the renderer
  // that replaced it. They are kinds, like every other key here, so nothing
  // about them is a page id — and keeping them costs nothing while a daemon
  // and a shell are free to ship separately: a page that declared `traceList`
  // draws the pipeline renderer rather than landing in UnsupportedControl. The
  // Go side retires them by renaming the `kind` field; the aliases are the
  // grace period, not a second implementation.
  ['enableFlow', WizardControl],
  ['statusCard', HomeSummaryControl],
  ['traceList', PipelineTraceControl],
])

/**
 * The registered kinds, in served order. Exported for the coverage test in
 * renderers.test.tsx, which asserts that no key here is a page id: page ids in
 * this codebase are namespaced and dotted, so a dotted key would be a registry
 * that had started naming screens instead of kinds.
 */
export function rendererKinds(): string[] {
  return [...RENDERERS.keys()]
}

export function renderControl(control: Control, ctx: ControlContext): ReactElement {
  const Renderer = RENDERERS.get(control.kind)
  // An unknown kind is named rather than skipped. Rendering nothing would leave
  // a hole in the page and no way to tell whether the page or the shell is at
  // fault; naming it makes the gap self-describing.
  if (!Renderer) return <UnsupportedControl control={control} ctx={ctx} />
  return <Renderer control={control} ctx={ctx} />
}
