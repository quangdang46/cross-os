// First run, end to end, through the shell and nothing else.
//
// ONE TEST DRIVES THE WHOLE PRODUCT. Not a render of one control, and not a
// story about several: the window is mounted, the pages are reached by clicking
// the nav the host served, and every step is a bound call the shell actually
// makes. The first run is a SEQUENCE — land on the wizard, apply a profile,
// walk the permission step, verify, finish, and the next load lands somewhere
// else — and a suite of per-control tests cannot say whether the sequence
// holds. That is what this file is for.
//
// The stub stands in for the daemon (see ../test/fixtures for why it is
// stateful and where the sorting rule comes from). Everything the shell is
// being asked to do here is real work: render the served order, pick the landing
// page, dispatch an action through the registry, re-read a source on the
// refresh token, report a failure in words and keep the page usable.
//
// The permission step is where this test is least forgiving, and it used to be
// unforgiving in the worst way: permissions.openSettings had no bound call at
// all, so the button could only apologise, and a wizard that ran to the end with
// that button never once having opened the pane was a green test. It is a real
// write now, and the assertion below is the button DISPATCHING it — the person
// clicking Open Settings and the daemon recording that the pane was asked for
// is the only version of this step worth passing.
//
// Two other things this test now pins that it could not before. The Windows
// profile is picked from INSIDE the flow, on the step that declares the profile
// cards as its body, rather than by walking to a second page — which is the
// step the shell used to flatten into a label. And finishing records a write:
// the flag that ends onboarding is latched by CompleteOnboarding and nothing
// else, so a wizard that merely re-read readiness could never have ended.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { failOn, machine, press, reset } from '../test/fixtures'

// Replaces lib/service, the shell's only module that names the generated
// bindings. Loaded through the factory rather than a top-level binding so the
// hoisted mock does not capture a variable that is not initialised yet.
vi.mock('../lib/service', async () => {
  const fixtures = await import('../test/fixtures')
  return { Service: fixtures.stub, service: fixtures.stub }
})

/** Flushes the microtask queue React needs before an async render settles. */
async function settle(): Promise<void> {
  for (let turn = 0; turn < 8; turn += 1) {
    await act(async () => {
      await Promise.resolve()
    })
  }
}

/** Mounts the shell and waits for its first poll to land. */
async function open(): Promise<void> {
  const { default: App } = await import('../App')
  render(<App />)
  await settle()
}

/** Clicks a nav row by the title the host served for it. */
async function goTo(title: string): Promise<void> {
  const row = screen.getAllByRole('button').find((button) => {
    const label = button.querySelector('.section-label')
    return label?.textContent === title
  })
  expect(row, `the nav serves a ${title} page`).toBeTruthy()
  fireEvent.click(row as HTMLButtonElement)
  await settle()
}

/** The control section whose accessible name is `name`. */
function pane(name: string): HTMLElement {
  return screen.getByRole('region', { name })
}

/** Presses a chord on a recorder, in the spelling the daemon stores. */
function capture(button: HTMLButtonElement, key: string): void {
  fireEvent.click(button)
  fireEvent.keyDown(button, { key, code: key === 'Tab' ? 'Tab' : `Key${key.toUpperCase()}`, ctrlKey: true })
}

beforeEach(() => reset())
afterEach(() => cleanup())

describe('a fresh machine, from the first launch to the last decision', () => {
  it('walks setup, remaps a chord, and reads back the rule that won', async () => {
    // --- 1. a fresh profile lands on the wizard -----------------------------
    await open()

    expect(screen.getByRole('heading', { name: 'Welcome', level: 2 })).toBeTruthy()
    const active = screen.getAllByRole('button').filter((b) => b.getAttribute('aria-current') === 'page')
    expect(active.map((b) => b.querySelector('.section-label')?.textContent)).toEqual(['Welcome'])

    // Nothing is ready and the wizard says which permission buys each row,
    // rather than showing a stepper with no verdict beside it.
    expect(await screen.findByText(/0 checks of 4 ready/)).toBeTruthy()
    const checklist = pane('Readiness')
    expect(within(checklist).getByText('Keyboard interception')).toBeTruthy()
    expect(within(checklist).getAllByText('Not ready')).toHaveLength(4)
    expect(within(checklist).getAllByText('grant Accessibility in System Settings')).toHaveLength(4)

    // --- 2. pick the Windows 11 profile, from inside the flow --------------
    // The step that declares the profile cards as its own body, so the product
    // is chosen in the middle of setting it up rather than on a second page.
    // No nav click between here and the write: that is the point of the step.
    const firstRun = pane('Welcome to CrossOS')
    fireEvent.click(within(firstRun).getByRole('button', { name: /Pick a Windows profile/ }))
    fireEvent.click(await within(firstRun).findByRole('button', { name: 'Apply Windows 11' }))
    await waitFor(() => expect(machine.calls).toContain('ApplyProfile'))

    // The rollup on the card is the answer to "did the click land", and the
    // capability that ships no rules reads 0 of 0 rather than borrowing a count.
    expect(await within(firstRun).findByText('All 3 shortcuts on')).toBeTruthy()
    expect(within(firstRun).getByText(/no rule or profile capability covers it yet/)).toBeTruthy()

    // --- 3. the System Settings step opens the pane, in one write ----------
    await goTo('Welcome')
    const wizard = pane('Welcome to CrossOS')
    // A profile has been applied, so the daemon has derived the first THREE
    // steps done — welcome, the pick itself, and the per-extension step the pick
    // performed — and moved its own cursor on to the permission. The reader's
    // cursor is still where they left it: two cursors, two facts, both on screen.
    expect(within(wizard).getAllByText('Done')).toHaveLength(3)
    const waiting = within(wizard)
      .getAllByRole('listitem')
      .find((li) => li.textContent?.includes('Next up'))
    expect(waiting?.textContent).toContain('Open System Settings')
    fireEvent.click(within(wizard).getByRole('button', { name: /Open System Settings/ }))
    fireEvent.click(within(wizard).getByRole('button', { name: 'Open Settings' }))

    // The click ARRIVES. A button that only refused could not make this call,
    // and a first run that finished without the pane ever being asked for is the
    // failure this assertion was inverted to catch.
    await waitFor(() => expect(machine.calls).toContain('OpenSystemSettings'))
    expect(await within(wizard).findByText('Open Settings: done.')).toBeTruthy()

    // The rest of the wizard is still there and still working. The labels are
    // the daemon's own tokens, so a page that declared a different one draws a
    // different button — which is the point.
    expect(within(wizard).getByRole('button', { name: 'verify' })).toBeTruthy()
    expect(within(wizard).getByRole('button', { name: 'Next' })).toBeTruthy()

    // The row-scoped write refuses the same way: a wizard has no row, so it
    // must not send an empty plugin id and ask about a plugin that cannot exist.
    fireEvent.click(within(wizard).getByRole('button', { name: 'enable' }))
    expect(await within(wizard).findByText(/the control did not name it/)).toBeTruthy()

    // Verifying before the permission is granted is the honest case: the write
    // is a RE-READ, so it reports what the daemon can see now rather than
    // telling the person they did it.
    fireEvent.click(within(wizard).getByRole('button', { name: 'verify' }))
    expect(await within(wizard).findByText(/0 checks of 4 ready/)).toBeTruthy()

    // --- 4. the person grants the permission, and the tap comes back --------
    await goTo('Safety')
    fireEvent.click(await screen.findByRole('button', { name: 'Re-enable interception' }))
    await waitFor(() => expect(machine.calls).toContain('Resume'))

    // --- 5. readiness is green, and the flow can finish ---------------------
    await goTo('Welcome')
    const finished = pane('Welcome to CrossOS')
    expect(await within(finished).findByText('Every check is ready.')).toBeTruthy()
    expect(within(pane('Readiness')).getAllByText('Ready')).toHaveLength(4)

    fireEvent.click(within(finished).getByRole('button', { name: 'Finish setup' }))
    await waitFor(() => expect(machine.onboarded).toBe(true))

    // --- 6. the next load lands on Home -------------------------------------
    cleanup()
    await open()
    expect(screen.getByRole('heading', { name: 'Home', level: 2 })).toBeTruthy()
    const landing = screen.getAllByRole('button').filter((b) => b.getAttribute('aria-current') === 'page')
    expect(landing.map((b) => b.querySelector('.section-label')?.textContent)).toEqual(['Home'])
    // The card answers for the machine rather than for the wizard that is done.
    expect(await screen.findByText('Windows 11')).toBeTruthy()
    expect(screen.getByText(/Every installed extension is switched on/)).toBeTruthy()

    // --- 7. remap a chord through the keymap editor ------------------------
    await goTo('Shortcuts')
    const editor = pane('Record a shortcut')
    expect(await within(editor).findAllByRole('listitem')).toHaveLength(3)

    // The search really narrows: the list the daemon serves, filtered.
    fireEvent.change(within(editor).getByLabelText('Search shortcuts'), { target: { value: 'rename' } })
    await waitFor(() => expect(within(editor).getAllByRole('listitem')).toHaveLength(1))
    fireEvent.change(within(editor).getByLabelText('Search shortcuts'), { target: { value: '' } })

    capture(within(editor).getByRole('button', { name: 'Record a shortcut' }), 'Tab')
    // The chip beside the recorder is the capture; the list below separately
    // shows the rule the daemon already had for that chord. Reading the one
    // that is this control's result is the point.
    expect(editor.querySelector('.ctl-recorder')?.textContent).toContain('ctrl+Tab')

    fireEvent.change(within(editor).getByLabelText('Action'), { target: { value: 'window.switch' } })
    fireEvent.click(within(editor).getByRole('button', { name: 'Save rule' }))
    expect(await within(editor).findByText(/Saved the rule for ctrl\+Tab/)).toBeTruthy()
    expect(machine.rules).toHaveLength(1)
    expect(machine.rules[0].chord).toBe('Ctrl+Tab')

    // --- 8. build the context rule that beats it ----------------------------
    const builder = pane('Rule builder')
    fireEvent.change(within(builder).getByLabelText('IF the front app is'), {
      target: { value: 'com.apple.finder' },
    })
    capture(within(builder).getByRole('button', { name: 'AND press the shortcut' }), 'Tab')
    fireEvent.change(within(builder).getByLabelText('THEN'), { target: { value: 'finder.rename' } })
    // The sentence is drawn as the picks are made — a builder, not a form. The
    // middle clause stays open until the write comes back, because `chord` is
    // the daemon's derivation and this control does not compose it.
    expect(within(builder).getByText(/^IF App = Finder AND .+ THEN finder\.rename$/)).toBeTruthy()

    fireEvent.click(within(builder).getByRole('button', { name: 'Save rule' }))
    expect(await within(builder).findByText(/Saved rule user\.2/)).toBeTruthy()
    expect(machine.rules).toHaveLength(2)
    // The daemon came back with the derivation filled in, and the stored row is
    // drawn from that answer rather than from what the editor sent.
    const stored = machine.rules[1]
    expect(stored.chord).toBe('Ctrl+Tab')
    expect(stored.scope).toBe('app:com.apple.finder')
    // Two rules now answer to one chord, and the list keeps them apart by id
    // rather than collapsing them — which is what lets the conflict resolver
    // offer one of them to be switched off.
    expect(within(builder).getAllByText('ctrl+Tab')).toHaveLength(2)
    expect(within(builder).getByRole('button', { name: 'Remove user.2' })).toBeTruthy()

    // --- 9. press the key, and read the decision ---------------------------
    // The router decides on a kernel tap event, and no bound call can
    // synthesise one — the fixture supplies that one event and nothing else.
    press('Ctrl+Tab')
    await goTo('Activity')

    const trace = pane('trace')
    expect(await within(trace).findByText(/Ctrl\+Tab → finder\.rename/)).toBeTruthy()
    expect(within(trace).getByText(/won by user\.2/)).toBeTruthy()
    expect(within(trace).getByText(/lost to it: user\.1, win\.switch/)).toBeTruthy()

    const stages = within(trace).getAllByRole('listitem').flatMap((li) =>
      Array.from(li.querySelectorAll('.ctl-stage')),
    )
    expect(stages).toHaveLength(5)
    expect(stages.map((stage) => stage.querySelector('.ctl-stage-name')?.textContent)).toEqual([
      'event',
      'context',
      'rule',
      'intent',
      'action',
    ])

    // --- 10. and the contest is a state, not a save-time error --------------
    await goTo('Shortcuts')
    const conflicts = pane('Conflicts')
    expect(await within(conflicts).findByText(/won by user\.2/)).toBeTruthy()
    expect(
      within(conflicts).getByRole('button', { name: 'Turn off user.1 so Ctrl+Tab resolves to user.2' }),
    ).toBeTruthy()
  })
})

// --- what a broken machine must look like ------------------------------------
//
// The rule the whole file is for: a source that fails shows WHY, in place, and
// the rest of the page keeps working. A settings window that goes blank — or
// worse, that keeps showing yesterday's value as if it were current — has told
// the person nothing they can act on.

describe('when a source fails', () => {
  it('names the failure, keeps the page usable, and drops the stale value', async () => {
    // The wizard reads the onboarding row (which carries the readiness rows on
    // it), the checklist reads readiness directly, and the window's own status
    // comes from a third source. Each control depends on ONE of them, so
    // breaking the two the first-run page draws must cost exactly those two and
    // leave the rest of the window standing.
    failOn('OnboardingState', 'readiness source unavailable')
    failOn('Readiness', 'readiness source unavailable')
    await open()

    const wizard = pane('Welcome to CrossOS')
    expect(await within(wizard).findByText(/readiness source unavailable/)).toBeTruthy()
    expect(within(pane('Readiness')).getByText(/readiness source unavailable/)).toBeTruthy()

    // The nav, the stepper and the landing page are untouched: a dead source
    // is one row on the page, never a blank window. A source that still
    // answers is still shown, which is the other half — one dead call must not
    // blank the ones that worked.
    expect(screen.getByRole('heading', { name: 'Welcome', level: 2 })).toBeTruthy()
    expect(screen.getByRole('navigation', { name: 'Settings pages' })).toBeTruthy()
    expect(within(wizard).getByRole('button', { name: 'verify' })).toBeTruthy()
    expect(screen.getByText('CrossOS 0.4.0')).toBeTruthy()

    // And the error is a sentence the daemon wrote, not a blank and not the
    // last good value: the stepper says it has nothing to verify rather than
    // reporting zero checks ready.
    expect(within(wizard).getByText(/no readiness checks, so this step has nothing to verify/)).toBeTruthy()
  })

  it('drops a value the daemon used to serve, rather than showing it as current', async () => {
    // The strongest form of "never a stale value": the machine had a profile
    // and a live tap, then the readiness source stopped answering. The card
    // must not keep reporting the checks it saw a moment ago.
    await open()
    await goTo('Profiles')
    fireEvent.click(await screen.findByRole('button', { name: 'Apply Windows 11' }))
    await waitFor(() => expect(machine.profile).toBe('windows11'))

    failOn('Readiness', 'readiness source unavailable')
    await goTo('Home')

    const card = pane('What is on')
    expect(await within(card).findByText(/readiness source unavailable/)).toBeTruthy()
    expect(within(card).queryByText(/of 4 ready/)).toBeNull()
    expect(
      within(card).getByText(/no readiness checks, so this card cannot say what is usable/),
    ).toBeTruthy()
    // The rows the card does still have keep their answers, because their
    // sources are the ones that are alive.
    expect(within(card).getByText('Windows 11')).toBeTruthy()
  })
})

describe('when nothing is served', () => {
  it('says what is missing instead of drawing a blank page', async () => {
    // A machine nobody has touched yet: no rule has been written and nothing
    // has been decided. Both lists are legitimately empty, and each one must
    // say so in words — a blank list and a broken list look identical to the
    // person reading them.
    await open()
    await goTo('Shortcuts')
    const builder = pane('Rule builder')
    expect(await within(builder).findByText(/No rules have been written/)).toBeTruthy()

    await goTo('Activity')
    expect(await screen.findByText(/Nothing has been decided yet/)).toBeTruthy()
    // And the shell's own timeline is NOT printed under it: a page that
    // declares a trace already has one log, and two copies that disagree about
    // how much history exists is the one thing a log view must never do.
    expect(screen.queryByText('No activity recorded yet.')).toBeNull()
  })

  it('refuses a write with nothing chosen, and says which half is missing', async () => {
    await open()
    await goTo('Shortcuts')
    const builder = pane('Rule builder')
    fireEvent.click(await within(builder).findByRole('button', { name: 'Save rule' }))
    expect(await within(builder).findByText('Press the shortcut this rule answers to.')).toBeTruthy()
    expect(machine.rules).toHaveLength(0)

    const editor = pane('Record a shortcut')
    fireEvent.click(within(editor).getByRole('button', { name: 'Save rule' }))
    expect(await within(editor).findByText(/Record the shortcut first/)).toBeTruthy()
  })
})
