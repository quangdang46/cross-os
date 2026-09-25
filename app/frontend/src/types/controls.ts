// Control + payload types for the settings shell (bead cross-os-itq).
//
// Three decisions are baked into this file and the other lanes should not undo
// them quietly:
//
//  1. The row types are declared HERE, not imported from ../bindings. The
//     bindings are a build output (gitignored) and do not exist until someone
//     runs `wails3 generate bindings ./...` in app/. Declaring the shapes
//     against an interface the shell owns is what keeps a page readable
//     without reading generated JavaScript — and lib/service.ts then checks
//     the real generated Service against ServiceApi, so the copy cannot drift.
//     The cost, stated plainly: the frontend does not typecheck without the
//     generated bindings. That is the same rule the vite build already obeys,
//     since the @wailsio/runtime plugin refuses to build without them too.
//
//  2. THE PROPERTY SPELLINGS BELOW MUST MATCH THE GENERATOR'S OUTPUT, and the
//     generator follows encoding/json: a Go field with a `json:"name"` tag is
//     emitted as `name`, and a field with no tag is emitted under its Go name.
//     So the sixteen tagged rows (MatrixRow … UserRuleRow) are snake_case
//     here, exactly as the daemon serves them, while Status/PluginState/Page
//     and AppRow are PascalCase because those Go structs carry no json tags.
//     This is not a preference: with the PascalCase spelling these interfaces
//     are satisfied only by a hand-written shim, and every one of those
//     properties is `undefined` at runtime against the bindings Wails actually
//     generates.
//
//  3. No list of page ids, control ids or plugin ids appears anywhere below.
//     Those are runtime data (§3.6c: a page is a Go change with zero UI
//     change). What belongs in a type file is the shape of that data.

/**
 * One control as a page's schema declares it. The optional fields are the ones
 * the Core pages in app/backend/pages.go actually use; the index signature is
 * deliberate — a page may add a field to its schema without a shell change, and
 * reading one that this build does not model must degrade to `undefined` at
 * runtime rather than to a compile error in the renderer.
 */
export interface Control {
  kind: string
  id: string
  label?: string
  text?: string
  note?: string
  action?: string
  source?: string
  /** A setup flow's steps: a bare title, or a declared step (see WizardStep). */
  steps?: (string | WizardStep)[]
  items?: string[]
  /** Action ids this control may invoke, permission-checked by the daemon. */
  actions?: string[]
  /** Copy a destructive button shows before it runs. */
  confirm?: string
  format?: string
  [extra: string]: unknown
}

/** One behavior-matrix rule (config.getMatrix). */
export interface MatrixRow {
  rule_id: string
  plugin: string
  action: string
  keys: string
  contexts: string[]
  enabled: boolean
}

/** One per-app override (config.getOverrides); shadows a matrix rule in one app. */
export interface OverrideRow {
  app: string
  rule_id: string
  action: string
  keys: string
  enabled: boolean
}

/**
 * One snap-zone rectangle (config.getZones). Geometry is float because a
 * monitor's usable area rarely divides evenly; the editor rounds for display
 * and sends the exact number the user typed.
 */
export interface ZoneRow {
  id: string
  name: string
  x: number
  y: number
  w: number
  h: number
}

/** One command-palette entry. Commands arrive as data, not as code paths. */
export interface CommandRow {
  id: string
  title: string
  plugin: string
}

/** One plugin's declarative config_schema, already decoded by the daemon. */
export interface SchemaRow {
  plugin: string
  title: string
  schema: Record<string, unknown>
}

/** One thing CrossOS created on this machine (safety.ownershipAudit). */
export interface AuditRow {
  resource: string
  id: string
  owner: string
  created_at: string
}

/**
 * The trial in flight (safety.trialState). Both durations are milliseconds so
 * the countdown is arithmetic rather than string parsing. State "none" with an
 * empty plugin is a real answer — "no trial is running" — and must never render
 * as a broken or zero countdown.
 */
export interface TrialState {
  plugin: string
  state: string
  remaining_ms: number
  timeout_ms: number
}

/** One readiness checklist item. Detail says what to do, not merely that not. */
export interface ReadinessRow {
  id: string
  label: string
  ready: boolean
  detail: string
}

/**
 * One wizard step with the DAEMON's verdict on it.
 *
 * Detail is why the step is not done — the reason to act, not merely the fact
 * of waiting — and it is absent (not empty) on a done step, so a page leaves
 * the slot out rather than print a blank line where an explanation would have
 * been. The shell renders this and never writes to it.
 */
export interface OnboardingStep {
  id: string
  label: string
  done: boolean
  detail?: string
}

/**
 * The whole first-run wizard in one answer (the daemon's onboardingState).
 *
 * Readiness RIDES ALONG rather than being looked up again: the step verdicts on
 * this row were computed FROM those rows (core/cmd/crossos/pagedata.go,
 * handleOnboardingState), so a wizard that re-read the readiness verb to explain a
 * step would be showing a second opinion that can disagree with the verdict it
 * is explaining. That is why a control reads the two from here and never joins
 * them itself.
 *
 * current_step is the daemon's cursor — the first step it has not derived as
 * done, or "done" when it has nothing left. A page must not keep a counter of
 * its own beside this one: two cursors on one flow is the disagreement this
 * row exists to make impossible.
 */
/**
 * What the recorder is ACTUALLY doing, read back from it. The snake_case
 * spelling is the daemon's own (`app/backend/uisources.go`), like every other
 * wire row here.
 *
 * Mode is a string rather than a number because a person has to read what each
 * one keeps, and "2" says nothing. The daemon owns the vocabulary and the
 * labels are spelled in record.Mode's String.
 */
export interface ObserveStateRow {
  observe: boolean
  mode: string
}

/** What each privacy mode keeps, in the words the recorder's own comment uses. */
export const OBSERVE_MODES: Record<string, string> = {
  'metadata-only': 'Metadata only — the decision, the app, the rule, the intent. Typed text, file contents and secrets are replaced.',
  debug: 'Debug — metadata plus the structural parameters of each action.',
  'full-trace': 'Full trace — everything, including the values above. Never on by default.',
  off: 'Off — nothing is recorded.',
}

/**
 * One rule claiming a contested chord, in rank order (winner first).
 *
 * The action is the human name the same rule table renders elsewhere, so a
 * row here and a row in the matrix call the same rule the same thing. The
 * trace path this replaced carried the rule id alone, which is how
 * "won by windows-keyboard.ctrl-c-copy" ended up being the whole sentence.
 */
export interface ConflictClaim {
  rule_id: string
  plugin: string
  action: string
}

/**
 * One chord two still-firing rules both claim (the conflicts source).
 *
 * Winner and Losers are the decision path's own verdict rather than
 * something the shell worked out, so the editor cannot offer to keep a rule
 * that actually wins — and the source answers for a chord nobody has
 * pressed, which is the collision worth warning about.
 */
export interface ConflictRow {
  keys: string
  winner: string
  losers: string[]
  rules: ConflictClaim[]
}

export interface OnboardingRow {
  completed: boolean
  current_step: string
  steps: OnboardingStep[]
  readiness: ReadinessRow[]
  ready: number
  total: number
}

/**
 * One step as a PAGE declares it, in the object form.
 *
 * A bare string in `steps` is a title and nothing else, which is every wizard
 * the daemon has served so far. The object form adds the two things a step
 * needs beyond its title: the `id` the daemon's verdict is keyed by (so a step's
 * done mark is a JOIN, never a position guess), and the control KIND whose body
 * the step draws inline (so a flow that puts the profile cards in the middle of
 * itself declares that, rather than the shell special-casing a page).
 *
 * Both fields are optional so declaring one costs a page nothing, and a step
 * that uses neither behaves exactly like a bare string.
 */
export interface WizardStep {
  /** The daemon's step id this title is the same step as. */
  id?: string
  label: string
  /** A control kind drawn as this step's body (e.g. the profile cards). */
  body?: string
}

/**
 * One capability a profile bundles, with the rollup the profile card shows.
 *
 * Enabled/Total count only the behavior-matrix rules, so a capability that
 * ships no rules (a window zone, say) reads 0 of 0 rather than "1 of 21
 * shortcuts on" for a list it does not have. RuleIDs keeps both vocabularies
 * the daemon declares, which is why it is a list of plain strings here.
 */
export interface ProfileCapabilityRow {
  id: string
  label: string
  plugin: string
  available: boolean
  reason?: string
  rule_ids: string[]
  enabled: number
  total: number
  live: boolean
  /**
   * The PREVIEW, per capability: how many of this capability's rules applying
   * would switch on that are off right now, and whether it would switch the
   * extension on.
   *
   * Optional because a daemon that predates the preview simply does not send
   * them, and a control that read `undefined` as NaN would render "NaN
   * shortcuts to turn on" — so the shell treats absent as zero and says it
   * has nothing to preview. A missing field is a fact about the daemon, not
   * an error to paper over.
   */
  will_enable?: number
  will_enable_plugin?: boolean
}

/**
 * One profile card. The profile is the product ("a Windows-like setup in one
 * click") and its capabilities are the rollup that answers whether the click
 * landed, so the card arrives with both rather than making a control fetch a
 * second source to find out.
 */
export interface ProfileRow {
  id: string
  label: string
  description: string
  active: boolean
  capabilities: ProfileCapabilityRow[]
  /**
   * The card's PREVIEW — what applying this profile will change, computed by
   * the daemon and read BEFORE the click rather than reported after it.
   *
   * `will_enable` counts rules that are off and would be switched on, and
   * `already_on` the ones that are on and would be left alone.
   * `will_enable_plugins` counts EXTENSIONS the apply would switch on, and it
   * is the field that moves on a fresh install: a rule is on unless somebody
   * explicitly turned it off, so applying a profile on an untouched machine
   * switches on no rules at all and the whole of the change is the extension.
   * A preview that showed only the rules would read "0 shortcuts to turn on"
   * beside a keyboard that does not work, which is true and useless.
   *
   * All optional for the same reason as the per-capability halves: absent
   * means this daemon does not preview, and the card says so rather than
   * inventing a number.
   */
  will_enable?: number
  already_on?: number
  will_disable?: number
  will_enable_plugins?: number
  /**
   * Whether a recorded undo point exists, and what to say when none does.
   *
   * These two arrive TOGETHER on purpose. A control handed only the boolean
   * greys the Revert button out silently, and a silent grey-out is
   * indistinguishable from a button that is broken — so the daemon sends the
   * sentence and the control renders it. `revertible` absent means an older
   * daemon that has no revert at all, which is its own sentence.
   */
  revertible?: boolean
  revert_reason?: string
}

/** The physical input one decision was made from. */
export interface TraceEvent {
  keys: string
  source: string
  device?: string
  key_code: number
}

/** The cached decision context the router logged. Never a live query. */
export interface TraceContext {
  app_id: string
  app_mode: string
  window_id?: string
  win_class?: string
}

/** One pipeline step, in the order it ran. */
export interface StageRow {
  stage: string
  detail: string
}

/**
 * One recorded decision as fields. The flattened event log this replaces was
 * a sentence per decision; stages, the focused app and the losing rules are
 * all still in the record, so a page reads them instead of parsing prose.
 */
export interface TraceRow {
  at: string
  decision: string
  event: TraceEvent
  context: TraceContext
  winner: string
  losers: string[]
  intent: string
  action: string
  stages: StageRow[]
  params: string
}

/**
 * One registered plugin's manifest facts. Name and Version are the manifest's
 * own and are empty when a plugin ships none — the row then carries the
 * reason, which is why neither field is defaulted to the id here.
 */
export interface PluginMetaRow {
  id: string
  name: string
  version: string
  permissions: string[]
  loaded: boolean
  reason?: string
}

/**
 * One application a rule may be scoped to — the row behind a rule builder's
 * "when the front app is X" picker.
 *
 * PascalCase because this row carries no json tags: it is the adapter's own
 * application-identity record, served verbatim so the picker's row and the
 * matcher's row cannot drift into two shapes for one application.
 */
export interface AppRow {
  BundleID: string
  Executable: string
  PID: number
  DisplayName: string
  AppMode: string
  Category: string
}

/**
 * One person-authored rule.
 *
 * The first block is the stored rule's own dimensions, under the keys the
 * write path takes them: an edit sends a row back and the editor never
 * composes a payload. Chord, Action, Priority, Specificity and Scope are
 * DERIVED by the daemon for display — sending one back would be a number the
 * person can change without changing the rule — which is why they are grouped
 * here and the comment above the write call repeats the point.
 */
export interface UserRuleRow {
  id: string
  key: string
  modifiers: string[]
  app_modes: string[]
  app_ids: string[]
  device_id: string
  capability: string
  parameters?: Record<string, unknown>
  emit: boolean

  // Derived for display; the daemon recomputes these on every write.
  chord: string
  action: string
  priority: number
  specificity: number
  scope: string
}

/**
 * A row of the shortcut table (config.getShortcuts). This one is NOT a typed
 * struct on either side: the daemon builds it as a Go map, so the key names are
 * whatever the daemon wrote. Indexing it instead of declaring fields is the
 * honest reading of "[]map[string]any" — the shell must not invent a field the
 * daemon never promised. Read it through the narrowers in lib/wire.ts.
 */
export interface ShortcutRow {
  [field: string]: unknown
}

/**
 * One chord a person pressed, as the recorder caught it.
 *
 * Modifiers are the daemon's OWN spellings (Ctrl, Shift, Alt, Win — the rows
 * userrules.Modifiers serves and ModifierMask reads back), not the DOM's
 * `ctrlKey`/`metaKey`: a rule stores these strings, so a capture that spelled
 * them any other way would be a chord the router cannot match.
 */
export interface ChordCapture {
  key: string
  modifiers: string[]
}

/**
 * One claim on a chord, as the decision pipeline reports it: the rule that won
 * and the rules that lost to it. Both are the router's own verdict (the winner
 * is tr.Winner, the losers are tr.Losers, both produced by rule.Resolve), so a
 * resolver built on these cannot offer a rule that does not in fact lose.
 */
export interface ChordContest {
  /** The chord every claim here is for, in the daemon's rendering. */
  keys: string
  winner: string
  losers: string[]
}

/** One installed plugin, as the dashboard/status payload serves it. */
export interface PluginState {
  ID: string
  Enabled: boolean
  Healthy: string
  /** Where it came from. "builtin" is compiled into the daemon and cannot crash. */
  Origin: string
}

/** The daemon's status payload. Version comes from the daemon, never a literal. */
export interface Status {
  Running: boolean
  SafeMode: boolean
  Killed: boolean
  Plugins: PluginState[] | null
  Interception: boolean
  TapError: string
  Version: string
}

/**
 * One row of the Explorer's file-type catalog: the presets the New > submenu
 * offers, which a person edits rather than types.
 *
 * The Go row EMBEDS the catalog's own struct (app/backend/uisources.go), and
 * the generator flattens an embedded struct into these flat properties —
 * which is why there is no nested `fileType` object here. That is the wire, and
 * it is the reason the row is declared against the daemon's fields rather than
 * as a wrapper: a copy that nested them would decode a row of `undefined` in
 * the settings pane.
 *
 * The catalog allows TWO presets to share one extension ("data.json" and
 * "report.json" are two presets, not a duplicate), and a blank baseName is a
 * dotfile — so a row's identity is (baseName, ext) together, and never ext
 * alone.
 *
 * MenuTitle is the label Finder will show, derived by the daemon on every read
 * rather than stored. It is included so a page can render the exact string the
 * menu will, and it is never sent back: the write takes the editable fields
 * only, because a stored copy of a derivation is a second thing to keep true.
 */
export interface FileTypeRow {
  ext: string
  baseName: string
  displayName: string
  template: string
  enabled: boolean
  builtIn: boolean
  menuTitle: string
}

/**
 * The shell's typed view of the Wails-bound Service — the ONE seam between the
 * frontend and the daemon.
 *
 * It covers the calls a control makes. Page discovery (Pages) and the UI log
 * (UILogs) are App.tsx's, and GetEventLogs is not here because the trace
 * arrives through ControlContext.logs, already fetched by whoever owns the
 * refresh cadence. Keeping the surface narrow is what stops a control from
 * quietly becoming a second App.
 *
 * The real generated Service satisfies this structurally: every signature below
 * matches a bound method in app/backend/service.go, so `typeof Service` is
 * assignable to ServiceApi and a Go method whose arity or return changes stops
 * compiling here rather than reaching the window.
 */
export interface ServiceApi {
  GetStatus(): Promise<Status | null>
  TogglePlugin(id: string, enabled: boolean): Promise<void>

  PanicStop(): Promise<Record<string, unknown> | null>
  Resume(): Promise<Record<string, unknown> | null>
  ResetEverything(): Promise<string[] | null>

  BeginTrial(pluginID: string): Promise<string>
  ConfirmTrial(pluginID: string, confirmed: boolean, healthy: boolean): Promise<string>
  RollbackTrial(pluginID: string, reason: string): Promise<string>

  SetRuleEnabled(ruleID: string, enabled: boolean): Promise<boolean>
  Shortcuts(): Promise<({ [key: string]: unknown } | null)[] | null>
  SetShortcuts(shortcuts: ShortcutRow[]): Promise<number | null>
  FinderMenu(): Promise<({ [key: string]: unknown } | null)[] | null>
  SetMenuItemEnabled(id: string, enabled: boolean): Promise<({ [key: string]: unknown } | null)[] | null>
  /**
   * The Explorer's file-type catalog, in the order the editor left it.
   *
   * Both writes answer with the WHOLE catalog rather than with the row they
   * touched, so the editor redraws from the daemon's own account of what is
   * stored instead of assuming its own optimistic edit landed. A write that
   * returned only the edited row would let a rejected edit sit on screen
   * looking saved until the next poll.
   */
  FileTypes(): Promise<FileTypeRow[] | null>
  SetFileType(row: FileTypeRow): Promise<FileTypeRow[] | null>
  /**
   * Replaces the catalog's order. The list is the COMPLETE permutation the
   * daemon demands — no id twice, none missing, none invented — because a
   * partial list is a drag the daemon could not see the end of, and the rows
   * it did not name would keep positions the person is no longer looking at.
   */
  ReorderFileTypes(ids: string[]): Promise<FileTypeRow[] | null>

  GetMatrix(): Promise<MatrixRow[] | null>
  GetOverrides(): Promise<OverrideRow[] | null>
  SetOverride(app: string, ruleID: string, enabled: boolean): Promise<OverrideRow>
  GetZones(): Promise<ZoneRow[] | null>
  SetZones(zones: ZoneRow[]): Promise<number | null>
  Commands(): Promise<CommandRow[] | null>
  PluginSchemas(): Promise<SchemaRow[] | null>
  OwnershipAudit(): Promise<AuditRow[] | null>
  TrialState(): Promise<TrialState>
  Readiness(): Promise<ReadinessRow[] | null>

  // The first-run wizard's own source and its one terminal write. Both are
  // declared here rather than narrowed at the call site because the Wails
  // Service DOES carry them (app/backend/service.go) — ServiceApi was simply
  // behind, and lib/service.ts is what makes that a compile error rather than an
  // undefined at runtime. The readiness rows ride along on the row, so a
  // control never has to join two sources the daemon already joined.
  OnboardingState(): Promise<OnboardingRow>
  CompleteOnboarding(): Promise<void>
  /**
   * Opens the System Settings pane the Accessibility grant is made in
   * (permissions.openSettings). The grant is made by hand, in another
   * application, so this is the first-run flow's only door onto it.
   *
   * It opens a window and decides nothing: the step's verdict stays the
   * daemon's re-derived readiness, and a resolved promise is never read as a
   * granted permission. Off darwin the daemon refuses, and that refusal
   * arrives here as a rejection carrying its own message.
   */
  OpenSystemSettings(): Promise<void>
  /**
   * Turns the recorder's dry-run on or off. The argument is a plain bool
   * because the daemon treats a payload without the flag as a bad request
   * rather than as a state — a toggle that can be left in no state is not a
   * toggle.
   */
  SetObserve(on: boolean): Promise<void>
  /**
   * Reads the recorder's ACTUAL position: whether it is dry-running, and the
   * privacy mode it is keeping. Read rather than echoed from the last write,
   * because a page that learns the state only from its own write can only
   * ever label the belief.
   */
  ObserveState(): Promise<ObserveStateRow | null>
  /**
   * Every chord two still-firing rules both claim (the conflicts source), with the
   * decision path's own winner and losers. Read rather than derived: it
   * answers for a chord nobody has pressed yet, and the ranking it reports is
   * the one the router will use.
   */
  Conflicts(): Promise<ConflictRow[] | null>

  // Wave 3: the profile cards, the decision trace, the manifest facts, the
  // app picker, and the person-authored rule table. Same rule as the ten
  // above — these are sources a page may declare, never per-page endpoints.
  Profiles(): Promise<ProfileRow[] | null>
  /**
   * Applies a profile, optionally narrowed to a SUBSET of its capabilities.
   *
   * The subset travels with the apply rather than as a second write, because
   * the two together are one gesture: a switch that had its own verb, and an
   * apply that had its own, would let a person flip a switch and then apply
   * the whole bundle with the second click and never know the first was
   * ignored. Omitting it means every available capability, which is what the
   * pre-switch card did and what a caller with no UI sends.
   */
  ApplyProfile(profileID: string, capabilities?: string[]): Promise<Record<string, unknown> | null>
  /**
   * Undoes the last profile apply: the exact prior verdict of every id the
   * plan touched, the profile string, and the running router's in-memory
   * plugin map.
   *
   * No argument, because there is one active profile and one recorded
   * snapshot and the daemon holds both. The reply is the daemon's own account
   * of what it restored rather than a bare "done", for the reason the trace's
   * erase returns the rows that are LEFT: a page that redraws from its own
   * click is a page showing the belief instead of the state.
   *
   * A rejection is a real refusal and carries its own words — the daemon has
   * no snapshot to return to for a profile applied before the undo point was
   * recorded, and saying so is the only honest answer. A resolved promise is
   * never read as "something was undone".
   */
  ProfileDeactivate(): Promise<Record<string, unknown> | null>
  Traces(): Promise<TraceRow[] | null>
  /**
   * Empties the recorder and answers with the list as it stands afterwards —
   * the erased one, not a count and not the previous read's rows.
   *
   * The reply is the whole point. A verb that answered "done" leaves the page
   * showing the decisions it just erased until the next poll, which is several
   * seconds during which the person who pressed the button is reading "cleared"
   * over rows that are still there. The daemon's own account of what is left is
   * what the page redraws from.
   *
   * A rejection here is a real failure and says so: a clear that could not
   * reach the recorder, answered as a success, is a keystroke log the person
   * believes they destroyed and that is still on disk.
   */
  TracesClear(): Promise<TraceRow[] | null>
  PluginMeta(): Promise<PluginMetaRow[] | null>
  Apps(): Promise<AppRow[] | null>
  UserRules(): Promise<UserRuleRow[] | null>
  /** Creates or updates one rule; the daemon returns the id it derived. */
  SetUserRule(rule: UserRuleRow): Promise<string>
  /** Removes one rule and returns the table as it now stands. */
  DeleteUserRule(id: string): Promise<UserRuleRow[] | null>
}
