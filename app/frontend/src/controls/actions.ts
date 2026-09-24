// The action registry: which bound method a page's declared action runs
// (bead cross-os-itq).
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

export type ActionCommand = (service: ServiceApi) => Promise<unknown>

export const ACTION_COMMANDS: Record<string, ActionCommand> = {
  'safety.panicStop': (service) => service.PanicStop(),
  'safety.resume': (service) => service.Resume(),
  'safety.reset': (service) => service.ResetEverything(),
}

/**
 * commandFor looks an action up through the prototype-safe path. A plain object
 * lookup would answer "toString" or "constructor" with a function inherited
 * from Object.prototype, and renderControl would then try to draw it as a
 * control.
 */
export function commandFor(action: string): ActionCommand | undefined {
  if (!Object.prototype.hasOwnProperty.call(ACTION_COMMANDS, action)) return undefined
  return ACTION_COMMANDS[action]
}
