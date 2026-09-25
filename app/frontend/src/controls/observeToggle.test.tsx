// The observe capture control (cross-os-pbd).
//
// One claim, tested hard: the sign on screen is a fact about the recorder,
// read from the recorder, and not an echo of the click that produced it.
// Everything else here is plumbing; this is the part that used to be a static
// caption saying "Turn Observe on" on a machine that was already observing.
//
// The structural assertions — a destructive button while running, a pulsing
// pill beside it, a distinct waiting state — are ports of Karabiner's capture
// control (CaptureInputEventsView.swift:15-32, CaptureActiveLabel.swift:16-49)
// and are worth pinning, because the reason for each is the reason the port
// exists: stopping a recorder is not the mirror of starting one.
//
// Fixtures use invented ids on purpose — a page or control id in src/ is
// daemon vocabulary, and shell.test.tsx greps for it.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { RenderResult } from '@testing-library/react'
import { renderControl } from './index'
import type { ControlContext } from './index'
import { machine, reset, stub, failOn } from '../test/fixtures'
import type { ServiceApi, Status } from '../types/controls'

const CONTROL = { kind: 'observeToggle', id: 'probe', label: 'Observe' }

function context(service: ServiceApi, refreshToken = 0): ControlContext {
  return {
    service,
    status: null as Status | null,
    logs: [],
    refreshToken,
    note: () => {},
    refresh: () => {},
    pageId: '',
  }
}

function show(service: ServiceApi, refreshToken = 0): RenderResult {
  return render(<>{renderControl(CONTROL, context(service, refreshToken))}</>)
}

const IDLE = /Start observing/
const RUNNING = /Stop observing/

afterEach(() => {
  cleanup()
  reset()
})

describe('the observe capture control', () => {
  it('reads the sign from the recorder, not from anything it could have clicked', async () => {
    // The write is broken in this build and the recorder is ON. A sign derived
    // from the control's own state would have nothing to derive from here and
    // would fall back to a default — which on this page is the one wrong
    // answer that matters.
    machine.observing = true
    failOn('SetObserve', 'this build cannot change the recorder')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('button', { name: RUNNING })).toBeTruthy())
    expect(screen.getByRole('status').textContent).toContain('Recording')
  })

  it('tracks the recorder across two reads, in both directions', async () => {
    const service = stub as unknown as ServiceApi
    const first = show(service)
    await waitFor(() => expect(screen.getByRole('button', { name: IDLE })).toBeTruthy())

    fireEvent.click(screen.getByRole('button', { name: IDLE }))
    await waitFor(() => expect(screen.getByRole('button', { name: RUNNING })).toBeTruthy())
    // The read, not the write: the stub only reports ON if the write landed.
    expect(await stub.ObserveState()).toEqual({ observe: true, mode: 'metadata-only' })

    fireEvent.click(screen.getByRole('button', { name: RUNNING }))
    await waitFor(() => expect(screen.getByRole('button', { name: IDLE })).toBeTruthy())
    expect(await stub.ObserveState()).toEqual({ observe: false, mode: 'metadata-only' })
    first.unmount()
  })

  it('follows a change it did not make', async () => {
    // The distinguishing property, and the one a fire-and-forget cannot have:
    // the recorder moves without this page clicking anything, and the sign
    // follows on the next read.
    const service = stub as unknown as ServiceApi
    const first = show(service, 0)
    await waitFor(() => expect(screen.getByRole('button', { name: IDLE })).toBeTruthy())

    machine.observing = true // another writer, behind the page's back
    first.unmount()

    const second = show(service, 1)
    await waitFor(() => expect(screen.getByRole('button', { name: RUNNING })).toBeTruthy())
    second.unmount()
  })

  it('offers a destructive stop and a live pill while recording', async () => {
    // The port's two structural claims. A destructive control, because
    // stopping a recorder is not the mirror of starting one
    // (CaptureInputEventsView.swift:16-19); and the sign BESIDE the control
    // that ends it rather than inside the button about to be pressed (:22).
    machine.observing = true
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('button', { name: RUNNING })).toBeTruthy())
    const stop = screen.getByRole('button', { name: RUNNING })
    expect(stop.className).toContain('ctl-stop')
    const pill = screen.getByRole('status')
    expect(pill.className).toContain('ctl-live')
    expect(pill.querySelector('.ctl-live-dot')).toBeTruthy()
  })

  it('withholds both directions while the daemon has not answered', async () => {
    // Karabiner's third state (CaptureActiveLabel.swift:38-49). While the
    // recorder's position is unknown this control offers NEITHER start nor
    // stop: pressing the wrong one on a machine that is already recording is
    // the failure this page exists to prevent.
    failOn('ObserveState', 'the daemon is not answering')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('status').textContent).toMatch(/Waiting/))
    expect(screen.queryByRole('button', { name: IDLE })).toBeNull()
    expect(screen.queryByRole('button', { name: RUNNING })).toBeNull()
    expect(screen.getByRole('button', { name: /Ask the daemon again/ })).toBeTruthy()
    // Still, not pulsing: a dot breathing beside a machine that has not
    // started is the one lie this control exists to avoid.
    expect(screen.getByRole('status').getAttribute('data-state')).toBe('waiting')
  })

  it('says what the recorder keeps, in the mode the daemon named', async () => {
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByText(/Recording:/)).toBeTruthy())
    expect(screen.getByText(/replaced/)).toBeTruthy()
  })

  it("keeps the control's own refusal in words", async () => {
    failOn('SetObserve', 'the recorder refused')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('button', { name: IDLE })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: IDLE }))
    await waitFor(() => expect(screen.getByText(/the recorder refused/)).toBeTruthy())
  })
})
