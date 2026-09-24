// The trial countdown and its three buttons (bead cross-os-itq).
//
// This is the control that replaces the window.prompt stand-in App.tsx carried
// for confirm and rollback. A prompt cannot be styled with the shell's own
// vocabulary, cannot be reached without a mouse, hands the user a free-text box
// where a plugin id belongs, and disappears the moment it is dismissed — so
// the plugin is picked from a list the daemon served and the trial in flight
// is shown, not asked about.
//
// The Confirm flag is worth reading twice. ConfirmTrial(pluginID, confirmed,
// healthy) is the daemon's fail-closed gate (§8.4): `confirmed` is the user's
// click, which is why this button is the only thing that sets it true, but
// `healthy` is a claim about the daemon's own state and is read from
// status.Plugins. When the shell cannot see the plugin it passes false — a
// health flag the frontend invented is exactly the auto-approval the gate
// exists to prevent, and the daemon will then say why it refused.
//
// The one-second tick below is a clock, not a poll: it redraws a duration the
// daemon already handed over. The trial state itself is re-read only when
// App.tsx bumps refreshToken, which is what keeps a control from talking to the
// daemon on a timer of its own.
//
// Nothing expires a trial on the daemon side yet. safety.trialState is a pure
// read, and only confirm and rollback remove an entry from the map, so an
// elapsed trial keeps reporting state "trial" with nothing left. The countdown
// therefore says what is actually true — the window is closed, the way out is
// rollback — and drops the Confirm button, which from here on can only abort
// the trial and report "TRIAL expired".

import { useEffect, useState } from 'react'
import type { ReactElement } from 'react'
import type { TrialState } from '../types/controls'
import { failedTo } from '../lib/wire'
import { formatCountdown, humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, SelectField, TextField } from './common'
import type { ControlProps } from './common'

export function TrialControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const trial = useResource<TrialState | null>(
    () => ctx.service.TrialState(),
    ctx.refreshToken,
    null,
    ctx.note,
  )
  const [plugin, setPlugin] = useState('')
  const [reason, setReason] = useState('Rolled back from the Settings page')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [outcome, setOutcome] = useState('')
  // Wall-clock anchor rather than a decrementing counter: a backgrounded
  // webview throttles timers, and a counter would show a trial with more time
  // left than it has. `now - deadline` cannot drift.
  const [deadline, setDeadline] = useState(0)
  const [now, setNow] = useState(() => Date.now())

  const state = trial.data
  const running = state !== null && state !== undefined && state.state !== 'none'
  const plugins = ctx.status?.Plugins ?? []

  useEffect(() => {
    if (state && state.state !== 'none' && state.remaining_ms > 0) {
      setDeadline(Date.now() + state.remaining_ms)
    } else {
      setDeadline(0)
    }
  }, [state])

  useEffect(() => {
    if (deadline === 0) return
    const tick = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(tick)
  }, [deadline])

  const remaining = deadline === 0 ? 0 : Math.max(0, deadline - now)
  // Which buttons exist is the page's decision: core.safety declares
  // safety.confirmTrial and safety.rollbackTrial on this control, and a page
  // that does not declare them gets no buttons for them.
  const declared = Array.isArray(control.actions) ? control.actions : []
  const canConfirm = declared.includes('safety.confirmTrial')
  const canRollback = declared.includes('safety.rollbackTrial')
  const chosen = plugin || state?.plugin || plugins[0]?.ID || ''

  async function send(key: string, action: string, call: () => Promise<string>): Promise<void> {
    setBusy(key)
    setError('')
    setOutcome('')
    try {
      const result = await call()
      setOutcome(result)
      ctx.note(`${action} → ${result || 'done'}`)
      // Re-read now rather than waiting for the next refresh tick: the buttons
      // on screen must match the trial that is actually running.
      trial.reload()
    } catch (reason) {
      const message = failedTo(action, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  function start(): void {
    void send('start', `Starting a trial for ${chosen}`, () => ctx.service.BeginTrial(chosen))
  }

  function confirm(): void {
    const healthy = plugins.some((p) => p.ID === state?.plugin && p.Healthy === 'healthy')
    void send('confirm', `Keeping ${state?.plugin ?? ''} enabled`, () =>
      ctx.service.ConfirmTrial(state?.plugin ?? '', true, healthy),
    )
  }

  function rollback(): void {
    void send('rollback', `Rolling back ${state?.plugin ?? ''}`, () =>
      ctx.service.RollbackTrial(state?.plugin ?? '', reason.trim() || 'user rollback'),
    )
  }

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error || trial.error}>
      {running && state ? (
        <>
          <p className="ctl-value">
            {humanize(state.plugin)} is in its trial ({state.state}).
          </p>
          {/* aria-live off, stated explicitly: a countdown that announced itself
              would say a number sixty times a minute. The remaining time is
              still plain text, so a screen reader reads it on demand. */}
          <p className="ctl-countdown" aria-live="off">
            {formatCountdown(remaining)} left
            {remaining === 0 ? ' — time is up; roll it back to clear it.' : ''}
          </p>
          {canConfirm && remaining > 0 ? (
            <div className="ctl-actions">
              <button className="ctl-input" type="button" disabled={busy !== ''} onClick={confirm}>
                {busy === 'confirm' ? 'Working…' : 'Keep it enabled'}
              </button>
            </div>
          ) : null}
          {canRollback ? (
            <>
              <TextField
                label="Why are you rolling back?"
                value={reason}
                onChange={setReason}
                disabled={busy !== ''}
              />
              <div className="ctl-actions">
                <button className="ctl-input" type="button" disabled={busy !== ''} onClick={rollback}>
                  {busy === 'rollback' ? 'Working…' : 'Roll back this trial'}
                </button>
              </div>
            </>
          ) : null}
        </>
      ) : (
        <>
          <p className="ctl-value">No trial is running.</p>
          {plugins.length === 0 ? (
            <EmptyState>No plugins are installed, so there is nothing to try yet.</EmptyState>
          ) : (
            <>
              <SelectField
                label="Plugin to try"
                value={chosen}
                options={plugins.map((p) => ({ value: p.ID, text: humanize(p.ID) }))}
                onChange={setPlugin}
                disabled={busy !== ''}
              />
              <div className="ctl-actions">
                <button className="ctl-input" type="button" disabled={busy !== ''} onClick={start}>
                  {busy === 'start' ? 'Working…' : 'Start its trial'}
                </button>
              </div>
            </>
          )}
        </>
      )}
      {outcome ? <p className="ctl-value">Result: {outcome}</p> : null}
    </ControlFrame>
  )
}
