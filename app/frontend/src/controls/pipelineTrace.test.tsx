// The decision pipeline's copy and clear (bead w6-frontend-setup-renderers).
//
// The pipeline rows themselves are asserted in renderers.test.tsx, which owns
// the general "every kind draws something" sweep. What is tested HERE is the
// pair of operations the reference's event history offers over a list that is
// otherwise read-only, because the two are a matched set and neither is
// interesting alone:
//
//   - the copy, which is how the list leaves the window, and which must not be
//     able to hand somebody an empty document that reads as evidence;
//   - the clear, which is how the list stops existing, and which must not be
//     able to report success for a write the daemon refused.
//
// Both are ports of Karabiner's event history view:
//   InputEventHistoryView.swift:14-23  the two-format copy menu
//   InputEventHistoryView.swift:28-35 both disabled while the list is empty
//   EventHistory.swift:299-301         the clear is a real erase
//
// Fixtures use invented ids on purpose — shell.test.tsx greps every file under
// src for daemon vocabulary, and the erase's id is daemon vocabulary.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { RenderResult } from '@testing-library/react'
import { renderControl } from './index'
import type { ControlContext } from './index'
import { failOn, machine, reset, stub } from '../test/fixtures'
import type { ServiceApi, Status, TraceRow } from '../types/controls'

/**
 * The Wails clipboard, captured. The real module is replaced rather than
 * allowed to run: `Clipboard.SetText` posts to /wails/runtime over HTTP, and
 * under vitest the Wails vite plugin is off, so there is no runtime on the
 * other end of that fetch. Letting it through would make this file a test of
 * whether the machine has a Wails server, which is not the claim.
 *
 * This is the same seam switcher.test.tsx opens for Window, and it is what
 * makes the assertions below possible at all: a test cannot check what landed
 * on the clipboard if the call throws before anything is written.
 */
const clipboard: { text: string; calls: number; refuse: string | null } = {
  text: '',
  calls: 0,
  refuse: null,
}

vi.mock('@wailsio/runtime', () => ({
  Clipboard: {
    SetText: (text: string) => {
      clipboard.calls += 1
      if (clipboard.refuse !== null) {
        return Promise.reject(new Error(clipboard.refuse))
      }
      clipboard.text = text
      return Promise.resolve()
    },
  },
}))

const CONTROL = { kind: 'pipelineTrace', id: 'probe', label: 'Decisions' }

const COPY_JSON = /Copy as JSON/
const COPY_TSV = /Copy as TSV/
const CLEAR = /Clear recorded decisions/

/** A decision with one field of each kind an export copies, so a formatter
 *  that drops a column is visible in the pasted text. */
function trace(over: Partial<TraceRow> = {}): TraceRow {
  return {
    at: '2026-01-02T03:04:05Z',
    decision: 'replace',
    event: { keys: 'cmd+Left', source: 'keyboard', device: 'built-in', key_code: 21 },
    context: { app_id: 'com.example.editor', app_mode: 'native' },
    winner: 'window-keys.snap-left',
    losers: ['other.claim', 'third.claim'],
    intent: 'nav.left',
    action: 'winlayout.snapLeft',
    params: 'fraction=0.5',
    stages: [
      { stage: 'event', detail: 'cmd+Left from built-in' },
      { stage: 'rule', detail: 'window-keys.snap-left matched' },
    ],
    ...over,
  }
}

/** The decisions the daemon is serving, newest write last. */
function serve(rows: TraceRow[]): void {
  machine.decisions = rows
}

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

/** One act() turn, enough for a useResource load to land. */
async function oneTurn(): Promise<void> {
  await act(async () => {
    await Promise.resolve()
  })
}

/** The button whose visible text matches, asserted to exist first. */
function button(name: RegExp): HTMLButtonElement {
  const found = screen.getByRole('button', { name })
  expect((found as HTMLButtonElement).disabled, `${name} is offered live`).toBe(false)
  return found as HTMLButtonElement
}

afterEach(() => {
  cleanup()
  reset()
  clipboard.text = ''
  clipboard.calls = 0
  clipboard.refuse = null
})

describe('the pipeline, empty', () => {
  beforeEach(() => {
    serve([])
  })

  it('offers neither a copy nor a clear over a list with nothing in it', async () => {
    // Karabiner disables both (InputEventHistoryView.swift:28,35) and the
    // reason is worth keeping: a clear over an empty list can only ever claim
    // to have done something, and a copy over one pastes "[]" — a document that
    // reads as a report of what the recorder saw and is not one.
    show(stub as unknown as ServiceApi)
    await screen.findByText(/Nothing has been decided yet/)
    for (const name of [COPY_JSON, COPY_TSV, CLEAR]) {
      const found = screen.getByRole('button', { name })
      expect((found as HTMLButtonElement).disabled, `${name} is disabled over an empty list`).toBe(
        true,
      )
    }
  })

  it('still says the list is empty rather than drawing nothing', async () => {
    show(stub as unknown as ServiceApi)
    expect(await screen.findByText(/Nothing has been decided yet/)).toBeTruthy()
  })
})

describe('copying the pipeline out', () => {
  beforeEach(() => {
    serve([trace()])
  })

  it('puts the decisions on the clipboard as JSON, and names every column', async () => {
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(COPY_JSON))
    await waitFor(() => expect(clipboard.calls).toBe(1))

    const parsed = JSON.parse(clipboard.text) as Record<string, string>[]
    expect(parsed).toHaveLength(1)
    // The fields a reader cannot reconstruct from the page: who won, which
    // rules lost, and the parameters the recorder already redacted. An export
    // carrying only the chord would be a worse screenshot of the same list.
    expect(parsed[0].winner).toBe('window-keys.snap-left')
    expect(parsed[0].losers).toBe('other.claim, third.claim')
    expect(parsed[0].params).toBe('fraction=0.5')
    expect(parsed[0].at).toBe('2026-01-02T03:04:05Z')
    expect(parsed[0].stages).toContain('rule: window-keys.snap-left matched')
  })

  it('puts a header row first in TSV, so the columns are named once they leave', async () => {
    // A column of bare timestamps is unreadable out of this window, and whoever
    // opens the file is the one who has to know what the columns are.
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(COPY_TSV))
    await waitFor(() => expect(clipboard.calls).toBe(1))

    const lines = clipboard.text.split('\n')
    expect(lines[0]).toBe(
      'Timestamp\tDecision\tKeys\tSource\tApp\tApp mode\tWinner\tLosers\tIntent\tAction\tParams\tStages',
    )
    expect(lines[1]).toContain('window-keys.snap-left')
    // And the file ends at a line boundary: some readers treat a final line
    // with no terminator as one row short.
    expect(clipboard.text.endsWith('\n')).toBe(true)
  })

  it('replaces a tab or a newline inside a value rather than splitting the row', async () => {
    // The load-bearing part of the TSV port (EventHistory.swift:362-368). TSV
    // has no quoting convention, so an embedded tab is a column break and an
    // embedded newline is a row break — and the file still opens, which is what
    // makes the corruption so easy to miss. A params string carrying a tab is
    // entirely possible: it is whatever the action put there.
    serve([trace({ params: 'text=hello\tworld\nsecond line' })])
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(COPY_TSV))
    await waitFor(() => expect(clipboard.calls).toBe(1))

    const body = clipboard.text.split('\n').filter((line) => line !== '')
    // One header, one decision. A newline that survived would have made three.
    expect(body).toHaveLength(2)
    expect(body[1]).toContain('text=hello world second line')
  })

  it('says how many decisions were copied, so a paste is not a guess', async () => {
    serve([trace(), trace({ at: '2026-01-02T03:04:06Z' })])
    show(stub as unknown as ServiceApi)
    await screen.findByText(/2 decisions/)
    fireEvent.click(button(COPY_JSON))
    expect(await screen.findByText(/Copy as JSON — 2 decisions\./)).toBeTruthy()
  })

  it('copies the whole tail, not the page, so the count and the rows agree', async () => {
    // The control draws newest first and exports in the daemon's own order.
    // Those are two orders of one list, and the export is the one somebody
    // pastes into a bug report — so the count in the confirmation has to be the
    // count that is actually in the clipboard.
    const rows = [trace({ at: '2026-01-02T03:04:05Z' }), trace({ at: '2026-01-02T03:04:06Z' })]
    serve(rows)
    show(stub as unknown as ServiceApi)
    await screen.findByText(/2 decisions/)
    fireEvent.click(button(COPY_JSON))
    await waitFor(() => expect(clipboard.calls).toBe(1))
    expect((JSON.parse(clipboard.text) as unknown[]).length).toBe(rows.length)
  })

  it('says a copy that could not be made, and does not claim the clipboard took it', async () => {
    clipboard.refuse = 'no clipboard on this desktop'
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(COPY_JSON))
    const alert = await screen.findByText(/did not go through/)
    expect(alert.textContent).toContain('no clipboard on this desktop')
    // Nothing was erased, and the control says so rather than leaving the
    // reader to work out that a failed copy is not a lost list.
    expect(alert.textContent).toContain('nothing was erased')
    expect(clipboard.text).toBe('')
  })
})

describe('clearing the pipeline', () => {
  beforeEach(() => {
    serve([trace()])
  })

  it('empties the list through the daemon and redraws from the daemon, not from a click', async () => {
    // The whole reason the verb answers with the list: redrawing from the
    // reply is what makes the rows on screen a fact about the recorder rather
    // than a belief about the button.
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(CLEAR))

    expect(await screen.findByText(/Nothing has been decided yet/)).toBeTruthy()
    // The fixture empties the same list the read serves, so this is the
    // recorder's state and not the control's.
    expect(await (stub as unknown as ServiceApi).Traces()).toEqual([])
  })

  it('says how many it cleared, because an empty list is also what "never recorded" looks like', async () => {
    // The empty list that replaces the rows is the only evidence a person has,
    // and it is identical to the screen a machine that never recorded anything
    // draws. "1 decision cleared" and "there was never anything" must not be
    // the same picture.
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(CLEAR))
    expect(await screen.findByText(/1 decision cleared from the recorder\./)).toBeTruthy()
  })

  it('refuses to report a clear the daemon did not perform', async () => {
    // A keystroke log somebody believes they destroyed and that is still on
    // disk is not a small lie, so a failure is shown as one — and the rows
    // stay on screen, because they are still there.
    failOn('TracesClear', 'the recorder is busy')
    show(stub as unknown as ServiceApi)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(CLEAR))

    const alert = await screen.findByText(/did not go through/)
    expect(alert.textContent).toContain('the recorder is busy')
    // Still drawn. The failure says the clear did not happen; it does not also
    // hide the list it did not erase.
    expect(screen.getByText(/com\.example\.editor/)).toBeTruthy()
    expect(await (stub as unknown as ServiceApi).Traces()).toHaveLength(1)
  })

  it('does not report success when the daemon answers with rows it says it kept', async () => {
    // A reply that is not the emptied list is the case a count-returning verb
    // would produce, and a control that trusted the promise rather than the
    // rows would say "cleared" over a recorder that still holds everything.
    const service = {
      ...(stub as unknown as ServiceApi),
      TracesClear: () => Promise.resolve([trace()]),
    } as unknown as ServiceApi
    show(service)
    await screen.findByText(/com\.example\.editor/)
    fireEvent.click(button(CLEAR))

    expect(await screen.findByText(/Nothing was erased/)).toBeTruthy()
    expect(screen.getByText(/com\.example\.editor/)).toBeTruthy()
  })

  it('disables the clear while the write is in flight, and re-enables it after', async () => {
    // A window that leaves the button live during the write takes the same
    // erase twice, and the second lands after the first with no way to tell
    // which one stuck. The write is HELD open here so the assertion is about
    // the control and not about how fast a promise happened to resolve.
    let release = (): void => {}
    const held = new Promise<TraceRow[]>((resolve) => {
      release = () => resolve([])
    })
    const service = {
      ...(stub as unknown as ServiceApi),
      TracesClear: () => held,
    } as unknown as ServiceApi
    show(service)
    await screen.findByText(/com\.example\.editor/)

    fireEvent.click(button(CLEAR))
    await oneTurn()
    const during = screen.getByRole('button', { name: /Clearing…/ })
    expect((during as HTMLButtonElement).disabled).toBe(true)

    await act(async () => {
      release()
      await held
    })
    await waitFor(() =>
      expect(
        (screen.getByRole('button', { name: CLEAR }) as HTMLButtonElement).disabled,
      ).toBe(false),
    )
  })

  it('refuses a clear when the list it was offered over is empty', async () => {
    // The disabled button is the reference's rule; this is the half that says
    // the control enforces it rather than relying on the browser, because a
    // keyboard user can and does press disabled-adjacent things.
    serve([])
    show(stub as unknown as ServiceApi)
    await screen.findByText(/Nothing has been decided yet/)
    const found = screen.getByRole('button', { name: CLEAR })
    fireEvent.click(found as HTMLButtonElement)
    await oneTurn()
    expect(await (stub as unknown as ServiceApi).Traces()).toEqual([])
  })
})
