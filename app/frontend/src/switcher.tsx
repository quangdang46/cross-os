// The switcher overlay: the second window this frontend builds.
//
// The settings shell is a window the person opens. This is the window that
// appears when the switcher chord is pressed and disappears from view again
// after it — it is a SEPARATE document (src/switcher.html) rather than a
// component of the shell, because a switcher summoned over a settings window
// that is already up has to be a second window, and a second window is a second
// entry, not a second page.
//
// The loop is the daemon's long poll, not a timer of the shell's own: the wait
// blocks server-side for the chord and answers either with the trigger or with
// triggered=false when the budget ran out. So a spent budget is a value the loop
// reads and re-polls on, never an error and never a reason to give up — which is
// the whole reason the answer is a boolean rather than an exception
// (core/cmd/crossos/switcher.go, handleSwitcherWait).
//
// WHAT THE OVERLAY DRAWS IS THE SAME PANEL THE PAGE DRAWS. There is no overlay
// renderer and no second copy of a tile: the SwitcherPanelControl is mounted
// here with a control declaration this file owns, because the overlay is not a
// page. It cannot ask the host for the page by id — a page id in a conditional
// is the thing §3.6c forbids, and the overlay is exactly the case that would
// tempt it — so it declares the control it needs as DATA and the registry
// resolves the kind, the same way any served page's schema would.
//
// The two action ids are the daemon's own method names, assembled in the
// registry, and both sides of the switcher resolve them through it: the wait
// here, the row write in the panel. Nothing names a bound method directly.

import { StrictMode, useEffect, useRef, useState } from 'react'
import type { ReactElement } from 'react'
import ReactDOM from 'react-dom/client'
import { Window } from '@wailsio/runtime'
import { service } from './lib/service'
import { asBool, asText, failedTo } from './lib/wire'
import { commandFor, SWITCHER_FOCUS, SWITCHER_WAIT } from './controls/actions'
import { SwitcherPanelControl } from './controls/SwitcherPanelControl'
import type { ControlContext } from './controls'
import type { Control } from './types/controls'

/**
 * The control the overlay draws.
 *
 * Data, not a branch: a kind the registry resolves, a row write the page's own
 * schema would have declared, and the wait the loop below runs. The label is in
 * words because a named thing is never identified by its key alone.
 */
const OVERLAY_CONTROL: Control = {
  kind: 'switcherPanel',
  id: 'overlay',
  label: 'Switcher',
  rowAction: SWITCHER_FOCUS,
  actions: [SWITCHER_WAIT],
  note: 'One tile per window. Clicking a tile raises it, which is what letting go of the chord does.',
}

/**
 * The gap between two waits that both answered "nothing happened".
 *
 * The daemon's own budget is the real wait — it blocks for the chord — so this
 * only bounds a daemon that answers instantly, which keeps a broken or mocked
 * one from spinning this window's event loop. It is deliberately not the poll
 * interval: the cadence belongs to the daemon, and a shell-side timer here would
 * be a second answer to a question the daemon already answers.
 */
const IDLE_GAP_MS = 50

/** The wait's answer, read defensively: a missing field is "nothing happened",
 *  which is the answer an idle machine gives anyway. */
interface Trigger {
  triggered: boolean
  action: string
  windowID: string
}

function readTrigger(value: unknown): Trigger {
  const record = value as { triggered?: unknown; action?: unknown; window_id?: unknown } | null
  return {
    triggered: asBool(record?.triggered) ?? false,
    action: asText(record?.action),
    windowID: asText(record?.window_id),
  }
}

export function SwitcherOverlay(): ReactElement {
  const [summoned, setSummoned] = useState(false)
  const [fault, setFault] = useState('')
  const [note, setNote] = useState('')
  const [refreshToken, setRefreshToken] = useState(0)
  /** A wait still in flight when the effect was torn down. Each run takes a
   *  number, so an answer that lands after the run it belongs to is dropped
   *  rather than shown — the same generation check lib/useResource makes. */
  const generation = useRef(0)

  useEffect(() => {
    const mine = ++generation.current
    let timer: ReturnType<typeof setTimeout> | undefined
    const resolved = commandFor(SWITCHER_WAIT)

    if (!resolved) {
      // The registry is the one place a bound call is named, so an id that
      // resolves to nothing is a defect somebody has to close rather than a
      // switcher that silently never opens.
      setFault(
        `This build has no command for ${SWITCHER_WAIT}, so the switcher can never be waited for.`,
      )
      return () => {
        generation.current += 1
      }
    }
    const wait = resolved

    async function poll(): Promise<void> {
      let answer: Trigger
      try {
        answer = readTrigger(await wait(service))
      } catch (reason) {
        if (generation.current !== mine) return
        setFault(failedTo('Waiting for the switcher', reason))
        timer = setTimeout(() => void poll(), IDLE_GAP_MS)
        return
      }
      if (generation.current !== mine) return
      setFault('')
      if (answer.triggered) {
        setSummoned(true)
        // The tiles are the daemon's answer AT the summon, so the panel is
        // re-read rather than left showing whatever was on screen last time the
        // chord was pressed.
        setRefreshToken((token) => token + 1)
        try {
          // Every trigger shows the window. The daemon TAKES a trigger when it
          // is handed over, so one chord is one show; and a window that is
          // already up treats a second show as the no-op it is, which is what
          // makes a chord that arrives while the switcher is still up work.
          await Window.Show()
        } catch (reason) {
          setFault(failedTo('Showing the switcher', reason))
        }
        // And the loop does NOT end here: the next chord is the next trigger,
        // and an overlay that stopped waiting after the first one would open
        // once per window rather than once per chord.
        timer = setTimeout(() => void poll(), IDLE_GAP_MS)
        return
      }
      // A spent budget is the daemon's "your turn is over", not an incident:
      // the answer that matters is the action it was waiting for, and an idle
      // machine never sends one.
      timer = setTimeout(() => void poll(), IDLE_GAP_MS)
    }

    void poll()
    return () => {
      generation.current += 1
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [])

  const ctx: ControlContext = {
    service,
    status: null,
    logs: [],
    refreshToken,
    note: setNote,
    refresh: () => setRefreshToken((token) => token + 1),
    // The overlay is not a page, so it has no page id — and a control that
    // needed one would be a control reading a screen's identity, which is the
    // thing this file exists without.
    pageId: '',
  }

  return (
    <div className="switcher-overlay">
      {fault ? (
        <p className="ctl-error" role="alert">
          {fault}
        </p>
      ) : null}
      {summoned ? (
        <SwitcherPanelControl control={OVERLAY_CONTROL} ctx={ctx} />
      ) : (
        // Said in words, so a window that has not been summoned yet is not
        // indistinguishable from one whose tiles failed to arrive.
        <p className="ctl-empty">Waiting for the switcher chord.</p>
      )}
      {note ? <p className="ctl-value">{note}</p> : null}
    </div>
  )
}

export default SwitcherOverlay

/**
 * Mount only when this document is the one being loaded.
 *
 * The guard is the test's: switcher.test.tsx imports the component above to
 * assert what a trigger does, and a module that mounted itself on import would
 * put a second copy of this window in a document that has no layout to show it
 * in. The shell's own entry (main.tsx) has no such guard because nothing imports
 * it.
 */
const root = typeof document === 'undefined' ? null : document.getElementById('switcher-root')
if (root) {
  ReactDOM.createRoot(root).render(
    <StrictMode>
      <SwitcherOverlay />
    </StrictMode>,
  )
}
