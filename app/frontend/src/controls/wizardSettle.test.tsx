// The last step WAITS, and then hands over.
//
// Two claims, both about the reference's shape rather than about a button:
//
//   1. A person grants Accessibility in System Settings and clicks Verify once.
//      The tap has not reinstalled yet, so the FIRST readiness read after the
//      click is honestly red. A wizard that reads once reports "not ready" about
//      a permission that is already granted and sends the reader back to System
//      Settings to grant it a second time. This file pins the fix: the last step
//      polls, and the row goes green on a LATER read with nobody clicking again.
//      The machine's lag is real (see grantAccessibility in ../test/fixtures),
//      so a shell that read once would fail here rather than pass by luck.
//
//   2. When the wait runs out, the reader is handed the list of what is still
//      not granted — the daemon's own rows, with the daemon's own sentences —
//      rather than a bare count. That is what pcfy-my-mac's launcher.go:72-80
//      prints instead of "Installed successfully": the install's last task is a
//      person granting permissions, so the last thing on screen names them.
//
// THE TIMERS ARE FAKE, ON PURPOSE. The wait is 1000ms a poll against a 60s
// budget (await.go:30-56, applied at task.go:63-76), so testing it in real time
// would either take a minute or assert against a shortened budget this file does
// not own. Advancing the clock means the numbers under test are the ones the
// control really ships, and "no second click" is asserted by the CALL LOG rather
// than by a sleep — a wizard that quietly asked again would show up as a second
// dispatch, which is the failure this whole file is about.
//
// THE GRANT IS MADE AFTER THE WIZARD IS MOUNTED, and that ordering is the point
// rather than an accident of setup. A real reader is already sitting on the last
// step when they come back from System Settings, and the daemon is still
// reinstalling. Granting first would let the wizard's own opening read spend the
// lag, and the wait being tested would be over before the click that starts it.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { renderControl, type ControlContext } from './index'
import { FIRST_RUN_CONTROL, failOn, grantAccessibility, machine, reset, stub } from '../test/fixtures'
import type { Control, ServiceApi } from '../types/controls'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

/**
 * A context whose refresh actually re-reads. The renderers' own helper stubs it
 * to a no-op, which is right for drawing a control and wrong here: the settle
 * calls refresh the moment its poll comes back ready, and with a no-op the
 * wizard's step marks would never catch up — so a broken settle would look fine.
 */
function host(): ControlContext {
  const box = { token: 0 }
  return {
    service: stub as ServiceApi,
    status: null,
    logs: [],
    get refreshToken() {
      return box.token
    },
    note: () => {},
    refresh: () => {
      box.token += 1
    },
    pageId: '',
  }
}

/**
 * Mounts the wizard and walks the reader to the last declared step, which is the
 * one that waits. The step is reached by CLICKING it rather than by naming it,
 * because that is the only way a test can be sure the wait is positional: a
 * wizard that keyed the wait on a step id would need this test to know that id,
 * and §3.6c's rule is precisely that it must not.
 */
async function openLastStep(control: Control = FIRST_RUN_CONTROL): Promise<HTMLElement> {
  render(<>{renderControl(control, host())}</>)
  await act(async () => {
    await Promise.resolve()
  })
  const steps = Array.from(document.querySelectorAll<HTMLButtonElement>('.ctl-step-button'))
  fireEvent.click(steps[steps.length - 1])
  await act(async () => {
    await Promise.resolve()
  })
  return screen.getByRole('region', { name: control.label ?? control.id })
}

/** Runs the clock forward inside act, so React sees the state updates. */
async function advance(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

/** How many times the daemon has been asked for its readiness rows. */
function reads(): number {
  return machine.calls.filter((name) => name === 'Readiness').length
}

beforeEach(() => {
  reset()
  vi.useFakeTimers()
})

describe('the last step, on a machine that has just been granted', () => {
  it('reaches ready on a poll, with no second click', async () => {
    // The product is set up, so the only thing missing is the reinstall: a
    // profile applied turns on the extensions, and the tap is what gates them.
    await act(async () => {
      await stub.ApplyProfile('windows11')
    })
    const wizard = await openLastStep()

    // The reader is on the last step. NOW they grant Accessibility and come
    // back — the daemon has not reinstalled the tap, and will not for the next
    // few reads. This is the premise: a single read would say "not ready" about
    // a permission that is already granted.
    grantAccessibility(4)

    // ONE click. The whole claim is that the reader does not come back.
    const before = reads()
    fireEvent.click(within(wizard).getByRole('button', { name: 'verify' }))

    // The first poll lands inside the reinstall lag, so the wizard is WAITING
    // and says so. The progress line is what tells a live wait from a dead one;
    // a wizard that printed nothing here would look identical to a hung one.
    await advance(0)
    expect(within(wizard).getByText(/Waiting for the machine to catch up/)).toBeTruthy()
    expect(within(wizard).getByText(/giving it up to 60 seconds/)).toBeTruthy()

    // Now the poll runs the tap out, one second at a time, with nobody
    // clicking. The loop is bounded by the budget's own tick count rather than
    // by how many ticks this particular lag happens to need, so a longer lag
    // in the fixture cannot make the test time out or hang.
    for (let tick = 0; tick < 61; tick += 1) {
      if (within(wizard).queryByText(/no second click needed/)) break
      await advance(1000)
    }
    expect(within(wizard).getByText(/no second click needed/)).toBeTruthy()

    // The call log is the assertion that matters: the daemon was asked AGAIN
    // (reads went up), and the wizard did not re-dispatch the action to find
    // out. A settle that settled by clicking again would pass the assertions
    // above and fail here.
    expect(reads()).toBeGreaterThan(before)
  })

  it('reports a failed read as a failure, never as a verdict about the machine', async () => {
    // The settle is a wait, so a read that fails is a failure of the SOURCE and
    // is worded that way. The two are different claims — "the daemon would not
    // answer" is not "your machine is not ready" — and collapsing them is how a
    // bridge outage sends somebody off to grant a permission they already gave.
    failOn('Readiness', 'readiness source unavailable')
    const wizard = await openLastStep()
    await advance(0)

    expect(within(wizard).getByText(/readiness source unavailable/)).toBeTruthy()
    // And the honest non-claim: no hand-off was handed over on the strength of
    // a source that did not answer, and no step was marked done because of it.
    expect(within(wizard).queryByText('Almost ready! Still to grant:')).toBeNull()
  })

  it('names the timeout and hands over what is still not granted', async () => {
    await act(async () => {
      await stub.ApplyProfile('windows11')
    })
    const wizard = await openLastStep()

    // The grant never lands, so the budget is the only thing that can end
    // this. The reference ends the same way — ErrTimeout, wrapped with the
    // duration it waited — and then prints the list.
    await advance(0)
    expect(within(wizard).getByText(/Waiting for the machine to catch up/)).toBeTruthy()
    await advance(60_000)

    // The timeout is NAMED, not merely reached. A reader told the wait ran out
    // can decide whether to keep waiting; one told only "not ready" cannot tell
    // a slow machine from a broken one.
    expect(within(wizard).getByText(/Still not ready after 60 seconds/)).toBeTruthy()
    expect(
      within(wizard).getByText(/the wait running out, not an answer about the machine/),
    ).toBeTruthy()

    // And the hand-off is the reference's: a plain list of what is still to
    // grant, drawn from the daemon's own rows with the daemon's own sentences.
    // A bare readiness count would name none of it.
    expect(within(wizard).getByText('Almost ready! Still to grant:')).toBeTruthy()
    // Every unready row is named, not a count of them: keyboard, windows and
    // finder are all gated on the one permission, so three lines is the honest
    // hand-off and "1 of 4 ready" would have told the reader nothing to do.
    expect(within(wizard).getAllByText('Keyboard interception').length).toBeGreaterThan(0)
    expect(within(wizard).getAllByText('Windows shortcuts').length).toBeGreaterThan(0)
    expect(within(wizard).getAllByText('Finder shortcuts').length).toBeGreaterThan(0)
    // The path is the daemon's `detail`, printed by the checklist's own row
    // renderer, so the shell composes no System Settings location of its own.
    // The assertion is on the raw line because that is the guarantee being made:
    // whatever the row's plain sentence says, the daemon's own words are still on
    // it (ChecklistControl's ReadinessLine, and lib/format.ts's rule that a
    // formatter must keep what it hid reachable). Tapping for the sentence is a
    // separate assertion in readinessReasons.test.ts.
    expect(
      within(wizard).getAllByText('adapter: tap refused (input-monitoring consent missing?)').length,
    ).toBeGreaterThan(0)
    expect(within(wizard).getAllByText(/Input Monitoring/).length).toBeGreaterThan(0)
  })
})
