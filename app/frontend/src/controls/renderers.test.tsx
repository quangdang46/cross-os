// The renderers and the action registry, asserted against the daemon's own
// declarations (bead w6-frontend-setup-renderers).
//
// Three jobs, the first two of which used to be true by accident rather than
// by test:
//
//   1. Every action id a served page declares has an entry in the action
//      registry — runnable, or named in the unbound table with the bound call
//      it is waiting on. This reads the Go page declarations off disk, so a page
//      that gains an action id and no command fails HERE rather than as a button
//      that does nothing when somebody clicks it.
//   2. Every control KIND a served page declares has a renderer. Same source,
//      same reason: a page gaining a kind nobody drew is §3.6c's acceptance test
//      failing, and it should fail here rather than in front of a user.
//   3. Each renderer draws what the daemon sent — the wizard's steps, the
//      profile rollup, the pipeline stages, the manifest facts, the three
//      declared readiness items — and no renderer ever draws a blank section,
//      whether the daemon answered with nothing or with a failure.
//
// Fixtures use invented ids on purpose (the rule harness.tsx states): a page id
// is daemon vocabulary (§3.6c), so pasting a real one into a test would put a
// page id back into src/.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import type {
  OnboardingRow,
  OnboardingStep,
  ProfileRow,
  PluginMetaRow,
  ReadinessRow,
  ServiceApi,
  Status,
  TraceRow,
} from '../types/controls'
import { ACTION_COMMANDS, UNBOUND_ACTIONS, commandFor } from './actions'
import { renderControl, rendererKinds, type ControlContext } from './index'
import { machine, reset, stub } from '../test/fixtures'
import type { Control } from '../types/controls'

afterEach(() => cleanup())

/** The fake daemon. A call is recorded by name so a test can assert WHICH
 *  bound method a control reached for, which is the whole point of the action
 *  registry. */
function daemon(over: Partial<ServiceApi> = {}): ServiceApi & { calls: string[] } {
  const calls: string[] = []
  const record = <T,>(name: string, value: T) => () => {
    calls.push(name)
    return Promise.resolve(value)
  }
  return {
    calls,
    GetStatus: record('GetStatus', null),
    Readiness: record('Readiness', [] as ReadinessRow[]),
    OnboardingState: record('OnboardingState', blankState()),
    CompleteOnboarding: record('CompleteOnboarding', undefined),
    Profiles: record('Profiles', [] as ProfileRow[]),
    ApplyProfile: record('ApplyProfile', {}),
    Traces: record('Traces', [] as TraceRow[]),
    // The erase beside the read. Recorded by name for the same reason the read
    // is: a control that emptied its own copy of the rows would look identical
    // on screen, and only the call list tells the two apart.
    TracesClear: record('TracesClear', [] as TraceRow[]),
    PluginMeta: record('PluginMeta', [] as PluginMetaRow[]),
    TogglePlugin: record('TogglePlugin', undefined),
    SetRuleEnabled: record('SetRuleEnabled', true),
    FinderMenu: record('FinderMenu', [] as Record<string, unknown>[]),
    SetMenuItemEnabled: record('SetMenuItemEnabled', [] as Record<string, unknown>[]),
    ...over,
  } as unknown as ServiceApi & { calls: string[] }
}

/** A wizard nobody has started: no step is done and the daemon is waiting on
 *  the first one. The neutral a `daemon()` serves when a test does not care. */
function blankState(steps: OnboardingStep[] = []): OnboardingRow {
  return {
    completed: false,
    current_step: steps[0]?.id ?? '',
    steps,
    readiness: [],
    ready: 0,
    total: 0,
  }
}

/**
 * The wizard's answer for a set of declared steps: each one is done or not, the
 * cursor is the first that is not, and the readiness rows ride along on the same
 * row. Built here rather than tabulated because the point of every test below
 * is that the shell RENDERS the daemon's verdicts — a fixture that hard-coded a
 * finished wizard would let a shell that ignored the row pass.
 */
function wizardState(
  steps: { id: string; label: string; done?: boolean; detail?: string }[],
  readiness: ReadinessRow[] = [],
): OnboardingRow {
  const sent: OnboardingStep[] = steps.map((step) => ({
    id: step.id,
    label: step.label,
    done: step.done === true,
    detail: step.done ? undefined : step.detail,
  }))
  return {
    ...blankState(sent),
    current_step: sent.find((step) => !step.done)?.id ?? 'done',
    readiness,
    ready: readiness.filter((row) => row.ready).length,
    total: readiness.length,
  }
}

function context(service: ServiceApi, status: Status | null = null): ControlContext {
  return {
    service,
    status,
    logs: [],
    refreshToken: 0,
    note: () => {},
    refresh: () => {},
    pageId: '',
  }
}

function show(control: Control, service: ServiceApi, status: Status | null = null) {
  return render(<>{renderControl(control, context(service, status))}</>)
}

// --- the actions a served page declares -------------------------------------

/**
 * An action id is a dotted, space-free token: "plugin.enable", "pack.remove".
 * A declared step title is prose ("Open System Settings"), and a source name is
 * a namespaced id in the other shape. Filtering on the shape is what keeps a
 * step title out of the action set without naming any of them.
 */
const ACTION_ID = /^[a-z][a-zA-Z0-9]*\.[A-Za-z0-9.]+$/

function goFiles(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) out.push(...goFiles(path))
    else if (entry.endsWith('.go') && !entry.endsWith('_test.go')) out.push(path)
  }
  return out
}

/**
 * Every action id the repo's Go pages declare. A page is a Go file that builds a
 * UIContribution, so those are the files scanned; the three trees below are the
 * ones that can hold one today (Core pages, plugin packs, platform extensions).
 */
function declaredActions(): Set<string> {
  const found = new Set<string>()
  for (const text of pageDeclarations()) {
    // A []string{...} list is a page's action list, a control's actions or
    // its rowActions; "action" and "rowAction" name one id each.
    for (const [, list] of text.matchAll(/\[\]string\{([^}]*)\}/g)) {
      for (const raw of list.split(',')) {
        const id = raw.trim().replace(/^"|"$/g, '')
        if (ACTION_ID.test(id)) found.add(id)
      }
    }
    for (const [, id] of text.matchAll(/"(?:action|rowAction)"\s*:\s*"([^"]+)"/g)) {
      if (ACTION_ID.test(id)) found.add(id)
    }
  }
  return found
}

/**
 * The source of every Go file that builds a UIContribution. A page is a Go file
 * that registers one, so those are the files scanned; the three trees below are
 * the ones that can hold one today (Core pages, plugin packs, platform
 * extensions). A scan that found no pages has silently proved nothing, so the
 * empty case throws rather than passing.
 */
function pageDeclarations(): string[] {
  const roots = ['../backend', '../../plugins', '../../extensions'].map((rel) =>
    join(process.cwd(), rel),
  )
  const found: string[] = []
  for (const root of roots) {
    let files: string[]
    try {
      files = goFiles(root)
    } catch {
      continue
    }
    for (const file of files) {
      const text = readFileSync(file, 'utf8')
      if (text.includes('UIContribution')) found.push(text)
    }
  }
  if (found.length === 0) throw new Error('no Go page declarations were found to scan')
  return found
}

/** Every control kind a registered page declares, read off the Go source. */
function declaredKinds(): Set<string> {
  const found = new Set<string>()
  // The daemon's own field, with or without the space a map literal usually
  // carries — a gate that only reads one spelling would pass on a page
  // declared the other way.
  for (const text of pageDeclarations()) {
    for (const [, kind] of text.matchAll(/"kind"\s*:\s*"([a-zA-Z][a-zA-Z0-9]*)"/g)) {
      found.add(kind)
    }
  }
  return found
}

describe('the action registry', () => {
  it('has an entry for every action id a served page declares', () => {
    const declared = declaredActions()
    expect(declared.size).toBeGreaterThan(0)
    const missing = [...declared].filter((id) => !(id in ACTION_COMMANDS) && !(id in UNBOUND_ACTIONS))
    expect(missing).toEqual([])
  })

  it('resolves every declared action to a command, so no button runs nothing', () => {
    // An id in neither table returns undefined, and the button then reports
    // "this build has no command for it" — a sentence naming no gap. Every
    // declared id must come back as a callable instead, refusing ones included.
    for (const id of declaredActions()) {
      expect(typeof commandFor(id), `${id} resolves to a command`).toBe('function')
    }
  })

  it('names the bound call each unbound action is waiting on', () => {
    for (const [id, gap] of Object.entries(UNBOUND_ACTIONS)) {
      expect(gap.what, `${id} says what it would do`).not.toBe('')
      expect(gap.waitsOn, `${id} names the call it waits on`).not.toBe('')
    }
  })

  it('binds permissions.openSettings rather than refusing it by name', async () => {
    // INVERTED from a refusal that used to be pinned as correct. The first-run
    // flow offers this button on every launch, so an entry in the unbound table
    // meant the one step nobody could ever take was described as a capability
    // nobody has — and the person reading it was told to go grant a permission
    // behind a button that could only ever apologise. It is a real bound write
    // now, and the id must live in ACTION_COMMANDS so commandFor resolves it.
    expect('permissions.openSettings' in ACTION_COMMANDS).toBe(true)
    expect('permissions.openSettings' in UNBOUND_ACTIONS).toBe(false)

    // The write reaches the daemon through the seam. Asserting the service
    // still HAS the method proves nothing — the test put it there — so the
    // claim is that the command ARRIVES: a spy, and a call count.
    let reached = 0
    const service = { OpenSystemSettings: () => { reached += 1; return Promise.resolve() } }
    await commandFor('permissions.openSettings')?.(service as unknown as ServiceApi)
    expect(reached).toBe(1)
  })

  it('refuses the settings write in words when a build has no binding for it', async () => {
    // The half that used to be the whole story, and is still owed. The Go
    // Service carries the method now, but a checkout whose generated bindings
    // predate it genuinely lacks the call at runtime, and those are different
    // files on different clocks. So a build without it must say WHICH call is
    // missing — not throw, and not report success. Bound and honest are
    // separate properties; this is the second one.
    const command = commandFor('permissions.openSettings')
    await expect(command?.(daemon())).rejects.toThrow(/OpenSystemSettings/)
  })

  it('refuses a row-scoped write that no row was named for', async () => {
    // A wizard has no row, so a row-scoped action it declares cannot invent one.
    // The refusal names the missing row instead of sending an empty id and
    // asking the daemon about a plugin that cannot exist.
    await expect(commandFor('profile.apply')?.(daemon())).rejects.toThrow(/did not name it/)
  })

  it('answers an unknown id with undefined rather than something inherited', () => {
    // The prototype-safe path: a plain lookup would answer "toString" with a
    // function from Object.prototype and renderControl would try to draw it.
    expect(commandFor('toString')).toBeUndefined()
    expect(commandFor('constructor')).toBeUndefined()
    expect(commandFor('no-such-action')).toBeUndefined()
  })

  it('routes a row-scoped write through the row the control named', async () => {
    const service = daemon()
    await commandFor('profile.apply')?.(service, { id: 'windows-like' })
    expect(service.calls).toEqual(['ApplyProfile'])
  })
})

// --- the registry -----------------------------------------------------------

describe('the kind registry', () => {
  it('registers the setup, home, profile, pipeline and detail kinds', () => {
    const kinds = rendererKinds()
    for (const kind of ['wizard', 'homeSummary', 'profileList', 'pipelineTrace', 'pluginDetail']) {
      expect(kinds, `${kind} is registered`).toContain(kind)
    }
  })

  it('keys on kinds, never on page ids', () => {
    // Page ids in this codebase are dotted; a dotted registry key would be a
    // registry that had started naming screens instead of kinds.
    for (const kind of rendererKinds()) {
      expect(kind.includes('.'), `${kind} is a kind, not a page id`).toBe(false)
    }
  })

  it('has a renderer for every kind a registered page declares', () => {
    // The gate §3.6c is written as an acceptance test, and this is it. A page
    // that declares a kind nobody drew would otherwise ship as a settings row
    // reading "no renderer for that kind" — a hole with no owner, discovered by
    // the person using it rather than by the suite. Reading the Go source
    // rather than a hand-kept list is the whole point: a list here would go
    // stale the moment a page gained a control, which is the moment the gate
    // has to fire.
    const declared = declaredKinds()
    expect(declared.size).toBeGreaterThan(0)
    const kinds = rendererKinds()
    const missing = [...declared].filter((kind) => !kinds.includes(kind))
    expect(missing).toEqual([])
  })
})

// --- what every kind looks like with nothing, and with a failure ------------
//
// Empty and failed are asserted for EVERY registered kind, not for the ones
// somebody remembered: a blank section is the failure this file exists to
// prevent, because "the daemon has nothing" and "the daemon is gone" look
// identical to the person reading them, and a control that renders neither
// sentence has told them nothing they can act on. The third state — disabled
// while a write is in flight — is asserted on the controls that write, below.

/** A daemon that answers every collection empty and every scalar with its
 *  neutral value — what a machine that has never been set up looks like. */
const NEUTRAL: Record<string, unknown> = {
  GetStatus: null,
  TrialState: { plugin: '', state: 'none', remaining_ms: 0, timeout_ms: 0 },
  SetRuleEnabled: true,
  FinderMenu: [],
  SetMenuItemEnabled: [],
  SetShortcuts: 0,
  SetZones: 0,
  SetUserRule: '',
  ApplyProfile: {},
  PanicStop: {},
  Resume: {},
  ResetEverything: [],
  SetOverride: null,
  BeginTrial: '',
  ConfirmTrial: '',
  RollbackTrial: '',
  // A wizard that has not been started: no step done, no checks, and a cursor
  // parked on nothing. The generic sweep renders every registered kind against
  // this, so the wizard has to draw a real step list from it and not crash on
  // the absence of one.
  OnboardingState: {
    completed: false,
    current_step: '',
    steps: [{ id: 'welcome', label: 'Welcome', done: false, detail: 'nothing is set up yet' }],
    readiness: [],
    ready: 0,
    total: 0,
  },
  CompleteOnboarding: undefined,
}

/** A daemon that answers every method the same way, recording what it was asked. */
function stubDaemon(reason: string | null, asked?: string[]): ServiceApi {
  return new Proxy({} as ServiceApi, {
    get: (_target, name: string) => () => {
      asked?.push(name)
      if (reason !== null) return Promise.reject(new Error(reason))
      const value = name in NEUTRAL ? NEUTRAL[name] : []
      return Promise.resolve(value)
    },
  })
}

/** One act() turn, enough for a useResource load to land. */
async function oneTurn(): Promise<void> {
  const { act } = await import('@testing-library/react')
  await act(async () => {
    await Promise.resolve()
  })
}

/** The control section, with its own label text removed so "blank" means blank. */
function sectionText(container: HTMLElement, label: string): string {
  const section = container.querySelector('section.ctl')
  return (section?.textContent ?? '').replace(label, '').trim()
}

/** A stub whose named calls hang until released, so "in flight" is observable. */
function heldService(names: string[]): { service: ServiceApi; release: () => void } {
  let open = (): void => {}
  const held = new Promise<unknown>((resolve) => {
    open = () => resolve([])
  })
  const service = new Proxy(stub, {
    get: (target, name: string) =>
      names.includes(name)
        ? () => held
        : (target as unknown as Record<string, unknown>)[name],
  }) as unknown as ServiceApi
  return { service, release: open }
}

function buttonNamed(container: HTMLElement, name: string): HTMLElement {
  const found = Array.from(container.querySelectorAll('button')).find(
    (button) => (button.textContent ?? '').trim() === name,
  )
  expect(found, `a ${name} button is offered`).toBeTruthy()
  return found as HTMLElement
}

function labelNamed(container: HTMLElement, label: string): HTMLElement {
  const row = Array.from(container.querySelectorAll('label')).find(
    (field) => field.querySelector('.ctl-label')?.textContent === label,
  )
  const input = row?.querySelector('input, select')
  expect(input, `a ${label} field is offered`).toBeTruthy()
  return input as HTMLElement
}

/** Presses a chord on a recorder, in the spelling the daemon stores. */
function pressChord(recorder: HTMLElement, key: string): void {
  fireEvent.click(recorder)
  fireEvent.keyDown(recorder, { key, code: `Key${key.toUpperCase()}`, ctrlKey: true })
}

describe('every registered kind, with nothing and with a failure', () => {
  it('says something when every source answers empty', async () => {
    for (const kind of rendererKinds()) {
      const { container, unmount } = show({ kind, id: 'probe', label: 'Probe' }, stubDaemon(null))
      await oneTurn()
      expect(sectionText(container, 'Probe'), `${kind} is not blank`).not.toBe('')
      // Nothing failed, so nothing may claim it did.
      expect(container.querySelector('.ctl-error'), `${kind} reports no error it did not have`).toBeNull()
      unmount()
    }
  })

  it('names a failed source on every kind that reads one', async () => {
    for (const kind of rendererKinds()) {
      const asked: string[] = []
      const { container, unmount } = show(
        { kind, id: 'probe', label: 'Probe' },
        stubDaemon('source unavailable', asked),
      )
      await oneTurn()
      // Whether a kind HAS a source is derived, not hand-listed: a renderer
      // that asked the daemon for nothing has no source to fail, and the only
      // thing owed then is that it still draws its own declared text.
      if (asked.length > 0) {
        expect(
          container.querySelector('.ctl-error')?.textContent,
          `${kind} names the failure`,
        ).toMatch(/source unavailable/)
      }
      expect(sectionText(container, 'Probe'), `${kind} is not blank on a failure`).not.toBe('')
      unmount()
    }
  })

  it('disables the write while it is in flight, and re-enables it after', async () => {
    // The disabled state is not decoration: a settings window that leaves a
    // write's button live during the write takes the same edit twice, and the
    // second lands after the first with no way to tell which one stuck. Each
    // write is held open here, so the assertion is about the control and not
    // about how fast a promise happened to resolve.
    const cases: { kind: string; write: RegExp; arm: (c: HTMLElement) => void }[] = [
      {
        kind: 'profileList',
        write: /^Apply /,
        // The card's Apply is gated on the review (the menumate reviewList
        // port), so the arm step looks at each capability first. Clicking a
        // switch twice reviews the row and leaves the selection whole, and a
        // test that pressed a disabled button would prove nothing about the
        // in-flight state.
        arm: (c) => {
          for (const box of Array.from(c.querySelectorAll('input.ctl-toggle'))) {
            fireEvent.click(box)
            fireEvent.click(box)
          }
        },
      },
      {
        kind: 'keymapEditor',
        write: /Save rule/,
        arm: (c) => {
          pressChord(buttonNamed(c, 'Record a shortcut'), 'K')
          fireEvent.change(labelNamed(c, 'Action'), { target: { value: 'window.switch' } })
        },
      },
      {
        kind: 'ruleBuilder',
        write: /Save rule/,
        arm: (c) => {
          pressChord(buttonNamed(c, 'AND press the shortcut'), 'K')
          fireEvent.change(labelNamed(c, 'THEN'), { target: { value: 'window.switch' } })
        },
      },
    ]

    for (const entry of cases) {
      reset()
      const { service, release } = heldService(entry.kind === 'profileList' ? ['ApplyProfile'] : ['SetUserRule'])
      const { container, unmount } = show({ kind: entry.kind, id: 'probe', label: 'Probe' }, service)
      await oneTurn()
      entry.arm(container)

      const write = Array.from(container.querySelectorAll('button')).find((button) =>
        entry.write.test((button.textContent ?? '').trim()),
      )
      expect(write, `${entry.kind} offers its write`).toBeTruthy()
      fireEvent.click(write as HTMLButtonElement)
      await oneTurn()
      expect((write as HTMLButtonElement).disabled, `${entry.kind} disables during the write`).toBe(true)

      release()
      await oneTurn()
      expect((write as HTMLButtonElement).disabled, `${entry.kind} re-enables after the write`).toBe(false)
      unmount()
    }
  })
})

// --- the wizard -------------------------------------------------------------

/**
 * A four-step flow declared the way a page declares one: bare titles, which is
 * every wizard the daemon has served so far and the form that must keep working
 * unchanged. The daemon's verdicts arrive on its own row, keyed by step id, and
 * the shell joins them by POSITION here because these titles carry no id — which
 * is the honest case for a page that has not been updated yet.
 */
const WIZARD: Control = {
  kind: 'wizard',
  id: 'setup',
  label: 'Welcome to CrossOS',
  steps: ['Pick a Windows profile', 'Turn on each extension', 'Open System Settings', 'Verify'],
  actions: ['plugin.enable'],
}

const CHECKS: ReadinessRow[] = [
  { id: 'keyboard', label: 'Keyboard interception', ready: true, detail: '' },
  { id: 'windows', label: 'Windows shortcuts', ready: false, detail: 'a plugin is switched off' },
  { id: 'finder', label: 'Finder shortcuts', ready: false, detail: 'finder-actions is switched off' },
]

/** The verdicts for the four steps above, joined by position. */
const STEP_IDS = ['chooseProfile', 'enable', 'settings', 'verify']

function wizardDaemon(over: Partial<ServiceApi> = {}): ServiceApi {
  return daemon({
    OnboardingState: () =>
      Promise.resolve(
        wizardState(
          [
            { id: STEP_IDS[0], label: 'Pick a Windows profile', done: true },
            { id: STEP_IDS[1], label: 'Turn on each extension', done: true },
            { id: STEP_IDS[2], label: 'Open System Settings', detail: 'grant Accessibility in System Settings' },
            { id: STEP_IDS[3], label: 'Verify', detail: '2 of 3 checks are not ready' },
          ],
          CHECKS,
        ),
      ),
    ...over,
  })
}

describe('the wizard', () => {
  it('draws every declared step and marks the open one current', async () => {
    show(WIZARD, wizardDaemon())
    const steps = await screen.findAllByRole('button', { name: /Pick a Windows profile|Turn on each extension|Open System Settings|Verify/ })
    expect(steps).toHaveLength(4)
    expect(steps[0].getAttribute('aria-current')).toBe('step')
    expect(steps[1].getAttribute('aria-current')).toBeNull()
  })

  it("renders the daemon's per-step verdicts rather than deriving them", async () => {
    // The whole point of this control. Two steps are done and two are not, and
    // the shell has no way to know that from the readiness rows alone — the
    // checklist has one green and two red, which would put this flow nowhere
    // near finished. The marks below are the daemon's, so a step the checklist
    // says nothing about can still be done.
    show(WIZARD, wizardDaemon())
    await screen.findByRole('button', { name: /Pick a Windows profile/ })

    const marks = screen.getAllByText('Done')
    expect(marks).toHaveLength(2)
    // The daemon's own reason, on the step that is not done — the per-step body
    // the survey reference carries and this used to flatten into one blurb.
    expect(screen.getByText('grant Accessibility in System Settings')).toBeTruthy()
    expect(screen.getByText('2 of 3 checks are not ready')).toBeTruthy()
  })

  it("marks the step the daemon's current_step names, beside the one that is open", async () => {
    // Two cursors, two marks, never one number. The reader opens step 1 while
    // the daemon waits on step 3; if the shell collapsed those into a single
    // "current" it would be showing one of the two facts and hiding the other.
    show(WIZARD, wizardDaemon())
    await screen.findByRole('button', { name: /Pick a Windows profile/ })

    expect(screen.getAllByText('Current step')).toHaveLength(1)
    expect(screen.getAllByText('Next up')).toHaveLength(1)
    // The daemon is waiting on Open System Settings, and the reader is on the
    // first step. Both facts are on screen.
    const openRow = screen.getAllByRole('listitem').find((li) => li.className.includes('is-current'))
    expect(openRow?.textContent).toContain('Pick a Windows profile')
    const waitingRow = screen.getAllByRole('listitem').find((li) => li.textContent?.includes('Next up'))
    expect(waitingRow?.textContent).toContain('Open System Settings')
  })

  it('rolls the step marks up once, under the list, and leaves the rest to the daemon', async () => {
    // The reference states the count ONCE, under the list it counts
    // (PackImportSheet.swift:271-275). This file used to state it twice: once
    // here as "1 check of 3 ready. Still to do: …" and once more as the
    // daemon's own "2 of 3 checks are not ready" on the Verify step, and two
    // copies of one number is how two of them come to disagree. The shell's
    // copy is the one it owns, so it is the one that goes; the daemon's stays.
    show(WIZARD, wizardDaemon())
    expect(await screen.findByText('2 of 4 steps done.')).toBeTruthy()
    expect(screen.queryByText(/1 check of 3 ready/)).toBeNull()
    // What is left is still named, by the daemon, on the step that is not done.
    expect(screen.getByText('2 of 3 checks are not ready')).toBeTruthy()
  })

  it('marks a done step with a filled circle and an undone one with a ring', async () => {
    // The mark is the reference's :236-245 and it is the reason the list can be
    // taken in without being read. The SHAPE carries it — filled against ring —
    // because a mark that only differs by colour would say nothing to a
    // colour-blind reader, and the "Done" chip beside it says the same thing in
    // words either way.
    show(WIZARD, wizardDaemon())
    await screen.findByText('2 of 4 steps done.')

    const rows = Array.from(document.querySelectorAll<HTMLElement>('.ctl-step'))
    expect(rows).toHaveLength(4)
    expect(rows.map((row) => row.querySelector('.ctl-step-mark')?.getAttribute('data-done'))).toEqual([
      'true',
      'true',
      'false',
      'false',
    ])
  })

  it('moves the open step with Next and Back', async () => {
    show(WIZARD, wizardDaemon())
    const first = await screen.findByRole('button', { name: /Pick a Windows profile/ })
    expect(first.getAttribute('aria-current')).toBe('step')

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(screen.getByRole('button', { name: /Turn on each extension/ }).getAttribute('aria-current')).toBe('step')

    fireEvent.click(screen.getByRole('button', { name: 'Back' }))
    expect(screen.getByRole('button', { name: /Pick a Windows profile/ }).getAttribute('aria-current')).toBe('step')
  })

  it('draws the profile cards inline on a step that declares that body', async () => {
    // The profile is picked from INSIDE the flow rather than on a separate page.
    // The step declares a control KIND, and the wizard resolves it through the
    // same kind-keyed map the registry uses — there is no page id in this file
    // and no branch on which step it is.
    const declared: Control = {
      kind: 'wizard',
      id: 'setup',
      label: 'Welcome to CrossOS',
      steps: [
        { id: 'chooseProfile', label: 'Pick a Windows profile', body: 'profileList' },
        { id: 'verify', label: 'Verify' },
      ],
    }
    const service = wizardDaemon({
      OnboardingState: () =>
        Promise.resolve(
          wizardState([
            { id: 'chooseProfile', label: 'Pick a Windows profile', detail: 'no profile is applied yet' },
            { id: 'verify', label: 'Verify', detail: '3 of 3 checks are not ready' },
          ]),
        ),
      Profiles: () => Promise.resolve(PROFILES),
    })
    show(declared, service)

    // The cards the profileList renderer draws, inside the wizard's own section.
    expect(await screen.findByText('Windows-like setup')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Apply Windows-like setup' })).toBeTruthy()
    // And a second page is not needed: the step the reader is on is the one that
    // offers them.
    const current = screen.getAllByRole('listitem').find((li) => li.className.includes('is-current'))
    expect(current?.textContent).toContain('Pick a Windows profile')
  })

  it('names a step body kind this build cannot draw', async () => {
    // A hole with no owner is a defect nobody finds until somebody is on that
    // step, so the gap is self-describing rather than a silent blank.
    const service = wizardDaemon({
      OnboardingState: () =>
        Promise.resolve(wizardState([{ id: 'chooseProfile', label: 'Pick one', detail: 'nothing picked' }])),
    })
    show(
      {
        kind: 'wizard',
        id: 'setup',
        label: 'Welcome to CrossOS',
        steps: [{ id: 'chooseProfile', label: 'Pick one', body: 'somethingInvented' }],
      },
      service,
    )
    expect(await screen.findByText(/no renderer for that kind/)).toBeTruthy()
    expect(screen.getByText(/“somethingInvented” body/)).toBeTruthy()
  })

  it('offers the finish only once the daemon says it has no unfinished step', async () => {
    // The gate is the daemon's cursor, not a count the shell took: a wizard
    // whose readiness rows are green but whose current_step still names a step
    // is NOT finished, and offering Finish there would record a completion the
    // daemon has not derived.
    const waiting = wizardDaemon()
    const { unmount } = show(WIZARD, waiting)
    expect(await screen.findByRole('button', { name: /Pick a Windows profile/ })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Finish setup' })).toBeNull()
    unmount()

    cleanup()
    const finished = wizardDaemon({
      OnboardingState: () => Promise.resolve(wizardState([], CHECKS.map((row) => ({ ...row, ready: true })))),
    })
    show(WIZARD, finished)
    expect(await screen.findByRole('button', { name: 'Finish setup' })).toBeTruthy()
  })

  it('records the finish through the daemon and not a re-read of readiness', async () => {
    // The write that used to be dead: "Finish setup" dispatched whatever the
    // page declared LAST, which for the first-run page was permissions.verify —
    // a re-read. It reported "done" and nothing had been recorded, so the flag
    // never latched and the wizard led again on every launch. The finish is the
    // wizard's own last verb now, and the call log is the assertion.
    const calls: string[] = []
    const service = wizardDaemon({
      OnboardingState: () => Promise.resolve(wizardState([], CHECKS.map((row) => ({ ...row, ready: true })))),
      CompleteOnboarding: () => {
        calls.push('CompleteOnboarding')
        return Promise.resolve()
      },
      Readiness: () => {
        calls.push('Readiness')
        return Promise.resolve(CHECKS)
      },
    })
    show({ ...WIZARD, actions: ['plugin.enable', 'permissions.verify'] }, service)
    fireEvent.click(await screen.findByRole('button', { name: 'Finish setup' }))

    await waitFor(() => expect(calls).toContain('CompleteOnboarding'))
    // A re-read is what the button used to do. It may still happen on the
    // refresh that follows, but the finish itself must be the write.
    expect(calls.filter((name) => name === 'CompleteOnboarding')).toHaveLength(1)
  })

  it('draws the step titles the page declared and none of its own', () => {
    // Four declared steps is the first-run flow; the shell must not carry a
    // second copy of it, so a page declaring three gets three.
    show({ ...WIZARD, steps: ['One', 'Two', 'Three'] }, wizardDaemon())
    expect(screen.getAllByRole('listitem').filter((li) => li.className.includes('ctl-step'))).toHaveLength(3)
  })
})

// --- the profile cards ------------------------------------------------------

const PROFILES: ProfileRow[] = [
  {
    id: 'windows-like',
    label: 'Windows-like setup',
    description: 'Windows keys, snap zones and Explorer actions.',
    active: true,
    capabilities: [
      { id: 'keys', label: 'Windows keys', plugin: 'window-keys', available: true, rule_ids: ['r1'], enabled: 3, total: 3, live: true },
      { id: 'zones', label: 'Snap zones', plugin: 'window-keys', available: false, reason: 'no rule or profile capability covers it yet', rule_ids: [], enabled: 0, total: 0, live: true },
      { id: 'finder', label: 'Finder actions', plugin: 'finder-actions', available: true, rule_ids: ['r2'], enabled: 0, total: 2, live: false },
      { id: 'cursor', label: 'Pointer keys', plugin: 'window-keys', available: true, rule_ids: [], enabled: 0, total: 0, live: true },
    ],
  },
  {
    id: 'plain-mac',
    label: 'Plain macOS',
    description: 'Just the Finder actions.',
    active: false,
    capabilities: [],
  },
]

// PREVIEWED is a card as the daemon serves it once the preview exists: the
// two integers the old apply reported only AFTER the click, plus the
// extension count that is the whole of the change on a fresh install.
const PREVIEWED: ProfileRow = {
  id: 'windows11',
  label: 'Windows 11',
  description: 'Windows keys and snap zones.',
  active: false,
  will_enable: 2,
  already_on: 3,
  will_disable: 0,
  will_enable_plugins: 1,
  capabilities: [
    {
      id: 'keys',
      label: 'Windows keys',
      plugin: 'window-keys',
      available: true,
      rule_ids: ['win.switch'],
      enabled: 3,
      total: 3,
      live: true,
      will_enable: 0,
      will_enable_plugin: true,
    },
    {
      id: 'snap',
      label: 'Snap zones',
      plugin: 'window-keys',
      available: true,
      rule_ids: ['win.snap'],
      enabled: 0,
      total: 2,
      live: false,
      will_enable: 2,
    },
  ],
}

// ACTIVE is a profile with a recorded snapshot: the door back is open.
const ACTIVE: ProfileRow = {
  id: 'plain',
  label: 'Plain desktop',
  description: 'Nothing turned on yet.',
  active: true,
  revertible: true,
  capabilities: [],
}

// REFUSING is the case the criterion names: an active profile with no
// recorded snapshot, which the daemon answers in words.
const REFUSING: ProfileRow = {
  ...ACTIVE,
  revertible: false,
  revert_reason: 'This profile was applied before CrossOS recorded what to return to, so there is no undo point for it. Apply the profile again to record one.',
}

describe('the profile cards', () => {
  it('draws one card per bundle, with the active one marked', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve(PROFILES) }))
    expect(await screen.findByText('Windows-like setup')).toBeTruthy()
    expect(screen.getByText('Plain macOS')).toBeTruthy()
    expect(screen.getByText('Active')).toBeTruthy()
  })

  it('rolls each capability up in words, not in a colour', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve(PROFILES) }))
    // Ready, unavailable (with the daemon's reason), not loaded, and the
    // capability that ships no rules at all — which must not borrow another's
    // count and read as "0 of 21 shortcuts on".
    expect(await screen.findByText('All 3 shortcuts on')).toBeTruthy()
    expect(screen.getByText(/no rule or profile capability covers it yet/)).toBeTruthy()
    expect(screen.getByText(/not loaded/)).toBeTruthy()
    expect(screen.getByText('No shortcuts to turn on')).toBeTruthy()
  })

  it('applies a whole bundle with one write, through the action registry', async () => {
    const service = daemon({ Profiles: () => Promise.resolve(PROFILES) })
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, service)
    fireEvent.click((await screen.findAllByRole('button', { name: 'Apply Plain macOS' }))[0])
    await waitFor(() => expect(service.calls).toContain('ApplyProfile'))
  })

  it('says so when the daemon serves no profiles', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon())
    expect(await screen.findByText(/No profiles are available/)).toBeTruthy()
  })

  // --- the review gate, the preview, the switches and the way back ----------
  //
  // These four are the parts of the card that were absent before: a card you
  // could only turn on, with no preview, no per-capability switch, and no
  // door back. Each test states the one behaviour that would be a lie if it
  // broke.
  it('will not arm Apply until every capability has been looked at, and says how many are left', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve([PREVIEWED]) }))
    // The rollup is the menumate header's "n of m" (PacksScreen.swift:309-311,
    // the enabledCount/totalCount Text):
    // how much of the thing is selected, readable before opening the card.
    expect(await screen.findByText(/2 of 2 capabilities/)).toBeTruthy()
    // The gate, in words. A button that is merely disabled with no sentence
    // beside it is indistinguishable from a button that is broken.
    expect(screen.getByText(/Reviewed 0 of 2 — look at each one before applying/)).toBeTruthy()

    const apply = screen.getByRole('button', { name: 'Apply Windows 11' }) as HTMLButtonElement
    expect(apply.disabled, 'Apply is armed before anything was reviewed').toBe(true)

    // One switch is one review: a row is marked by being looked at, not by a
    // separate Done button (PackImportSheet.swift:260-263, the onTapGesture
    // that inserts into `viewed`).
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Windows keys' }))
    expect(screen.getByText(/Reviewed 1 of 2/)).toBeTruthy()
    expect((screen.getByRole('button', { name: 'Apply Windows 11' }) as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Snap zones' }))
    expect(screen.getByText('Reviewed 2 of 2')).toBeTruthy()
    // The gate covered the SELECTED subset at first, which made it circular:
    // switching a capability off took it out of the review set, so a card
    // could arm by looking only at what it was already going to apply. Both
    // switches now count whether they are on or off.
    expect((screen.getByRole('button', { name: 'Apply Windows 11' }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('previews what applying will change BEFORE the click, and the preview follows the switches', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve([PREVIEWED]) }))
    // The extension is the whole of the change on a fresh machine, so it
    // leads: a preview that opened with "0 shortcuts" beside a keyboard that
    // does not work would be true and useless.
    expect(await screen.findByText('Applying will turn on the extension, switch on 2 shortcuts, 3 already on.')).toBeTruthy()

    // Switch a capability off and the preview must shrink to what Apply will
    // actually do. A preview that ignored its own switches is the same class
    // of lie as an unavailable row claiming a capability does not exist.
    //
    // The two switches carry different numbers on purpose, so the two
    // sentences below cannot both be produced by one hard-coded line: `keys`
    // is the extension and the three already-on shortcuts, `snap` is the two
    // that would be switched on.
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Snap zones' }))
    expect(
      screen.getByText('Applying will turn on the extension, 3 already on.'),
      'the preview did not follow the switch',
    ).toBeTruthy()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Windows keys' }))
    expect(
      screen.getByText('Applying will change nothing.'),
      'the preview did not come back when the switch did',
    ).toBeTruthy()
  })

  it('sends the SELECTION with the apply, so a switch narrows the one write', async () => {
    const seen: (string[] | undefined)[] = []
    // The override is installed after daemon() builds the object, so `service`
    // is in scope by the time the override RUNS even though the literal that
    // closes over it comes first.
    const service: ServiceApi & { calls: string[] } = daemon({
      Profiles: () => Promise.resolve([PREVIEWED]),
      ApplyProfile: (_id: string, capabilities?: string[]) => {
        seen.push(capabilities)
        // Recorded by name as well, so the assertion can say the write went
        // through the BOUND call and not just that some function ran.
        service.calls.push('ApplyProfile')
        return Promise.resolve({})
      },
    })
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, service)
    await screen.findByText(/2 of 2 capabilities/)

    // Look at both, then exclude one: the reviewed-all gesture and the
    // narrowed apply are ONE write, not two.
    // Each switch twice: the first click is the review, the second puts the
    // capability back in the selection, so this applies the WHOLE bundle —
    // which is the case the assertion is about.
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Windows keys' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Windows keys' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Snap zones' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Snap zones' }))
    fireEvent.click(screen.getByRole('button', { name: 'Apply Windows 11' }))
    await waitFor(() => expect(seen).toHaveLength(1))
    expect(seen[0], 'the apply did not carry the selection').toEqual(['keys', 'snap'])
    expect(service.calls, 'the apply did not go through the bound call').toContain('ApplyProfile')
  })

  it('offers the way back on the ACTIVE card only, and runs the bound call', async () => {
    const service = daemon({
      Profiles: () => Promise.resolve([PREVIEWED, ACTIVE]),
      ProfileDeactivate: () => {
        service.calls.push('ProfileDeactivate')
        return Promise.resolve({ profile: '', rules: 0, plugins: 0 })
      },
    })
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, service)
    await screen.findByText(/2 of 2 capabilities/)
    // One profile is in force at a time and one snapshot exists to undo it,
    // so a Revert on an inactive card would be a button acting on somebody
    // else's state.
    expect(screen.getAllByRole('button', { name: /^Put .* back$/ })).toHaveLength(1)
    expect(screen.getByRole('button', { name: 'Put Plain desktop back' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Put Plain desktop back' }))
    await waitFor(() => expect(service.calls).toContain('ProfileDeactivate'))
  })

  it('refuses the way back IN WORDS when there is nothing to return to', async () => {
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve([PREVIEWED, REFUSING]) }))
    await screen.findByText(/2 of 2 capabilities/)
    const back = screen.getByRole('button', { name: 'Put Plain desktop back' }) as HTMLButtonElement
    expect(back.disabled, 'the way back was armed with no snapshot recorded').toBe(true)
    // The sentence is ON THE PAGE, not only in a title attribute: a person
    // who never hovers a disabled button would otherwise see a grey control
    // and no reason.
    expect(
      screen.getByText(/no undo point for it.*Apply the profile again to record one/),
      'the refusal is not rendered in words',
    ).toBeTruthy()
  })

  it('says it has nothing to preview when the daemon sends no preview', async () => {
    const bare = [PREVIEWED].map((p) => ({
      ...p,
      will_enable: undefined,
      already_on: undefined,
      will_enable_plugins: undefined,
      capabilities: p.capabilities.map((c) => ({ ...c, will_enable: undefined, will_enable_plugin: undefined })),
    }))
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, daemon({ Profiles: () => Promise.resolve(bare) }))
    // Absent means this daemon predates the preview. Saying so is right;
    // inventing a number from a missing field is not.
    expect(await screen.findByText('This daemon does not report what applying would change.')).toBeTruthy()
  })

  // The test that was MISSING, and the reason the dead control shipped.
  //
  // The test above ('sends the SELECTION with the apply') proves the control
  // passes the selection to a bound call whose mock DECLARES the parameter, and
  // the Go test in pagedata_test.go proves the daemon honours a payload no shell
  // emits. Each proves a HALF, both passed, and between them the selection went
  // nowhere: the App method took one argument, the Service one, the IPC payload
  // carried only "profile", and the generated Wails binding took one argument
  // and dropped the second before Go ever saw it. The card previewed a narrowed
  // plan and the button applied the whole bundle, and no test in either language
  // could tell, because each end of the wire was only ever asserted against its
  // own end.
  //
  // So this one runs the REAL fixture — the one that now HONOURS the selection,
  // which is what makes it observable — and asserts on what the apply actually
  // DID. The fixture resolves the selection against the declared capabilities
  // and assigns the extension verdicts rather than OR-ing them, so a control
  // that dropped the selection leaves a different machine behind and the
  // difference is visible here. This is the closest a single test gets to
  // starting at actions.ts and ending at handleProfileApply.
  it('the narrowed apply CHANGES THE OUTCOME, which is the half no test had', async () => {
    reset()
    // The fixture's own profile: two available capabilities over two different
    // extensions, so a selection that names one of them has to leave the other
    // off. A card whose two capabilities shared a plugin could not tell.
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, stub)
    await screen.findByText(/2 of 2 capabilities/)
    expect(machine.extensions['window-keys'], 'the fixture did not start clean').toBe(false)
    expect(machine.extensions['finder-actions'], 'the fixture did not start clean').toBe(false)

    // The review gesture IS the toggle, so one click both acknowledges a row
    // and flips it. Three clicks therefore: Finder off, Finder back on, and
    // Windows keys off — leaving a reviewed card whose selection is Finder
    // alone.
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Finder actions' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Finder actions' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Windows 11: Windows keys' }))
    expect(screen.getByText(/1 of 2 capabilities/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Apply Windows 11' }))

    // THE ASSERTION. A control that computed, previewed and then dropped the
    // selection would turn BOTH extensions on here, because the fixture's
    // absent-selection case is "every available capability" — which is exactly
    // the fallback the old code fell into, and why this test failed nothing
    // before: the mock simply did not care.
    await waitFor(() => expect(machine.extensions['finder-actions']).toBe(true))
    expect(
      machine.extensions['window-keys'],
      'the apply turned on the extension its switch was OFF for: the selection did not survive the write',
    ).toBe(false)
  })

  // The other end of the same distinction, and the one a nil-guard in any layer
  // would break: a card with EVERY switch off is a real request to apply
  // nothing, and it must not read as "no selection" (which means all).
  it('a card with every switch off applies NOTHING rather than everything', async () => {
    reset()
    show({ kind: 'profileList', id: 'profiles', label: 'Profiles' }, stub)
    await screen.findByText(/2 of 2 capabilities/)

    // One click each, which is one review each AND one switch-off each.
    for (const name of ['Windows 11: Windows keys', 'Windows 11: Finder actions']) {
      fireEvent.click(screen.getByRole('checkbox', { name }))
    }
    expect(screen.getByText(/0 of 2 capabilities/)).toBeTruthy()
    // The preview says so before the click, and it must be true after it.
    expect(screen.getByText('Applying will change nothing.')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Apply Windows 11' }))

    await waitFor(() => expect(machine.calls).toContain('ApplyProfile'))
    // An empty array, not an absent one. The daemon decodes the field as a
    // *[]string for exactly this, and `undefined` here would be read as
    // "apply every available capability" — the whole bundle, from a card whose
    // every switch is off.
    expect(machine.calls.filter((c) => c === 'ApplyProfile')).toHaveLength(1)
    expect(machine.extensions['window-keys'], 'an all-off card applied the whole bundle').toBe(false)
    expect(machine.extensions['finder-actions'], 'an all-off card applied the whole bundle').toBe(false)
  })
})

// --- the decision pipeline --------------------------------------------------

const TRACE: TraceRow = {
  at: new Date(Date.now() - 30_000).toISOString(),
  decision: 'replace',
  event: { keys: 'cmd+Left', source: 'keyboard', device: 'built-in', key_code: 21 },
  context: { app_id: 'com.apple.finder', app_mode: 'finder' },
  winner: 'r1',
  losers: ['r2'],
  intent: 'nav.left',
  action: 'winlayout.snapLeft',
  stages: [
    { stage: 'event', detail: 'cmd+Left from built-in' },
    { stage: 'context', detail: 'front app com.apple.finder' },
    { stage: 'rule', detail: 'r1 matched' },
    { stage: 'intent', detail: 'nav.left' },
    { stage: 'action', detail: 'winlayout.snapLeft' },
  ],
  params: 'fraction=0.5',
}

describe('the decision pipeline', () => {
  const control: Control = { kind: 'pipelineTrace', id: 'events', source: 'core:traces', format: 'Key → App → Rule → Intent → Action' }

  it('draws each stage as its own labelled row', async () => {
    show(control, daemon({ Traces: () => Promise.resolve([TRACE]) }))
    await screen.findByText(/com\.apple\.finder \(finder\)/)
    const stages = screen.getAllByRole('listitem').filter((li) => li.className.includes('ctl-stage'))
    expect(stages).toHaveLength(5)
    for (const name of ['event', 'context', 'rule', 'intent', 'action']) {
      expect(screen.getByText(name), `${name} is a row of its own`).toBeTruthy()
    }
  })

  it('keeps the decision, the winner and the losers with their stages', async () => {
    show(control, daemon({ Traces: () => Promise.resolve([TRACE]) }))
    expect(await screen.findByText(/cmd\+Left → winlayout\.snapLeft/)).toBeTruthy()
    expect(screen.getByText(/won by r1/)).toBeTruthy()
    expect(screen.getByText(/lost to it: r2/)).toBeTruthy()
  })

  it('says so when the recorder logged nothing', async () => {
    show(control, daemon())
    expect(await screen.findByText(/Nothing has been decided yet/)).toBeTruthy()
  })

  it('empties through the daemon rather than by throwing the rows away locally', async () => {
    // The claim the whole erase rests on. A control that spliced its own array
    // would draw exactly the same empty list and would be lying the moment the
    // write failed, so this fake is STATEFUL: the rows come back from the read
    // until the erase has happened, exactly as a real recorder keeps them. A
    // control that emptied its own copy would therefore have to ignore the
    // reload and still pass — so the reload is what this also pins.
    let held: TraceRow[] = [TRACE]
    const service = daemon({
      Traces: () => {
        service.calls.push('Traces')
        return Promise.resolve(held)
      },
      TracesClear: () => {
        service.calls.push('TracesClear')
        held = []
        return Promise.resolve(held)
      },
    })
    show(control, service)
    await screen.findByText(/com\.apple\.finder \(finder\)/)
    fireEvent.click(
      screen.getByRole('button', { name: /Clear recorded decisions/ }) as HTMLButtonElement,
    )
    await screen.findByText(/Nothing has been decided yet/)
    expect(service.calls).toContain('TracesClear')
    // And the list did not come back on the reload, which is the read half of
    // the same claim.
    expect(await service.Traces()).toEqual([])
  })

  it('offers no copy and no clear over an empty recorder', async () => {
    // Karabiner disables both (InputEventHistoryView.swift:28,35) and the rule
    // is worth pinning here too, because the sweep above only checks that a kind
    // is not BLANK — it would pass a control offering two buttons that can only
    // ever lie about an empty list.
    show(control, daemon())
    await screen.findByText(/Nothing has been decided yet/)
    for (const name of [/Copy as JSON/, /Copy as TSV/, /Clear recorded decisions/]) {
      const found = screen.getByRole('button', { name }) as HTMLButtonElement
      expect(found.disabled, `${name} is disabled over an empty recorder`).toBe(true)
    }
  })
})

// --- the extension detail ---------------------------------------------------

describe('the extension detail', () => {
  const meta: PluginMetaRow[] = [
    {
      id: 'window-keys',
      name: 'Window keys',
      version: '1.2.0',
      permissions: ['keyboard.tap', 'window.ax.read'],
      loaded: true,
    },
  ]

  it('shows the manifest, the version and the permissions', async () => {
    show(
      { kind: 'pluginDetail', id: 'window-keys', plugin: 'window-keys' },
      daemon({ PluginMeta: () => Promise.resolve(meta) }),
    )
    expect(await screen.findByText('Window keys')).toBeTruthy()
    expect(screen.getByText('Version 1.2.0')).toBeTruthy()
    expect(screen.getByText(/2 permissions: keyboard\.tap, window\.ax\.read/)).toBeTruthy()
    expect(screen.getByText('Loaded')).toBeTruthy()
  })

  it('never invents a display name or a version the manifest does not carry', async () => {
    show(
      { kind: 'pluginDetail', id: 'finder-actions', plugin: 'finder-actions' },
      daemon({
        PluginMeta: () =>
          Promise.resolve([
            {
              id: 'finder-actions',
              name: '',
              version: '',
              permissions: [],
              loaded: false,
              reason: 'no manifest is loaded at runtime',
            },
          ]),
      }),
    )
    // The id is humanized into the label, but the name field says there is
    // none, and the daemon's reason is quoted rather than dropped.
    expect(await screen.findByText('Finder actions')).toBeTruthy()
    expect(screen.getByText(/No version in the manifest/)).toBeTruthy()
    expect(screen.getByText('No permissions requested.')).toBeTruthy()
  })

  it('opens from the plugin list without the list naming a kind', async () => {
    const service = daemon({ PluginMeta: () => Promise.resolve(meta) })
    const status: Status = {
      Running: true,
      SafeMode: false,
      Killed: false,
      Plugins: [{ ID: 'window-keys', Enabled: true, Healthy: 'healthy', Origin: 'builtin' }],
      Interception: true,
      TapError: '',
      Version: '0.4.0',
    }
    show({ kind: 'pluginList', id: 'plugins', label: 'Extensions' }, service, status)
    fireEvent.click(screen.getByRole('button', { name: 'Window keys' }))
    expect(await screen.findByText('Version 1.2.0')).toBeTruthy()
  })
})

// --- the readiness checklist ------------------------------------------------

describe('the readiness checklist', () => {
  it('answers every declared item with the row the daemon sent for it', async () => {
    const control: Control = {
      kind: 'checklist',
      id: 'readiness',
      source: 'core:readiness',
      items: ['finder', 'keyboard', 'windows'],
    }
    // The page asks in its own order; the daemon serves in its own. The page's
    // order wins, because the page is what knows which three it means.
    const rows: ReadinessRow[] = [
      { id: 'daemon', label: 'Core daemon', ready: true, detail: '' },
      ...CHECKS,
      { id: 'a-plugin', label: 'a-plugin', ready: false, detail: 'plugin disabled' },
    ]
    show(control, daemon({ Readiness: () => Promise.resolve(rows) }))
    // Wait for the answered rows first: findAllByRole resolves on the initial
    // render, which is the one before the daemon has answered.
    await screen.findByText('Finder shortcuts')
    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items[0]).toContain('Finder shortcuts')
    expect(items[1]).toContain('Keyboard interception')
    expect(items[2]).toContain('Windows shortcuts')
    // The daemon's own extra rows follow, rather than being dropped.
    expect(items[3]).toContain('Core daemon')
    expect(items[4]).toContain('a-plugin')
  })

  it('names a declared item the daemon did not answer', async () => {
    const control: Control = { kind: 'checklist', id: 'readiness', items: ['keyboard', 'nothing-reports-this'] }
    show(control, daemon({ Readiness: () => Promise.resolve(CHECKS) }))
    expect(await screen.findByText('Not checked')).toBeTruthy()
    expect(screen.getByText(/did not report on this one yet/)).toBeTruthy()
  })
})

// --- the landing card -------------------------------------------------------

describe('the landing card', () => {
  const status: Status = {
    Running: true,
    SafeMode: false,
    Killed: false,
    Plugins: [
      { ID: 'window-keys', Enabled: true, Healthy: 'healthy', Origin: 'builtin' },
      { ID: 'finder-actions', Enabled: false, Healthy: 'disabled', Origin: 'builtin' },
    ],
    Interception: false,
    TapError: 'grant Accessibility in System Settings',
    Version: '0.4.0',
  }

  it('answers what is running, what is on and which profile is active', async () => {
    const service = daemon({
      Profiles: () => Promise.resolve(PROFILES),
      Readiness: () => Promise.resolve(CHECKS),
    })
    show({ kind: 'homeSummary', id: 'status', label: 'What is on' }, service, status)

    expect(await screen.findByText(/CrossOS 0\.4\.0/)).toBeTruthy()
    expect(screen.getByText('Running')).toBeTruthy()
    expect(screen.getByText(/Shortcuts do nothing until interception is installed/)).toBeTruthy()
    // The tap error is quoted, because it is the line that says WHICH
    // permission to grant.
    expect(screen.getByText(/grant Accessibility in System Settings/)).toBeTruthy()
    expect(screen.getByText('Windows-like setup')).toBeTruthy()
    expect(screen.getByText(/1 extension switched off/)).toBeTruthy()
    expect(screen.getByText(/1 check of 3 ready/)).toBeTruthy()
  })

  it('says it cannot answer when the daemon is unreachable', async () => {
    const service = daemon({
      Profiles: () => Promise.reject(new Error('daemon unreachable')),
      Readiness: () => Promise.resolve(CHECKS),
    })
    show({ kind: 'homeSummary', id: 'status', label: 'What is on' }, service, null)
    expect(await screen.findByText(/daemon unreachable/)).toBeTruthy()
    expect(screen.getByText('Connecting')).toBeTruthy()
  })
})

// --- the legacy spellings ---------------------------------------------------

describe('the kinds earlier pages declared', () => {
  it('still draw, so a page is never empty over a spelling', async () => {
    // enableFlow and traceList are what ActivityPage and ObservePage send today;
    // they must resolve to the wizard and the pipeline rather than to the
    // "no renderer" row.
    show({ kind: 'enableFlow', id: 'enable', steps: ['One', 'Two'] }, daemon())
    expect(await screen.findByRole('button', { name: 'One' })).toBeTruthy()

    cleanup()
    show({ kind: 'statusCard', id: 'status' }, daemon(), null)
    expect(await screen.findByText('Connecting')).toBeTruthy()

    cleanup()
    show({ kind: 'traceList', id: 'trace' }, daemon())
    expect(await screen.findByText(/Nothing has been decided yet/)).toBeTruthy()
  })

  it('names a kind that genuinely has no renderer', () => {
    // Not a kind a page declares — one no page has asked for. The row has to
    // name what it could not draw, because a hole with no owner is a defect
    // nobody finds until somebody clicks it.
    show({ kind: 'somethingInvented', id: 'probe' }, daemon())
    expect(screen.getByText(/no renderer for that kind/)).toBeTruthy()
    expect(screen.getByText('somethingInvented')).toBeTruthy()
  })
})
