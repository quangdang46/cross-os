// The observe toggle's label (cross-os-pbd).
//
// One claim, tested hard: the words on the button are a fact about the
// recorder, read from the recorder, and not an echo of the click that produced
// them. Everything else this control does is plumbing; this is the part that
// was previously a static caption saying "Turn Observe on" on a machine that
// was already observing.
//
// Fixtures use invented ids on purpose — a page or control id in src/ is
// daemon vocabulary, and shell.test.tsx greps for it.

import { afterEach, describe, expect, it } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

function toggle(): HTMLElement {
  return screen.getByRole('button', { name: /Turn Observe (on|off)/ }) as HTMLElement
}

afterEach(() => {
  cleanup()
  reset()
})

describe('the observe toggle', () => {
  it('reads the label from the recorder, not from anything it could have clicked', async () => {
    // The write is broken in this build, and the recorder is ON. A label
    // derived from the button's own state would have nothing to derive from
    // here and would fall back to a default — which on this page is the one
    // wrong answer that matters.
    machine.observing = true
    failOn('SetObserve', 'this build cannot change the recorder')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe off/ })).toBeTruthy())
  })

  it('tracks the recorder across two reads, in both directions', async () => {
    const service = stub as unknown as ServiceApi
    const first = show(service)
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe on/ })).toBeTruthy())

    fireEvent.click(toggle())
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe off/ })).toBeTruthy())
    // The read, not the write: the stub only reports OFF if the write landed.
    expect(stub.ObserveState).toBeTypeOf('function')
    await act(async () => {
      expect(await stub.ObserveState()).toEqual({ observe: true, mode: 'metadata-only' })
    })

    fireEvent.click(toggle())
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe on/ })).toBeTruthy())
    await act(async () => {
      expect(await stub.ObserveState()).toEqual({ observe: false, mode: 'metadata-only' })
    })
    first.unmount()
  })

  it('follows a change it did not make', async () => {
    // The distinguishing property, and the one a fire-and-forget cannot have:
    // the recorder moves without this page clicking anything, and the label
    // follows on the next read. A control holding its own belief would keep
    // offering the other position until the person clicked to find out.
    const service = stub as unknown as ServiceApi
    const { unmount } = show(service, 0)
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe on/ })).toBeTruthy())

    // Another writer: the daemon's own state, set behind the page's back.
    machine.observing = true
    unmount()

    const second = show(service, 1)
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe off/ })).toBeTruthy())
    second.unmount()
  })

  it('says what the recorder keeps, in the mode the daemon named', async () => {
    const service = stub as unknown as ServiceApi
    show(service)
    await waitFor(() => expect(screen.getByText(/Recording:/)).toBeTruthy())
    expect(screen.getByText(/replaced/)).toBeTruthy()
  })

  it('will not say which way round it is when the daemon will not answer', async () => {
    failOn('ObserveState', 'the daemon is not answering')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByText(/did not report where observe mode stands/)).toBeTruthy())
    // The dangerous failure is a confident wrong label drawn from a default.
    expect(screen.queryByRole('button', { name: /^Turn Observe on$/ })).toBeNull()
    expect(screen.getByRole('button', { name: /Ask the daemon again/ })).toBeTruthy()
  })

  it("keeps the button's own refusal in words", async () => {
    failOn('SetObserve', 'the recorder refused')
    show(stub as unknown as ServiceApi)
    await waitFor(() => expect(screen.getByRole('button', { name: /Turn Observe on/ })).toBeTruthy())
    fireEvent.click(toggle())
    await waitFor(() => expect(screen.getByText(/the recorder refused/)).toBeTruthy())
  })
})
