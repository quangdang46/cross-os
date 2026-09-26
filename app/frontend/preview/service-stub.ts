// Preview-only replacement for src/lib/service: the same bound surface, served
// by the test fixture's stateful machine, so the shell renders real pages
// without the Wails bridge.
import { stub } from '../src/test/fixtures'

export const Service = stub as never
export const service = stub as never

/**
 * The overlay answers SwitcherWait with `triggered: false` — a spent budget,
 * which is the honest answer for a machine nobody pressed a chord on, and
 * leaves the switcher on "Waiting for the switcher chord." forever. Nothing can
 * synthesise the chord; that absence is the point, and the switcher's own tests
 * say so. So `?summon=1` answers every wait with a trigger, which is what puts
 * the panel on screen.
 *
 * EVERY wait, not the first: StrictMode mounts, unmounts and remounts, so the
 * overlay's effect and its first poll both run twice, and the generation check
 * discards the answer from the run that was torn down. A counter would spend
 * its one trigger on that discarded run and the panel would never appear — the
 * same reasoning the file's own comment gives for the generation check.
 *
 * HERE rather than in preview/switcher.tsx because of load order: a module
 * body runs before the importing module's, and switcher.tsx renders on import.
 * A patch written after its import statements has already missed the first
 * poll.
 */
if (new URLSearchParams(globalThis.location?.search ?? '').get('summon')) {
  const seam = stub as unknown as { SwitcherWait: (ms: number) => Promise<{ triggered: boolean }> }
  seam.SwitcherWait = () => Promise.resolve({ triggered: true })
}
