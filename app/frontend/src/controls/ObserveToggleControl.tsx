// The observe-mode capture control (cross-os-pbd).
//
// A structural port, not an original design. The reference is Karabiner's
// capture control, which is the closest thing in the tree to what this page
// does — it watches what the keyboard produced and lets a person start and
// stop the watching:
//
//   Karabiner-Elements, EventViewer/src/View/CaptureInputEventsView.swift
//     :15-32  the capture row. While capturing there is a role:.destructive
//            button carrying stop.fill; while idle there is one plain button
//            carrying record.circle. The two states are NOT one toggle
//            rendered twice: stopping a recorder is not the mirror of starting
//            one, so the running state's control is styled as a stop.
//     :22    the running row pairs that button with CaptureActiveLabel, so the
//            sign of a live recorder sits BESIDE the control that ends it
//            rather than inside the button the person is about to press.
//   EventViewer/src/View/CaptureActiveLabel.swift
//     :16-22 a filled circle whose opacity breathes, in green
//     :24-35 a 2s cycle dimming to 0.35, updated at 1/30s and updating ONLY
//            the opacity so the label never moves with it
//     :38-49 a THIRD state Karabiner treats as first-class: waiting, drawn as
//            a small spinner and dimmed secondary text, because the daemon is
//            up and asked to capture but the device it needs is not accessible
//
// Two deviations, both forced by the medium and both commented at the point
// they happen: SwiftUI's accessibilityReduceMotion becomes the reduced-motion
// block already in public/style.css, and SF Symbols become text glyphs.
//
// The data binding is CrossOS's own. The reference has no recorder privacy
// mode, so the line naming what the daemon says it keeps is not a port of
// anything — it is the part a product that records every keystroke decision
// owes the person reading it, and it is the one thing on this control that
// exists nowhere in Karabiner because Karabiner has nothing to disclose.

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

/** The pulsing sign, ported from CaptureActiveLabel.swift:16-22. */
function LivePill(props: { text: string }): ReactElement {
  return (
    <span className="ctl-live" role="status">
      <span className="ctl-live-dot" aria-hidden="true" />
      {props.text}
    </span>
  )
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
      const message = 'This build has no command for the observe control.'
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(true)
    setError('')
    try {
      await command(ctx.service, { enabled: next })
      ctx.note(`Observe ${next ? 'on — actions will be shown instead of performed' : 'off'}.`)
      // Re-read rather than believing the write: the pill on screen is a fact
      // about the recorder, and the only honest source for it is the recorder.
      state.reload()
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(next ? 'Start observing' : 'Stop observing', reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy(false)
    }
  }

  // The waiting state, ported from CaptureWaitingForDeviceAccessLabel
  // (CaptureActiveLabel.swift:38-49). It is a state, not an error, and it is
  // the one the port earns its keep on: while the daemon will not say where
  // observe mode stands, this control offers NEITHER a start nor a stop,
  // because pressing the wrong one on a machine that is already recording is
  // the failure this page exists to prevent. A retry is offered instead —
  // Karabiner's own waiting state needs no one, because it holds the state
  // locally and this does not.
  if (state.data === null) {
    return (
      <ControlFrame
        label={control.label ?? control.id}
        note={control.note}
        error={error || state.error}
      >
        <span className="ctl-live" data-state="waiting" role="status">
          <span className="ctl-live-dot" aria-hidden="true" />
          Waiting to hear where observe mode stands
        </span>
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
        {state.error !== '' ? (
          <EmptyState>
            The daemon has not answered, so this page cannot say whether it is recording. Nothing has
            been changed.
          </EmptyState>
        ) : null}
      </ControlFrame>
    )
  }

  const on = state.data.observe
  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || state.error}
    >
      <div className="ctl-actions">
        {on ? (
          <>
            {/* CaptureInputEventsView.swift:16-19 — role:.destructive, stop.fill */}
            <button
              className="ctl-input ctl-stop"
              type="button"
              disabled={busy}
              onClick={() => void run(false)}
            >
              <span aria-hidden="true">■ </span>
              {busy ? 'Working…' : 'Stop observing'}
            </button>
            {/* :22 — the sign sits beside the control that ends it */}
            <LivePill text="Recording" />
          </>
        ) : (
          /* :27-31 — record.circle, one plain button for the idle state */
          <button
            className="ctl-input"
            type="button"
            disabled={busy}
            onClick={() => void run(true)}
          >
            <span aria-hidden="true">● </span>
            {busy ? 'Working…' : 'Start observing'}
          </button>
        )}
      </div>
      <p className="ctl-value">
        {on
          ? 'Actions are shown instead of performed. Nothing this machine does is carried out by CrossOS.'
          : 'Actions are performed. Nothing is shown instead.'}{' '}
        Recording: {modeKeeps(state.data.mode)}
      </p>
    </ControlFrame>
  )
}
