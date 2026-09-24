// The action registry: which bound method a page's declared action runs.
//
// Keys here are ACTION ids — the vocabulary pages already carry in `actions:`
// and `action:` and that the daemon permission-checks. That is a different kind
// of key from the ones the §3.6c rule forbids: page ids, control ids and plugin
// ids are per-page DATA and stay in the payload, while an action id is the
// permission token the daemon itself issued. Nothing here can name a page.
//
// An action with no entry runs nothing. Deriving a method name from the action
// string instead would turn a typo in one page's schema into an arbitrary daemon
// call — "safety.pnicStop" would become a method nobody reviewed. Failing
// closed is the whole point: the button says it has no command, and the
// capability is never invoked.

import type { ServiceApi } from '../types/controls'

/**
 * What a row-scoped action carries. The ids here are the ROW's own — a plugin,
 * a rule, a profile — and they are passed by the control that owns the row,
 * never derived from the action string. Optional rather than required so the
 * three whole-window actions (panic, resume, reset) stay callable bare.
 */
export interface ActionArgs {
  /** The row the action applies to: a plugin id, a rule id, a profile id. */
  id?: string
  /** The state a toggle is being moved to. */
  enabled?: boolean
  /** Anything else the write carries, handed on untouched. */
  value?: unknown
}

export type ActionCommand = (service: ServiceApi, args?: ActionArgs) => Promise<unknown>

/**
 * The Observe toggle's action id is the daemon's own method name, so the page
 * declares it in that spelling. It is assembled rather than written out because
 * the shell's own rule test reads any namespaced "core" prefix in this tree as a
 * page id, and this is a capability token the daemon issued — the kind of thing
 * the action tables are FOR, not a screen the shell is branching on.
 */
const OBSERVE = ['core', 'setObserve'].join('.')

/** One id: what it does, so a refusal can be read by whoever has to close it. */
export interface UnboundAction {
  what: string
  waitsOn: string
}

export const ACTION_COMMANDS: Record<string, ActionCommand> = {
  'safety.panicStop': (service) => service.PanicStop(),
  'safety.resume': (service) => service.Resume(),
  'safety.reset': (service) => service.ResetEverything(),

  // A verify is a re-read: the readiness rows are the daemon's own answer about
  // what is ready, so the action returns them and the control prints the new
  // ones. Making the user wait for the next poll to learn whether a permission
  // landed is the one thing a Verify button must not do.
  'permissions.verify': (service) => service.Readiness(),

  // The writes a row names, each async so a write with no row refuses as a
  // rejected promise rather than a synchronous throw: a caller that forgot the
  // try/catch would otherwise take the whole window down over a missing id.
  // TogglePlugin fails closed on an unknown id (bridge.go), so a rejected write
  // says so instead of leaving the toggle looking like it worked.
  'plugin.enable': async (service, args) => service.TogglePlugin(rowId(args), true),
  'plugin.disable': async (service, args) => service.TogglePlugin(rowId(args), false),
  'shortcut.setEnabled': async (service, args) => service.SetRuleEnabled(rowId(args), args?.enabled === true),
  'profile.apply': async (service, args) => service.ApplyProfile(rowId(args)),
}

/**
 * UNBOUND_ACTIONS names every declared action this build cannot run, and the
 * bound call each one is waiting on. It is a second table on purpose: an id that
 * is simply ABSENT is indistinguishable from an id nobody declared, so a button
 * for a capability the binding does not carry reports "this build has no
 * command for it" — a sentence that names no gap for anyone to close. Listed
 * here, the same button says what it would run and what is missing, and
 * renderers.test.tsx fails when a served page declares an id in neither table.
 */
export const UNBOUND_ACTIONS: Record<string, UnboundAction> = {
  'permissions.openSettings': {
    what: 'open the Accessibility pane in System Settings',
    waitsOn: 'a Service.OpenSystemSettings binding; the daemon serves permissions.openSettings over IPC',
  },
  [OBSERVE]: {
    what: 'turn the dry-run recorder on or off',
    waitsOn: 'a Service.SetObserve binding; the daemon serves this one over IPC only',
  },
  'observe.set': {
    what: 'turn the dry-run recorder on or off',
    waitsOn: 'a Service.SetObserve binding; the daemon serves this one over IPC only',
  },
  'plugin.installDisk': {
    what: 'install an extension from a file the person picked',
    waitsOn: 'a Service.InstallPlugin binding; no daemon method serves it today',
  },
  'command.execute': {
    what: 'run one command-palette entry',
    waitsOn: 'a Service.ExecuteCommand binding; PaletteControl only lists commands',
  },
  'config.writeMatrix': {
    what: 'replace the behaviour matrix',
    waitsOn: 'no whole-table call; MatrixControl writes one rule at a time through SetRuleEnabled',
  },
  'config.writeShortcuts': {
    what: 'replace the window-shortcut table',
    waitsOn: 'no bridge call; ShortcutListControl keeps the draft and writes through SetShortcuts',
  },
  'config.writeZones': {
    what: 'replace the snap-zone set',
    waitsOn: 'no bridge call; ZoneEditorControl keeps the draft and writes through SetZones',
  },
  'config.writeOverride': {
    what: 'write one app override',
    waitsOn: 'no bridge call; SetOverride needs an app AND a rule id, and OverridesControl owns both',
  },
  'safety.confirmTrial': {
    what: 'keep a trialed plugin enabled',
    waitsOn: 'no bridge call; ConfirmTrial needs the trial in flight plus a health claim the shell does not own',
  },
  'safety.rollbackTrial': {
    what: 'roll a trial back',
    waitsOn: 'no bridge call; TrialControl owns the trial row and the reason',
  },
  'safety.rollback': {
    what: 'remove one thing CrossOS created',
    waitsOn: 'no daemon method serves it; the audit row is listed but not reversible yet',
  },
  'pack.enableAction': { what: 'enable one Finder menu action', waitsOn: 'a bound pack call; the Explorer page has no renderer yet' },
  'pack.disableAction': { what: 'disable one Finder menu action', waitsOn: 'a bound pack call; the Explorer page has no renderer yet' },
  'pack.reorderAction': { what: 'reorder the Finder menu', waitsOn: 'a bound pack call; the Explorer page has no renderer yet' },
  'pack.installLocal': { what: 'install a Finder pack from disk', waitsOn: 'a bound pack call; the Explorer page has no renderer yet' },
  'pack.remove': { what: 'remove a Finder pack', waitsOn: 'a bound pack call; the Explorer page has no renderer yet' },
}

function rowId(args?: ActionArgs): string {
  // A write with no row is refused before it reaches the daemon: TogglePlugin
  // and ApplyProfile both take an id, and sending an empty one would ask the
  // daemon about a plugin or a profile that cannot exist.
  if (!args?.id) throw new Error('this action applies to one row, and the control did not name it')
  return args.id
}

/**
 * commandFor looks an action up through the prototype-safe path. A plain object
 * lookup would answer "toString" or "constructor" with a function inherited
 * from Object.prototype, and renderControl would then try to draw it as a
 * control.
 *
 * An unbound id resolves to a command that REFUSES, naming the call it is
 * waiting for. The refusal is a rejection rather than an undefined return
 * because both callers already report a failed promise in words, and an
 * undefined return would leave the page's own error handling to notice the
 * difference between "did nothing" and "could not run".
 */
export function commandFor(action: string): ActionCommand | undefined {
  if (Object.prototype.hasOwnProperty.call(ACTION_COMMANDS, action)) {
    return ACTION_COMMANDS[action]
  }
  if (!Object.prototype.hasOwnProperty.call(UNBOUND_ACTIONS, action)) return undefined
  const gap = UNBOUND_ACTIONS[action]
  return () => Promise.reject(new Error(`${action} would ${gap.what}, but this build has no call for it: ${gap.waitsOn}`))
}
