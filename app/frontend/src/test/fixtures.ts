// The shell's shared fixtures: one stateful stub of the bound ServiceApi, the
// pages the host serves, and the rows those pages read.
//
// THE STUB IS STATEFUL ON PURPOSE. The shell's central promise is a SEQUENCE —
// a fresh profile lands on the wizard, a finished one lands on Home — and a
// fixture that answered every call from a fixed table could never be asked
// whether the sequence holds. So this stub keeps the machine's state: which
// profile is applied, whether the tap is installed, which extensions are on,
// which rules a person has written, and what the router decided. The pages it
// serves are sorted the way app/backend/host.go sorts them (Order, then the
// first-run flag while onboarding is still open, then the id), so the landing
// page is the HOST's answer arriving over the wire and never a fact the shell
// re-decides for itself.
//
// ONBOARDING ENDS ON A WRITE, AND ONLY THAT ONE WRITE. That mirrors the Go side,
// where the step machine is derived from the readiness rows
// (the onboardingState verb derives its steps from the readiness one) but the finished
// flag is the one part it cannot re-derive — a user who skipped a step this
// machine happens to satisfy would be sent back through it on every launch, so
// the sentence "I am finished" is latched by the onboardingComplete verb and nothing
// else writes it. A Readiness() that flipped the flag as a side effect of being
// READ would make a poll a write, which is the one thing a read must never be:
// the shell would then finish setup on a machine nobody finished.
//
// Ids here are invented, for the reason harness.tsx states: a page id is daemon
// vocabulary (§3.6c), and pasting a real one into a fixture would put a page id
// back into src/, which is what the shell's own rule test forbids.

import type {
  AppRow,
  AuditRow,
  CommandRow,
  ConflictRow,
  Control,
  MatrixRow,
  OnboardingRow,
  OnboardingStep,
  OverrideRow,
  PluginMetaRow,
  PluginState,
  ProfileCapabilityRow,
  ProfileRow,
  ReadinessRow,
  SchemaRow,
  ServiceApi,
  ShortcutRow,
  StageRow,
  Status,
  TraceRow,
  TrialState,
  UserRuleRow,
  WizardStep,
  ZoneRow,
} from '../types/controls'
// The two action ids the Switcher page declares, imported from the registry
// rather than spelled here: an action id is a permission token the daemon
// checks, so a fixture that wrote its own copy could pass against an id nothing
// is bound under.
import { SWITCHER_FOCUS, SWITCHER_WAIT } from '../controls/actions'

// --- the pages ---------------------------------------------------------------

/** One page as the host serves it, with the nav fields a test can vary. */
export function servedPage(fields: {
  id: string
  title: string
  group?: string
  order?: number
  firstRun?: boolean
  description?: string
  controls?: Control[]
}): unknown {
  return {
    ID: fields.id,
    Title: fields.title,
    Group: fields.group ?? '',
    Order: fields.order ?? 0,
    FirstRun: fields.firstRun ?? false,
    Schema: {
      type: 'page',
      description: fields.description ?? '',
      ...(fields.firstRun ? { firstRun: true } : {}),
      controls: fields.controls ?? [],
    },
  }
}

/**
 * The first-run flow, declared ONCE and read by two consumers that must never
 * disagree: the page below, which ships the steps a reader sees, and the stub's
 * onboardingState, which ships the verdicts. A wizard that listed its steps in
 * one place and judged a different list in another is precisely the two
 * cursors-on-one-flow bug the daemon row exists to prevent — and here it would
 * be the FIXTURE that disagreed with itself, which is worse, because the test
 * would be asserting against an answer nothing ever produced.
 *
 * Each step carries the daemon's id, so a done mark is a JOIN rather than a
 * position guess, and chooseProfile declares the control KIND whose cards it
 * draws: the Windows profile is picked from inside the flow rather than on a
 * separate page. The step ids are the daemon's own tokens (welcome, enable,
 * settings, verify) plus the one this flow adds; they are read as data and never
 * branched on by the shell.
 */
const ONBOARDING_STEPS: WizardStep[] = [
  { id: 'welcome', label: 'Welcome' },
  { id: 'chooseProfile', label: 'Pick a Windows profile', body: 'profileList' },
  { id: 'enable', label: 'Enable per extension' },
  { id: 'settings', label: 'Open System Settings' },
  { id: 'verify', label: 'Verify ready' },
]

/** The first-run page, in the two shapes the host serves it in. */
export const FIRST_RUN_CONTROL: Control = {
  kind: 'wizard',
  id: 'onboard',
  label: 'Welcome to CrossOS',
  steps: ONBOARDING_STEPS,
  actions: ['plugin.enable', 'permissions.openSettings', 'permissions.verify'],
}

/**
 * The whole nav, in the order a reader should meet it. One page per area of the
 * product, carrying the control kinds that area owns, so the golden path can
 * walk it without inventing a screen the daemon does not serve.
 */
export function productPages(): unknown[] {
  return [
    servedPage({
      id: 'welcome',
      title: 'Welcome',
      group: 'home',
      firstRun: true,
      description: 'Get CrossOS working in four steps.',
      controls: [
        FIRST_RUN_CONTROL,
        { kind: 'checklist', id: 'readiness', label: 'Readiness', source: 'daemon:readiness', items: ['keyboard', 'windows', 'finder'] },
      ],
    }),
    servedPage({
      id: 'dashboard',
      title: 'Home',
      group: 'home',
      description: 'What CrossOS is doing right now.',
      controls: [
        { kind: 'homeSummary', id: 'status', label: 'What is on' },
        { kind: 'checklist', id: 'readiness', label: 'Readiness', source: 'daemon:readiness', items: ['keyboard', 'windows', 'finder'] },
      ],
    }),
    servedPage({
      id: 'bundles',
      title: 'Profiles',
      group: 'home',
      order: 10,
      description: 'Start from a setup rather than a list of switches.',
      controls: [
        { kind: 'profileList', id: 'profiles', label: 'Profiles', source: 'daemon:profiles', note: 'Applying a profile enables its capabilities in one step.' },
        { kind: 'note', id: 'profileNote', text: 'A profile is a bundle, not an extension.' },
      ],
    }),
    servedPage({
      id: 'keys',
      title: 'Keyboard',
      group: 'shortcuts',
      order: 20,
      description: 'Which shortcuts do what, per app.',
      controls: [
        { kind: 'matrix', id: 'matrix', label: 'Behavior matrix', source: 'daemon:behaviorMatrix' },
        { kind: 'overrides', id: 'overrides', label: 'App overrides', source: 'daemon:appOverrides' },
      ],
    }),
    servedPage({
      id: 'chording',
      title: 'Windows',
      group: 'shortcuts',
      order: 30,
      description: 'Window shortcuts and snap zones.',
      controls: [
        { kind: 'shortcutList', id: 'shortcuts', label: 'Window shortcuts', source: 'daemon:windowShortcuts', editable: true },
        { kind: 'zoneEditor', id: 'zones', label: 'Snap zones', source: 'daemon:snapZones' },
      ],
    }),
    servedPage({
      id: 'switch',
      title: 'Switcher',
      group: 'shortcuts',
      order: 35,
      description: 'The windows the switcher can raise, in the order it will raise them.',
      controls: [
        {
          kind: 'switcherPanel',
          id: 'windows',
          label: 'Open windows',
          source: 'daemon:windows',
          rowAction: SWITCHER_FOCUS,
          actions: [SWITCHER_WAIT],
          note: 'One row per window, named by its title. Clicking a row raises it.',
        },
      ],
    }),
    servedPage({
      id: 'remap',
      title: 'Shortcuts',
      group: 'shortcuts',
      order: 40,
      description: 'Record a shortcut, choose what it runs, resolve what it collides with.',
      controls: [
        { kind: 'keymapEditor', id: 'keymap', label: 'Record a shortcut', source: 'daemon:behaviorMatrix' },
        { kind: 'conflictResolver', id: 'conflicts', label: 'Conflicts' },
        { kind: 'ruleBuilder', id: 'rules', label: 'Rule builder' },
      ],
    }),
    servedPage({
      id: 'menu',
      title: 'Explorer',
      group: 'shortcuts',
      order: 50,
      description: 'Finder menu items from extension packs.',
      controls: [
        { kind: 'packList', id: 'packs', source: 'daemon:finderPacks' },
        { kind: 'actionSettings', id: 'actionSettings', source: 'daemon:packAction', fields: ['placement', 'variants', 'timeoutSeconds'] },
        { kind: 'gateBadge', id: 'levelB', source: 'daemon:packActionGate' },
      ],
    }),
    servedPage({
      id: 'log',
      title: 'Activity',
      group: 'activity',
      order: 60,
      description: 'What CrossOS did and why.',
      controls: [
        { kind: 'button', id: 'panicStop', label: 'PANIC STOP', action: 'safety.panicStop' },
        { kind: 'traceList', id: 'trace', source: 'daemon:traces', format: 'Key → App → Rule → Intent → Action' },
      ],
    }),
    servedPage({
      id: 'observe',
      title: 'Observe',
      group: 'activity',
      order: 70,
      description: 'Watch what CrossOS sees, without acting on it.',
      controls: [
        { kind: 'button', id: 'observeToggle', label: 'Turn Observe on', action: 'observe.set' },
        { kind: 'traceList', id: 'events', source: 'daemon:traces', format: 'Key → App → Rule → Intent → Action' },
      ],
    }),
    servedPage({
      id: 'extensions',
      title: 'Extensions',
      group: 'advanced',
      order: 80,
      description: 'Extensions installed on this machine.',
      controls: [
        { kind: 'pluginList', id: 'plugins', source: 'daemon:plugins', rowHealth: true },
        { kind: 'note', id: 'trialNote', text: 'New installs enter trial. Confirm or roll back on the Safety page.' },
      ],
    }),
    servedPage({
      id: 'safety',
      title: 'Safety',
      group: 'advanced',
      order: 100,
      description: 'Stop everything instantly, review what CrossOS changed.',
      controls: [
        { kind: 'button', id: 'resume', label: 'Re-enable interception', action: 'safety.resume' },
        { kind: 'trial', id: 'trialCountdown', label: 'New integration trial', source: 'daemon:trialCountdown' },
        { kind: 'auditList', id: 'ownership', label: 'What CrossOS created', source: 'daemon:ownershipAudit' },
      ],
    }),
    servedPage({
      id: 'about',
      title: 'About',
      group: 'advanced',
      order: 110,
      description: 'Version, license and credits.',
      controls: [
        { kind: 'version', id: 'version', source: 'daemon:version' },
        { kind: 'license', id: 'license', source: 'daemon:licenseMIT' },
        { kind: 'credits', id: 'credits', source: 'daemon:attributionEntries' },
        { kind: 'schemaForm', id: 'pluginSettings', source: 'daemon:pluginSchemas' },
        { kind: 'palette', id: 'palette', source: 'daemon:commands' },
      ],
    }),
  ]
}

// --- the machine -------------------------------------------------------------

interface Machine {
  /** True once a verify saw every check green; the flag stops leading after. */
  onboarded: boolean
  /** The applied profile id, '' when none has been applied. */
  profile: string
  /** The keyboard tap: what the Accessibility permission buys. */
  interception: boolean
  /**
   * Whether the recorder is dry-running, and what it is keeping. Held as state
   * rather than echoed, so ObserveState is a READ of what SetObserve changed —
   * the same distinction the real recorder makes, and the one a fire-and-forget
   * toggle would lose.
   */
  observing: boolean
  recordMode: string
  /**
   * Chords two rules both claim, as the decision path ranks them. Held as
   * state so a test can put a collision here that nobody has pressed — which
   * is the case a trace-derived path could not produce at all, and the one
   * worth warning a person about.
   */
  conflicts: ConflictRow[]
  /**
   * Readiness reads still to answer "the tap is not back" AFTER a grant landed.
   *
   * This is the gap the wizard's settle exists for, modelled rather than
   * described: on a real machine the person flips the switch in System Settings
   * and comes straight back, and the daemon has not reinstalled the tap yet, so
   * the first reads are honestly red. A fixture where a grant turned the rows
   * green in the same tick could not tell a wizard that WAITS from one that
   * reads once and happens to be right, which is the only distinction the
   * hand-off turn was about. Zero means the tap is already back.
   */
  tapReinstallsIn: number
  extensions: Record<string, boolean>
  rules: UserRuleRow[]
  decisions: TraceRow[]
  /** The open windows the switcher lists, most-recently-used first. */
  windows: WindowRow[]
  /** A bound method to reject, by name. Absent means the call resolves. */
  faults: Record<string, unknown>
  /** Every call the shell made, in order, for assertions about the sequence. */
  calls: string[]
}

/**
 * One window as the daemon's switcher row carries it. The shape is declared
 * here rather than imported from the renderer: the row is the DAEMON's wire
 * shape (core/cmd/crossos/switcher.go, switcherRow), so a fixture that took it
 * from the control it draws would only be checking the control against itself.
 */
interface WindowRow {
  window_id: string
  app_id: string
  title: string
  index: number
  selected: boolean
  skippable: boolean
}

function freshWindows(): WindowRow[] {
  return [
    { window_id: '412', app_id: 'com.apple.finder', title: 'Downloads', index: 0, selected: false, skippable: false },
    { window_id: '881', app_id: 'com.apple.Terminal', title: 'crossos — zsh', index: 1, selected: true, skippable: false },
    { window_id: '207', app_id: 'com.apple.Safari', title: '', index: 2, selected: false, skippable: true },
  ]
}

function fresh(): Machine {
  return {
    onboarded: false,
    profile: '',
    interception: false,
    observing: false,
    recordMode: 'metadata-only',
    conflicts: [],
    tapReinstallsIn: 0,
    extensions: { 'window-keys': false, 'finder-actions': false },
    rules: [],
    decisions: [],
    windows: freshWindows(),
    faults: {},
    calls: [],
  }
}

/** The machine the stub answers from. Reset it between tests. */
export const machine: Machine = fresh()

/** Puts the machine back to a machine that has never been set up. */
export function reset(): void {
  Object.assign(machine, fresh())
}

/** Makes one bound method reject, so a failing source can be exercised alone. */
export function failOn(method: string, why: string): void {
  machine.faults[method] = new Error(why)
}

/**
 * grantAccessibility is the person finishing the System Settings step, and it
 * is where the reinstall lag lives.
 *
 * `reads` is how many readiness answers the daemon will still give as not-ready
 * before the tap is back — 1 for the common case where the reader has just come
 * back from System Settings, 0 for a machine that has been sitting long enough
 * that the tap is already up. The default is 1 rather than 0 because a fixture
 * that made the grant instant would let a control that reads ONCE pass a test
 * whose whole claim is that reading once is not enough. TWO tests now make that
 * claim — controls/wizardSettle.test.tsx on the wait itself, and the e2e golden
 * path on the whole flow through the shell — so the lag is load-bearing off the
 * wizard page as well as on it.
 */
export function grantAccessibility(reads = 1): void {
  machine.interception = true
  machine.tapReinstallsIn = reads
}

function answer<T>(name: string, value: () => T): Promise<T> {
  machine.calls.push(name)
  if (name in machine.faults) return Promise.reject(machine.faults[name])
  // Through an executor, not Promise.resolve(value()): a bound call that throws
  // on its own argument must REJECT like a real one, not throw synchronously
  // past the caller that awaited it.
  return new Promise<T>((resolve) => resolve(value()))
}

// --- the rows ----------------------------------------------------------------

/** The behaviour matrix the daemon serves, before anyone edits it. */
function matrixRows(): MatrixRow[] {
  return [
    { rule_id: 'win.switch', plugin: 'window-keys', action: 'window.switch', keys: 'Ctrl+Tab', contexts: [], enabled: machine.extensions['window-keys'] },
    { rule_id: 'win.snapLeft', plugin: 'window-keys', action: 'window.snapLeft', keys: 'cmd+Left', contexts: ['finder'], enabled: true },
    { rule_id: 'finder.rename', plugin: 'finder-actions', action: 'finder.rename', keys: 'F2', contexts: [], enabled: machine.extensions['finder-actions'] },
  ]
}

function readinessRows(): ReadinessRow[] {
  // The grant is in, the reinstall is not: each read spends one of the pending
  // answers, so the rows turn green on a LATER read rather than the same one.
  // Spent here, at the single place every readiness answer is derived, so the
  // onboarding row and the readiness verb cannot disagree about how many reads
  // are left — the same rule the daemon's two handlers share one function for.
  const reinstalling = machine.tapReinstallsIn > 0
  if (reinstalling) machine.tapReinstallsIn -= 1
  const tap = machine.interception && !reinstalling
  const keys = machine.extensions['window-keys']
  const finder = machine.extensions['finder-actions']
  const tapError = 'grant Accessibility in System Settings'
  return [
    // The daemon row a page did not ask for: the served list is always longer
    // than the declared one, and the checklist says so rather than dropping it.
    { id: 'daemon', label: 'CrossOS daemon', ready: tap, detail: tap ? '' : tapError },
    { id: 'keyboard', label: 'Keyboard interception', ready: tap && keys, detail: tap && keys ? '' : tap ? 'window-keys is switched off' : tapError },
    { id: 'windows', label: 'Windows shortcuts', ready: tap && keys, detail: tap && keys ? '' : tap ? 'window-keys is switched off' : tapError },
    { id: 'finder', label: 'Finder shortcuts', ready: tap && finder, detail: tap && finder ? '' : tap ? 'finder-actions is switched off' : tapError },
  ]
}

/**
 * The daemon's step machine, derived the way the onboardingState handler derives it
 * (core/cmd/crossos/pagedata.go, handleOnboardingState): every verdict is a
 * function of live state, the flag overrides all of them, and the cursor is the
 * FIRST step not derived as done rather than a counter kept beside the list.
 *
 * Deriving it here rather than storing it is what makes this a fixture of the
 * product's rule and not of a button. Nothing in this function is a verdict the
 * test asserted into existence — change what the machine has actually done and
 * the steps move on their own, which is the whole claim the wizard renders.
 */
function onboardingState(): OnboardingRow {
  const rows = readinessRows()
  const ready = rows.filter((row) => row.ready).length
  const allReady = rows.length > 0 && ready === rows.length
  const keyboard = rows.find((row) => row.id === 'keyboard')
  const anyExtension = Object.values(machine.extensions).some(Boolean)
  // A profile IS the product: applying one switches on what it bundles, so a
  // picked profile is what makes the per-extension step true. Deriving it the
  // other way round would leave the flow asking for a switch the pick already
  // performed.
  const chosen = machine.profile !== ''

  const done: Record<string, boolean> = {
    welcome: chosen || anyExtension || keyboard?.ready === true || allReady,
    chooseProfile: chosen,
    enable: chosen || anyExtension,
    settings: keyboard?.ready === true,
    verify: allReady,
  }
  // Detail is WHY a step is not done, so the wizard can say what to do instead
  // of only that it is waiting. The daemon omits it on a done step, so a shell
  // that printed a blank line there would be printing a sentence nobody wrote.
  const detail: Record<string, string> = {
    chooseProfile: 'no profile is applied yet',
    enable: 'no extension is switched on yet',
    settings: keyboard?.detail ?? '',
    verify: `${rows.length - ready} of ${rows.length} checks are not ready`,
  }

  const steps: OnboardingStep[] = []
  let current = 'done'
  for (const step of ONBOARDING_STEPS) {
    const id = step.id ?? ''
    const isDone = machine.onboarded || done[id] === true
    steps.push({ id, label: step.label, done: isDone, detail: isDone ? undefined : detail[id] })
    if (!isDone && current === 'done') current = id
  }
  // The flag is the one part that cannot be re-derived, so it is the one that
  // ends the cursor: a machine the person declared finished has no step left
  // even though its readiness rows still say what they say.
  if (machine.onboarded) current = 'done'

  return { completed: machine.onboarded, current_step: current, steps, readiness: rows, ready, total: rows.length }
}

function capability(id: string, label: string, plugin: string, rule: string, total: number): ProfileCapabilityRow {
  const on = machine.extensions[plugin] ? total : 0
  return {
    id,
    label,
    plugin,
    available: true,
    rule_ids: rule === '' ? [] : [rule],
    enabled: on,
    total,
    live: true,
  }
}

function profileRows(): ProfileRow[] {
  return [
    {
      id: 'windows11',
      label: 'Windows 11',
      description: 'Windows keys, snap zones and Explorer actions.',
      active: machine.profile === 'windows11',
      capabilities: [
        capability('keys', 'Windows keys', 'window-keys', 'win.switch', 3),
        capability('finder', 'Finder actions', 'finder-actions', 'finder.rename', 1),
        // A capability that ships no rules of its own: it must read 0 of 0
        // rather than borrowing another capability's count.
        { id: 'pointer', label: 'Pointer keys', plugin: 'window-keys', available: false, reason: 'no rule or profile capability covers it yet', rule_ids: [], enabled: 0, total: 0, live: true },
      ],
    },
    {
      id: 'plain',
      label: 'Plain desktop',
      description: 'Nothing turned on yet.',
      active: machine.profile === 'plain',
      capabilities: [],
    },
  ]
}

function appRows(): AppRow[] {
  return [
    { BundleID: 'com.apple.finder', Executable: '/System/Library/CoreServices/Finder.app', PID: 401, DisplayName: 'Finder', AppMode: 'finder', Category: 'system' },
    { BundleID: 'com.apple.Terminal', Executable: '/System/Applications/Utilities/Terminal.app', PID: 812, DisplayName: 'Terminal', AppMode: 'terminal', Category: 'system' },
  ]
}

function pluginStates(): PluginState[] {
  return [
    { ID: 'window-keys', Enabled: machine.extensions['window-keys'], Healthy: machine.extensions['window-keys'] ? 'healthy' : 'disabled' },
    { ID: 'finder-actions', Enabled: machine.extensions['finder-actions'], Healthy: machine.extensions['finder-actions'] ? 'healthy' : 'disabled' },
  ]
}

function statusPayload(): Status {
  return {
    Running: true,
    SafeMode: false,
    Killed: false,
    Plugins: pluginStates(),
    Interception: machine.interception,
    TapError: machine.interception ? '' : 'grant Accessibility in System Settings',
    Version: '0.4.0',
  }
}

function metaRows(): PluginMetaRow[] {
  return [
    { id: 'window-keys', name: 'Window keys', version: '1.2.0', permissions: ['keyboard.tap', 'window.ax.read'], loaded: machine.extensions['window-keys'] },
    { id: 'finder-actions', name: '', version: '', permissions: [], loaded: false, reason: 'no manifest is loaded at runtime' },
  ]
}

function zoneRows(): ZoneRow[] {
  return [
    { id: 'left', name: 'Left half', x: 0, y: 0, w: 0.5, h: 1 },
    { id: 'right', name: 'Right half', x: 0.5, y: 0, w: 0.5, h: 1 },
  ]
}

function auditRows(): AuditRow[] {
  return machine.profile === ''
    ? []
    : [{ resource: 'login-item', id: 'com.crossos.agent', owner: 'crossos', created_at: new Date(Date.now() - 60_000).toISOString() }]
}

function trialPayload(): TrialState {
  // State "none" with an empty plugin is a real answer, not a broken countdown.
  return machine.profile === '' ? { plugin: '', state: 'none', remaining_ms: 0, timeout_ms: 0 } : { plugin: 'window-keys', state: 'running', remaining_ms: 4 * 60_000, timeout_ms: 5 * 60_000 }
}

function schemaRows(): SchemaRow[] {
  return [
    {
      plugin: 'window-keys',
      title: 'Window keys',
      schema: {
        type: 'object',
        properties: { deadzone: { type: 'number', minimum: 0, maximum: 20, default: 6 } },
      },
    },
  ]
}

function commandRows(): CommandRow[] {
  return machine.extensions['finder-actions'] ? [{ id: 'finder.reveal', title: 'Reveal in Finder', plugin: 'finder-actions' }] : []
}

function overrideRows(): OverrideRow[] {
  return []
}

function shortcutRows(): ShortcutRow[] {
  return [
    { id: 'win.switch', keys: 'Ctrl+Tab', action: 'window.switch', enabled: machine.extensions['window-keys'] },
    { id: 'win.snapLeft', keys: 'cmd+Left', action: 'window.snapLeft', enabled: true },
  ]
}

/** The chord in the daemon's own spelling, which is what a stored rule carries. */
function chordOf(rule: UserRuleRow): string {
  return [...rule.modifiers, rule.key].join('+')
}

/**
 * The router's verdict for one press, as rule.Resolve reports it. The shell
 * never ranks anything: it reads the winner and the losers this records, and
 * the control that offers to switch a losing rule off is offering to change
 * which rule is ELIGIBLE, not to pick a winner itself.
 *
 * A rule the person wrote outranks a bundled one, and among person rules the
 * narrower scope wins — so an app-scoped rule displaces a global one on the
 * same chord. The decision is re-recorded on every press of the same chord,
 * because a ranking can change and the newest verdict is the one the router
 * would reach now.
 *
 * WHY A TEST SUPPLIES THE PRESS. In the running product the router decides on a
 * kernel tap event, and there is no bound call that can synthesise one — the
 * absence is the point, the same way there is no bound call that opens System
 * Settings. So the fixture supplies the one physical event a shell test cannot
 * produce, and everything after it (the stages, the winner, the losers) is what
 * the shell is being asked to read back.
 */
export function press(chord: string, front = 'com.apple.finder'): void {
  const written = machine.rules.filter((rule) => chordOf(rule) === chord)
  const bundled = matrixRows().filter((row) => row.keys === chord)
  const ranked = [...written].sort((a, b) => b.specificity - a.specificity)
  const winner = ranked[0]?.id ?? bundled[0]?.rule_id ?? ''
  const losers = [
    ...ranked.slice(1).map((rule) => rule.id),
    ...bundled.filter((row) => row.rule_id !== winner).map((row) => row.rule_id),
  ]
  const stages: StageRow[] = [
    { stage: 'event', detail: `${chord} from built-in` },
    { stage: 'context', detail: `front app ${front}` },
    { stage: 'rule', detail: losers.length === 0 ? `${winner} matched` : `${winner} matched, outranked ${losers.join(', ')}` },
    { stage: 'intent', detail: ranked[0]?.capability ?? bundled[0]?.action ?? 'no intent' },
    { stage: 'action', detail: ranked[0]?.capability ?? bundled[0]?.action ?? 'no action' },
  ]
  machine.decisions = machine.decisions.filter((row) => row.event.keys !== chord)
  // The router just resolved this chord, so the contest is a fact about the
  // machine and not only about this decision. The daemon's own conflict source
  // compiles the rule table independently of whether anyone has pressed
  // anything — but a test that only ever presses would otherwise never see a
  // chord reported, and the resolver is exactly the surface that must not wait
  // for a keystroke.
  if (losers.length > 0) {
    machine.conflicts = [
      ...machine.conflicts.filter((row) => row.keys !== chord),
      {
        keys: chord,
        winner,
        losers,
        rules: [winner, ...losers].map((rule_id) => {
          // A rule a person wrote belongs to no plugin, and saying so is the
          // honest reading — the daemon's own claim carries the owning plugin
          // for a builtin rule and nothing for a user-authored one, so the
          // fixture resolves it the same way: a builtin's plugin when the
          // matrix table names one, and 'yours' when the rule is not in it.
          const written = machine.rules.find((r) => r.id === rule_id)
          const builtin = bundled.find((r) => r.rule_id === rule_id) ?? matrixRows().find((r) => r.rule_id === rule_id)
          return {
            rule_id,
            plugin: builtin?.plugin ?? 'yours',
            action: written?.capability ?? builtin?.action ?? rule_id,
          }
        }),
      },
    ]
  }
  machine.decisions.push({
    at: new Date().toISOString(),
    decision: winner === '' ? 'pass' : 'replace',
    event: { keys: chord, source: 'keyboard', device: 'built-in', key_code: 45 },
    context: { app_id: front, app_mode: front.split('.').pop() ?? '' },
    winner,
    losers,
    intent: stages[3].detail,
    action: stages[4].detail,
    stages,
    params: '',
  })
}

// --- the served order --------------------------------------------------------

/**
 * The host's sort, in one function: Order first, then the first-run page while
 * onboarding is open, then the id. Modelled here because the SHELL does not
 * sort — it renders the order it was served — so the landing page is the
 * host's answer and a fixture that did not sort would be testing a shell
 * behaviour the shell does not have.
 */
/**
 * The nav order, mirrored from app/backend/host.go's sort. This is DELIBERATE
 * and is not a second implementation of it to be tidied away.
 *
 * App.tsx resolves the landing page as pages[0] and does no ordering of its
 * own, so the served order IS the product contract under test. productPages()
 * lists welcome first, and the id tiebreak below is what puts the first-run
 * page in front while the profile is fresh. Serving declaration order instead
 * would make firstRun.test.tsx's landing assertion unpassable — and that
 * assertion is about the wizard stopping leading, which is the bug this whole
 * file exists to help catch.
 *
 * The Go Host owns this ordering for real (cross-os-tsd, whose seed reads the
 * daemon's own answer at startup). If the two ever disagree, the Go side is
 * right and this mirror is the thing that drifted.
 */
function servedInOrder(pages: unknown[]): unknown[] {
  const rank = (page: Record<string, unknown>): [number, number, string] => [
    page.Order as number,
    machine.onboarded || !(page.FirstRun as boolean) ? 1 : 0,
    page.ID as string,
  ]
  return [...pages].sort((a, b) => {
    const [ao, af, ai] = rank(a as Record<string, unknown>)
    const [bo, bf, bi] = rank(b as Record<string, unknown>)
    if (ao !== bo) return ao - bo
    if (af !== bf) return af - bf
    return ai < bi ? -1 : ai > bi ? 1 : 0
  })
}

// --- the stub ----------------------------------------------------------------

/**
 * The control-facing surface, checked against ServiceApi. Kept separate from
 * the three calls the shell makes itself so the annotation below still bites:
 * an object literal cast through `unknown` would stop checking anything.
 */
const controlSurface: ServiceApi = {
  GetStatus: () => answer('GetStatus', statusPayload),

  TogglePlugin: (id, enabled) =>
    answer('TogglePlugin', () => {
      if (!(id in machine.extensions)) throw new Error(`no extension named ${id}`)
      machine.extensions[id] = enabled
    }),

  PanicStop: () =>
    answer('PanicStop', () => {
      machine.interception = false
      return { stopped: ['tap', 'plugin actions'] }
    }),
  Resume: () =>
    answer('Resume', () => {
      // What the Accessibility permission buys: the tap back, and the rows
      // derived from it green on the next readiness read. NO reinstall lag here,
      // and the difference is deliberate: Resume is the daemon reinstalling its
      // own tap, which it can do in the call. grantAccessibility is a PERSON
      // coming back from System Settings, which it cannot — that is the whole
      // gap the wizard's settle is for, so modelling it on both would test the
      // wait against a case that has nothing to wait for.
      machine.interception = true
      return { resumed: ['tap'] }
    }),
  ResetEverything: () => answer('ResetEverything', () => {
    // The call log is the test's own observer, not daemon state, so a reset
    // does not un-observe the call that asked for it.
    const calls = machine.calls
    reset()
    machine.calls = calls
    return []
  }),

  BeginTrial: (pluginID) => answer('BeginTrial', () => `trial started for ${pluginID}`),
  ConfirmTrial: () => answer('ConfirmTrial', () => 'trial kept'),
  RollbackTrial: () => answer('RollbackTrial', () => 'trial rolled back'),

  SetRuleEnabled: (ruleID, enabled) =>
    answer('SetRuleEnabled', () => {
      const row = matrixRows().find((entry) => entry.rule_id === ruleID)
      if (row) row.enabled = enabled
      const user = machine.rules.find((entry) => entry.id === ruleID)
      if (user) user.emit = enabled
      return true
    }),
  Shortcuts: () => answer('Shortcuts', shortcutRows),
  SetShortcuts: (rows) => answer('SetShortcuts', () => rows.length),

  GetMatrix: () => answer('GetMatrix', matrixRows),
  GetOverrides: () => answer('GetOverrides', overrideRows),
  SetOverride: (app, ruleID, enabled) =>
    answer('SetOverride', () => ({ app, rule_id: ruleID, action: '', keys: '', enabled })),
  GetZones: () => answer('GetZones', zoneRows),
  SetZones: (zones) => answer('SetZones', () => zones.length),
  Commands: () => answer('Commands', commandRows),
  PluginSchemas: () => answer('PluginSchemas', schemaRows),
  OwnershipAudit: () => answer('OwnershipAudit', auditRows),
  TrialState: () => answer('TrialState', trialPayload),
  // A READ, and only a read. The step machine is derived from these rows by
  // OnboardingState below, and the finished flag is written by CompleteOnboarding
  // and nothing else — so a verify that saw every row green reports what it
  // found and leaves the flag for the person to set. A Readiness() that flipped
  // it would make polling a write, and a wizard whose finish button appeared
  // because a page happened to re-read a source would be a wizard nobody
  // finished.
  Readiness: () => answer('Readiness', () => readinessRows()),
  OnboardingState: () => answer('OnboardingState', () => onboardingState()),
  CompleteOnboarding: () =>
    answer('CompleteOnboarding', () => {
      machine.onboarded = true
    }),
  // Opens the Accessibility pane and grants nothing. The grant is made by a
  // person in another application, which is why the fixture's own comment
  // called this a write the e2e clicks "to prove the button is a write rather
  // than a refusal" — but it is a write to nothing the test can observe, and
  // the step's verdict stays the daemon's re-derivation below. A stub that set
  // machine.onboarded here would be a shell inventing a granted permission,
  // which is the one thing this step must never do.
  //
  // The Go method answers with an error and nothing else, so the surface does
  // too: returning the daemon's {opened, pane} would be the shell knowing more
  // about the call's outcome than the Service publishes.
  OpenSystemSettings: () => answer('OpenSystemSettings', () => undefined),

  // Observe mode. The write stores and the read reports the store, so a test
  // that flips the toggle and then reads the state is exercising the same
  // round trip the daemon does — and a control that labelled itself from its
  // own click instead would still pass a fire-and-forget test.
  SetObserve: (on) =>
    answer('SetObserve', () => {
      machine.observing = on
    }),
  ObserveState: () =>
    answer('ObserveState', () => ({ observe: machine.observing, mode: machine.recordMode })),

  // A READ, never a derivation: the daemon compiles the contested rules into a
  // real router and answers with what it would pick, so the editor is shown the
  // ranking rather than making one. The fixture holds the answer so a test can
  // place a collision nobody pressed.
  Conflicts: () => answer('Conflicts', () => machine.conflicts),

  Profiles: () => answer('Profiles', profileRows),
  ApplyProfile: (profileID) =>
    answer('ApplyProfile', () => {
      if (profileID === 'windows11') {
        // A profile is the product: one write turns on everything it bundles,
        // which is why the wizard never asks for eleven switches.
        machine.extensions['window-keys'] = true
        machine.extensions['finder-actions'] = true
      }
      machine.profile = profileID
      return { profile: profileID, enabled: Object.keys(machine.extensions).filter((id) => machine.extensions[id]) }
    }),
  Traces: () => answer('Traces', () => [...machine.decisions]),
  PluginMeta: () => answer('PluginMeta', metaRows),
  Apps: () => answer('Apps', appRows),
  UserRules: () => answer('UserRules', () => [...machine.rules]),
  SetUserRule: (rule) =>
    answer('SetUserRule', () => {
      const stored: UserRuleRow = {
        ...rule,
        id: rule.id || `user.${machine.rules.length + 1}`,
        // Derived by the daemon on every write, so the stored row carries the
        // derivation rather than whatever the editor happened to send.
        chord: chordOf(rule),
        action: rule.capability,
        priority: rule.app_ids.length > 0 ? 20 : 10,
        specificity: rule.app_ids.length > 0 ? 2 : 1,
        scope: rule.app_ids.length > 0 ? `app:${rule.app_ids[0]}` : 'global',
      }
      const at = machine.rules.findIndex((entry) => entry.id === stored.id)
      if (at >= 0) machine.rules[at] = stored
      else machine.rules.push(stored)
      return stored.id
    }),
  FinderMenu: () =>
    answer('FinderMenu', () => [] as Record<string, unknown>[]),
  SetMenuItemEnabled: (id, enabled) =>
    answer('SetMenuItemEnabled', () => [] as Record<string, unknown>[]),
  DeleteUserRule: (id) =>
    answer('DeleteUserRule', () => {
      machine.rules = machine.rules.filter((entry) => entry.id !== id)
      return [...machine.rules]
    }),
}

/**
 * The switcher's three calls, kept beside the control surface rather than in it
 * for the reason every other extra call is kept out of ServiceApi: they are a
 * narrow, page-owned seam, and widening the shared interface for them would put
 * the switcher's vocabulary in every control's reach.
 */
const switcherSurface = {
  Windows: () => answer('Windows', () => [...machine.windows]),

  // The long poll. A SETTINGS window never summons a switcher, so the only
  // answer this machine can give is the idle one — a spent budget, which the
  // daemon spells triggered=false rather than an error. The overlay, which is
  // the surface that does wait for a chord, scripts its own answers; see
  // controls/switcher.test.tsx. The budget is the daemon's to clamp (the shell
  // asks for its default with zero), so it is taken and not interpreted.
  SwitcherWait: (timeoutMs: number) => answer('SwitcherWait', () => ({ triggered: false })),

  // Raising a window is the same write a release of the chord performs. It
  // moves the row to the front of the MRU, and the highlight FOLLOWS it — the
  // reference's rule that acting on a tile is a commitment, so the pick does not
  // slide off the window the person aimed at
  // (SelectionResolverSpecs.md:44-50, priority 5).
  SwitcherFocus: (windowID: string) =>
    answer('SwitcherFocus', () => {
      const raised = machine.windows.find((row) => row.window_id === windowID)
      if (!raised) throw new Error(`no window named ${windowID}`)
      const rest = machine.windows.filter((row) => row.window_id !== windowID)
      machine.windows = [{ ...raised, selected: true }, ...rest.map((row) => ({ ...row, selected: false }))].map(
        (row, index) => ({ ...row, index }),
      )
      return { window_id: windowID, focused: true }
    }),
}

/**
 * The three calls App.tsx makes itself, which ServiceApi deliberately does not
 * model (see the interface's own comment). The shell needs them to discover
 * pages and to show the two log sources, so the stub carries them alongside the
 * control surface rather than the control surface being widened to hold them.
 */
const shellSurface = {
  Pages: () => answer('Pages', () => servedInOrder(productPages())),
  GetEventLogs: () =>
    answer('GetEventLogs', () =>
      machine.decisions.map((row) => `${row.winner} ${row.action} keys=${row.event.keys} losers=${row.losers.join(',')}`),
    ),
  UILogs: () => answer('UILogs', () => (machine.profile === '' ? [] : ['Service.Resume: interception installed'])),
}

/**
 * The whole bound surface, over one mutable machine. Stable identity: App.tsx
 * imports this object once, so every call in a test reads the state as it
 * stands at that moment rather than a snapshot taken when the module loaded.
 *
 * OpenSystemSettings used to live in a second surface of its own, declared
 * outside ServiceApi on the grounds that the Wails Service did not model it.
 * It does now, so it sits in controlSurface where the annotation checks it.
 */
export const stub = Object.assign(controlSurface, shellSurface, switcherSurface)
