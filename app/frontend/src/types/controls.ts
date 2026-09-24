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
  steps?: string[]
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
}

/** The daemon's status payload. Version comes from the daemon, never a literal. */
export interface Status {
  Running: boolean
  SafeMode: boolean
  Killed: boolean
  Plugins: PluginState[]
  Interception: boolean
  TapError: string
  Version: string
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
  Shortcuts(): Promise<ShortcutRow[]>
  SetShortcuts(shortcuts: ShortcutRow[]): Promise<number>

  GetMatrix(): Promise<MatrixRow[]>
  GetOverrides(): Promise<OverrideRow[]>
  SetOverride(app: string, ruleID: string, enabled: boolean): Promise<OverrideRow>
  GetZones(): Promise<ZoneRow[]>
  SetZones(zones: ZoneRow[]): Promise<number>
  Commands(): Promise<CommandRow[]>
  PluginSchemas(): Promise<SchemaRow[]>
  OwnershipAudit(): Promise<AuditRow[]>
  TrialState(): Promise<TrialState>
  Readiness(): Promise<ReadinessRow[]>

  // Wave 3: the profile cards, the decision trace, the manifest facts, the
  // app picker, and the person-authored rule table. Same rule as the ten
  // above — these are sources a page may declare, never per-page endpoints.
  Profiles(): Promise<ProfileRow[]>
  ApplyProfile(profileID: string): Promise<Record<string, unknown> | null>
  Traces(): Promise<TraceRow[]>
  PluginMeta(): Promise<PluginMetaRow[]>
  Apps(): Promise<AppRow[]>
  UserRules(): Promise<UserRuleRow[]>
  /** Creates or updates one rule; the daemon returns the id it derived. */
  SetUserRule(rule: UserRuleRow): Promise<string>
  /** Removes one rule and returns the table as it now stands. */
  DeleteUserRule(id: string): Promise<UserRuleRow[]>
}
