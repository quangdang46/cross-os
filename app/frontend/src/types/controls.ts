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
//     So the eight tagged rows (MatrixRow … ReadinessRow) are snake_case here,
//     exactly as the daemon serves them, while Status/PluginState/Page are
//     PascalCase because those Go structs carry no json tags. This is not a
//     preference: with the PascalCase spelling these interfaces are satisfied
//     only by a hand-written shim, and every one of those properties is
//     `undefined` at runtime against the bindings Wails actually generates.
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

/** One command-palette entry (core.commands). Commands arrive as data. */
export interface CommandRow {
  id: string
  title: string
  plugin: string
}

/** One plugin's declarative config_schema (core.pluginSchemas). */
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

/** One readiness checklist item (core.readiness). Detail says what to do. */
export interface ReadinessRow {
  id: string
  label: string
  ready: boolean
  detail: string
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
}
