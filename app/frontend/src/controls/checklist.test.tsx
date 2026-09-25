// The readiness checklist, asserted against the daemon's answer and against
// the answers it did not give.
//
// ONE claim is load-bearing here, and the rest of this file exists to hold it
// in place. "Not checked" is a sentence about the DAEMON: it went through the
// list, and this id was not in it. It is only true once a read has landed. A
// read still in flight has not gone through anything, and a read that FAILED
// certainly did not — so drawing a declared id as "Not checked" in either of
// those states tells somebody to wait for an answer that is not coming. On the
// first-run page, where a person decides whether they are finished, that is the
// most expensive sentence this control could get wrong: a person who granted
// every permission walks away believing the checklist was still working.
//
// So the rule the tests pin is: a verdict is a row the daemon FILLED IN. The
// rows it filled in are always drawn — those are facts, and hiding them because
// a later read failed would throw away the truth. What is withheld is the
// verdict on everything it did not answer, and in its place there is one line
// saying why there is not one.
//
// The reference is menumate's onboarding diagnosis, where a check whose state
// the app cannot read carries no verdict mark at all rather than a red one
// (App/UI/OnboardingView.swift:210-213 — `granted` is a Bool?, and the icon is
// drawn only when it is non-nil). A row with no mark says "not known"; a row
// with a mark is a claim about the machine.
//
// Fixtures use invented check ids on purpose. A page id in src/ is daemon
// vocabulary and test/shell.test.tsx greps every file under src for the
// namespaced prefix, so nothing here may spell one out.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { Control, ReadinessRow, ServiceApi, Status } from '../types/controls'
import { renderControl, type ControlContext } from './index'

afterEach(() => cleanup())

/** A daemon whose readiness read answers `rows`, rejects outright, or never
 *  settles. `thenReject` is the case worth having on its own: the FIRST read
 *  lands and a LATER one fails, which is the only shape in which rows and an
 *  error are both on screen. The shared loader keeps whatever a read that DID
 *  answer put there and does not clear it on a failure, so the rows survive
 *  and the question this file asks is what the control does with them. */
function daemon(answer: {
  rows?: ReadinessRow[]
  reject?: string
  thenReject?: string
  hold?: boolean
} = {}): ServiceApi {
  const held = new Promise<ReadinessRow[]>(() => {})
  let reads = 0
  return {
    Readiness: () => {
      reads += 1
      if (answer.hold) return held
      if (answer.thenReject !== undefined && reads > 1) {
        return Promise.reject(new Error(answer.thenReject))
      }
      if (answer.reject !== undefined) return Promise.reject(new Error(answer.reject))
      return Promise.resolve(answer.rows ?? [])
    },
  } as unknown as ServiceApi
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

const CONTROL: Control = { kind: 'checklist', id: 'probe', label: 'Readiness' }

/** Renders the checklist, and hands back the rerender so a test can move the
 *  SAME instance to a new refresh token. That matters: a second render is a
 *  second component with a second empty state, so the "a later read failed"
 *  case cannot be built by unmounting and drawing again — the rows the first
 *  read answered with would go with it, which is not the situation being
 *  tested. App.tsx bumps the token on one live instance every few seconds. */
function show(service: ServiceApi, items?: string[]) {
  const control: Control = { ...CONTROL, items }
  const drawn = <>{renderControl(control, context(service, 0))}</>
  const result = render(drawn)
  return {
    ...result,
    reread: (token: number) => result.rerender(<>{renderControl(control, context(service, token))}</>),
  }
}

/** One act() turn, enough for a useResource load to land. */
async function turn(): Promise<void> {
  const { act } = await import('@testing-library/react')
  await act(async () => {
    await Promise.resolve()
  })
}

const DECLARED = ['tap', 'hotkeys', 'windows-keys']
const ANSWERED: ReadinessRow[] = [
  { id: 'tap', label: 'Keyboard interception', ready: true, detail: '' },
  { id: 'hotkeys', label: 'Window keys', ready: false, detail: 'window-keys is switched off' },
]
/** The row the daemon serves that no page asked for: the core daemon itself. */
const EXTRA: ReadinessRow = { id: 'daemon', label: 'CrossOS daemon', ready: true, detail: '' }

const NOT_CHECKED = 'Not checked'
const NOT_REPORTED = /did not report on this one yet/

describe('the readiness checklist', () => {
  it('names a declared check the daemon answered for, and the ones after it', async () => {
    // The ordering, which is the reason this control exists at all: the page
    // says which checks it wanted, so those lead, whatever order the daemon
    // served them in.
    show(daemon({ rows: [EXTRA, ...ANSWERED] }), DECLARED)
    await turn()

    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items[0]).toContain('Keyboard interception')
    expect(items[1]).toContain('Window keys')
    // The row the daemon added beyond what the page asked for follows rather
    // than being dropped — a served check is a fact about the machine.
    expect(items[items.length - 1]).toContain('CrossOS daemon')
  })

  it('says what to do about a check that is not ready, in the daemon’s words', async () => {
    // The row's whole value: the fact that it is not ready is useless on its
    // own, and the fix is the daemon's to word. A shell that paraphrased it
    // would throw away the only part that names the thing to go and do.
    show(daemon({ rows: ANSWERED }), DECLARED)
    await turn()

    expect(screen.getByText(NOT_CHECKED)).toBeTruthy()
    expect(screen.getByText('window-keys is switched off')).toBeTruthy()
  })

  it('draws no verdict while the read is still in flight', async () => {
    // The first render of every load, on every poll. Before this was pinned the
    // page opened on three "Not checked" rows — each one a sentence about a
    // daemon that had not been asked anything yet.
    show(daemon({ hold: true }), DECLARED)
    await turn()

    expect(screen.queryByText(NOT_CHECKED)).toBeNull()
    expect(screen.queryByText(NOT_REPORTED)).toBeNull()
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    // And it says what is going on rather than leaving the gap silent, because
    // silence next to a list of verdicts reads as "those are all of them".
    expect(screen.getByText(/have not been looked at/)).toBeTruthy()
  })

  it('draws no verdict when a later read fails, and keeps the rows the daemon did answer', async () => {
    // The failure this file exists for, in the only shape in which rows and an
    // error are both on screen: the first read landed, then the daemon went
    // away. The rows that ARE facts stay — the person granted permissions and
    // the page must not throw that away — but the check the daemon never got
    // to is NOT re-drawn as "Not checked", because nobody looked at it. It was
    // looked at, on the read before, and the answer was that it is not ready.
    // One daemon and ONE live control across both reads, so the second one is
    // genuinely the second and the rows the first answered with are still here.
    const service = daemon({ rows: ANSWERED, thenReject: 'the tap is not answering' })
    const view = show(service, DECLARED)
    await turn()
    view.reread(1)
    await turn()

    // The rows that ARE facts are still here, in the page's order.
    expect(screen.getByText('Keyboard interception')).toBeTruthy()
    expect(screen.getByText('Window keys')).toBeTruthy()

    // The check the daemon never reported on is not a finding.
    expect(screen.queryByText(NOT_CHECKED)).toBeNull()
    expect(screen.queryByText(NOT_REPORTED)).toBeNull()

    // The gap is said out loud, and the daemon's own words are the error row.
    expect(screen.getByText(/have not been looked at/)).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toContain('the tap is not answering')
  })

  it('does not blame the machine for a check the daemon never reached', async () => {
    // The difference the "Not checked" row would have collapsed. The sentence
    // names this answer, not this machine — a person who granted everything and
    // came back to a daemon that is down must not be told to go and grant it
    // again. Nothing has been looked at here, so there is nothing to show.
    show(daemon({ reject: 'the tap is not answering' }), DECLARED)
    await turn()

    expect(screen.queryByText(NOT_CHECKED)).toBeNull()
    expect(screen.queryByRole('listitem')).toBeNull()
    const gap = screen.getByText(/have not been looked at/).textContent ?? ''
    expect(gap).toMatch(/gap in this answer, not a finding about this machine/)
  })

  it('still names a declared check the daemon answered for and left out', async () => {
    // The case "Not checked" was written for, and the one the change above must
    // not have taken away: the read LANDED, the daemon went through its list,
    // and this id was not in it. That is a fact about the daemon, so it is
    // drawn as one.
    show(daemon({ rows: ANSWERED }), DECLARED)
    await turn()

    expect(screen.getByText(NOT_CHECKED)).toBeTruthy()
    expect(screen.getByText(NOT_REPORTED)).toBeTruthy()
    // The one that was asked for and never named is named in words, not
    // dropped: a check that quietly vanishes reads as a check that passed.
    expect(screen.getByText('Windows keys')).toBeTruthy()
  })

  it('tells an empty answer apart from a failed one', async () => {
    // "The daemon has nothing" and "the daemon is gone" must never look alike.
    // Both say something — a blank section is its own failure — but they are
    // not allowed to say the SAME thing.
    const empty = show(daemon({ rows: [] }))
    await turn()
    expect(empty.container.textContent).toContain('The daemon reported no readiness checks.')
    expect(empty.container.querySelector('.ctl-error')).toBeNull()
    empty.unmount()

    const failed = show(daemon({ reject: 'the daemon is not answering' }))
    await turn()
    expect(failed.container.textContent).not.toContain('The daemon reported no readiness checks.')
    expect(failed.container.querySelector('.ctl-error')?.textContent).toContain(
      'the daemon is not answering',
    )
    failed.unmount()
  })

  it('draws the daemon’s own extra rows after the ones the page asked for', async () => {
    // The served list is always longer than the declared one, and a page that
    // asked for three does not get to hide the rest.
    show(daemon({ rows: [...ANSWERED, EXTRA] }), DECLARED)
    await turn()

    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items[items.length - 1]).toContain('CrossOS daemon')
    expect(items[items.length - 1]).toContain('Ready')
  })
})
