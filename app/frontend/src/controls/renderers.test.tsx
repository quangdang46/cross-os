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
import { reset, stub } from '../test/fixtures'
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
        arm: () => {},
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

  it('says what the daemon reports is ready, and what is left', async () => {
    show(WIZARD, wizardDaemon())
    expect(await screen.findByText(/1 check of 3 ready/)).toBeTruthy()
    // The failing rows are named, so "not ready" is never a bare verdict.
    expect(screen.getByText(/Windows shortcuts, Finder shortcuts/)).toBeTruthy()
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
