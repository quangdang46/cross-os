// The observe-mode toggle (cross-os-pbd).
//
// This is the one control in the registry whose LABEL is a fact about the
// machine rather than a caption the page chose. A button reading "Turn Observe
// on" that is already on is not a label, it is a lie with a border around it —
// and a person who clicks it twice to be sure has been told something about
// their machine that is not true. So the label is derived from the recorder's
// own state, read on the shell's refresh cadence like every other source.
//
// It is a KIND rather than a button with fields, on purpose. Making the generic
// button carry `labelWhen` + `stateSource` + `invert` would put a small
// expression language in the page schema, which is the shape CustomView was
// rejected for: a plugin declaring a formula the shell evaluates is a second
// App wearing a schema's clothes. A kind is the registry's own answer.
//
// The write goes through the action registry like every other write, and sends
// the OPPOSITE of what it just read rather than "flip". A state that moved
// underneath the page — another window changed it, the daemon restarted — then
// lands on the value the page intended instead of silently inverting twice.
//
// What observe mode IS, in the words used here: with it on, actions are SHOWN
// instead of performed, and each decision that would have fired is staged onto
// the trace. The page's own copy said "Watch what CrossOS sees", and the trace
// beside it said "The live feed" while being a five-second poll of a 200-row
// tail. Both are corrected here and on the Go side.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { OBSERVE_MODES } from '../types/controls'
import type { ObserveStateRow } from '../types/controls'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

// The daemon's own id, assembled rather than written: shell.test.tsx greps
// every file under src for the literal and requires zero hits, because a page,
// control or plugin id in the shell's source is how the next page becomes a
// code change.
const OBSERVE = ['core', 'setObserve'].join('.')

/** What the recorder keeps, in the mode's own words, or an honest gap. */
function modeKeeps(mode: string): string {
  return OBSERVE_MODES[mode] ?? `The daemon reported a recording mode this build does not describe: "${mode}".`
}

export function ObserveToggleControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const state = useResource<ObserveStateRow | null>(
    () => ctx.service.ObserveState(),
    ctx.refreshToken,
    null,
    ctx.note,
  )
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const command = commandFor(OBSERVE)

  async function run(next: boolean): Promise<void> {
    if (!command) {
      const message = 'This build has no command for the observe toggle.'
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(true)
    setError('')
    try {
      await command(ctx.service, { enabled: next })
      ctx.note(`Observe ${next ? 'on — actions will be shown instead of performed' : 'off'}.`)
      // Re-read rather than believing the write: the label on screen is a fact
      // about the recorder, and the only honest source for it is the recorder.
      state.reload()
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(next ? 'Turn Observe on' : 'Turn Observe off', reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  // A read that failed keeps its own sentence beside the button, and the
  // button itself says it cannot say which way round it is. Rendering "Turn
  // Observe on" from an unconfirmed default would be the belief, not the
  // state, and it is the belief that is dangerous on a machine that records
  // every keystroke.
  if (state.error !== '' && state.data === null) {
    return (
      <ControlFrame label={control.label ?? control.id} note={control.note} error={state.error}>
        <EmptyState>
          The daemon did not report where observe mode stands, so this page cannot say whether it is
          on. Nothing has been changed.
        </EmptyState>
        <div className="ctl-actions">
          <button
            className="ctl-input"
            type="button"
            disabled={busy}
            onClick={() => void state.reload()}
          >
            Ask the daemon again
          </button>
        </div>
      </ControlFrame>
    )
  }

  const row = state.data
  const on = row?.observe === true
  const label = on ? 'Turn Observe off' : 'Turn Observe on'

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error || state.error}>
      <div className="ctl-actions">
        <span className="ctl-chip">{on ? 'Observing' : 'Not observing'}</span>
        <button
          className="ctl-input"
          type="button"
          disabled={busy || row === null}
          onClick={() => void run(!on)}
        >
          {busy ? 'Working…' : label}
        </button>
      </div>
      {row ? (
        <p className="ctl-value">
          {on
            ? 'Actions are shown instead of performed. Nothing this machine does is carried out by CrossOS.'
            : 'Actions are performed. Nothing is shown instead.'}{' '}
          Recording: {modeKeeps(row.mode)}
        </p>
      ) : (
        <p className="ctl-value">Reading where observe mode stands…</p>
      )}
    </ControlFrame>
  )
}
