// The remap surfaces (bead w7-frontend-remap).
//
// The four things this file holds down, in the order the spec lists them:
//
//   1. ShortcutListControl dispatches on `source`. Both WindowsPage and
//      CommandsPage declare a `shortcutList`, and before the dispatch they
//      rendered the same table — so the Shortcuts page, whose own description
//      is "every shortcut in one place", drew the window table instead. The
//      assertion is that the two sources draw DIFFERENT rows from the same
//      stubbed daemon, not merely that both draw something.
//
//   2. The keymap editor's four steps are present and are the served ones: a
//      search that filters, a recorder that captures a real press, an action
//      picker that is a SELECT over what the daemon served (never a text
//      field), and a conflict shown as a contest with a winner and losers
//      rather than a refusal.
//
//   3. A duplicate chord is a resolvable state. The control says who wins and
//      who lost, and offers the write that changes the answer. It must never
//      greet a contested chord with an error sentence, because the daemon has
//      already ranked it.
//
//   4. The rule builder assembles IF/AND/THEN from served lists only. Every
//      dimension is a picker or a capture; a field that accepts a typed action
//      name would be a way to save a rule the router cannot run.
//
// And the standing rule for all of them: an empty collection reads as EMPTY,
// not as broken. "No shortcuts are configured" is a fact about the machine; a
// failed load is a fact about the daemon, and the two must never look alike.
//
// Fixtures use invented ids on purpose (the rule harness.tsx states): a page id
// is daemon vocabulary (§3.6c), so pasting a real one into a test would put a
// page id back into src/.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type {
  ConflictRow,
  AppRow,
  MatrixRow,
  PluginMetaRow,
  ServiceApi,
  Status,
  TraceRow,
  UserRuleRow,
} from '../types/controls'
import { renderControl, rendererKinds, type ControlContext } from './index'
import type { Control } from '../types/controls'

afterEach(() => cleanup())

/** The stubbed daemon. A call is recorded by name so a test can assert WHICH
 *  bound method a control reached for — including a method the test overrode,
 *  because "the editor wrote the rule" is only worth asserting if the recorder
 *  saw the write rather than the test's own stub being called by hand. */
function daemon(over: Partial<ServiceApi> = {}): ServiceApi & { calls: string[] } {
  const calls: string[] = []
  const answers: Record<string, unknown> = {
    GetStatus: null,
    GetMatrix: [] as MatrixRow[],
    Traces: [] as TraceRow[],
    Conflicts: [] as ConflictRow[],
    PluginMeta: [] as PluginMetaRow[],
    Apps: [] as AppRow[],
    UserRules: [] as UserRuleRow[],
    SetUserRule: 'r-new',
    DeleteUserRule: [] as UserRuleRow[],
    SetRuleEnabled: true,
    Shortcuts: [],
    SetShortcuts: 0,
    TogglePlugin: undefined,
  }
  const stub: Record<string, unknown> = {}
  for (const [name, answer] of Object.entries(answers)) {
    const own = over[name as keyof ServiceApi]
    stub[name] = (...args: unknown[]) => {
      calls.push(name)
      return own
        ? (own as (...a: unknown[]) => unknown)(...args)
        : Promise.resolve(answer)
    }
  }
  return { ...stub, calls } as unknown as ServiceApi & { calls: string[] }
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

// --- fixtures ---------------------------------------------------------------

/** The window table (config.getShortcuts): three columns, action-led. */
const WINDOW_ROWS = [
  { action: 'snapLeft', modifiers: ['Win'], key: 'Left' },
  { action: 'snapRight', modifiers: ['Win'], key: 'Right' },
]

/** The registry (config.getMatrix): rule-led, one row per compiled rule. */
const MATRIX_ROWS: MatrixRow[] = [
  { rule_id: 'window.left', plugin: 'window-keys', action: 'Left Half', keys: 'Win+Left', contexts: [], enabled: true },
  { rule_id: 'terminal.interrupt', plugin: 'terminal-keys', action: 'Interrupt', keys: 'Ctrl+C', contexts: ['terminal'], enabled: false },
]

function trace(over: Partial<TraceRow> = {}): TraceRow {
  return {
    at: new Date(Date.now() - 60_000).toISOString(),
    decision: 'replace',
    event: { keys: 'Win+Left', source: 'keyboard', key_code: 0x25 },
    context: { app_id: 'com.apple.finder', app_mode: 'finder' },
    winner: 'window.left',
    losers: [],
    intent: 'window.move',
    action: 'snapLeft',
    stages: [],
    params: '',
    ...over,
  }
}

const APPS: AppRow[] = [
  {
    BundleID: 'com.apple.Terminal',
    Executable: '/System/Applications/Utilities/Terminal.app',
    PID: 401,
    DisplayName: 'Terminal',
    AppMode: 'terminal',
    Category: 'terminal',
  },
]

// --- 1. the source dispatch -------------------------------------------------

describe('the shortcut tables, dispatched by source', () => {
  const windowControl: Control = {
    kind: 'shortcutList',
    id: 'shortcuts',
    source: 'core:windowShortcuts',
    editable: true,
    conflicts: 'core:conflicts',
  }
  const allControl: Control = {
    kind: 'shortcutList',
    id: 'all',
    source: 'core:allShortcuts',
    conflicts: 'core:conflicts',
    editLinks: 'owner',
  }

  it('serves the window table and the registry to different rows', async () => {
    const service = daemon({
      Shortcuts: () => Promise.resolve(WINDOW_ROWS),
      GetMatrix: () => Promise.resolve(MATRIX_ROWS),
    })

    show(windowControl, service)
    // The window table's own row: an ACTION name in an editable field, a key,
    // and a modifier — the three columns config.getShortcuts serves.
    const action = await screen.findByLabelText('Action, row 1')
    expect((action as HTMLInputElement).value).toBe('snapLeft')
    expect((screen.getByLabelText('Key, row 1') as HTMLInputElement).value).toBe('Left')
    expect((screen.getByLabelText('Modifiers, row 1') as HTMLInputElement).value).toBe('Win')
    // The registry's action names are nowhere in this table.
    expect(screen.queryByText('Left Half')).toBeNull()

    cleanup()
    show(allControl, service)
    // The registry's own row: a rule's action, its chord, and whether it is on.
    const list = await screen.findByRole('list')
    expect(list.textContent).toContain('Left Half')
    expect(list.textContent).toContain('Interrupt')
    expect(list.textContent).not.toContain('snapLeft')
  })

  it('reads a different bound call for each source, not one call twice', async () => {
    const service = daemon({ GetMatrix: () => Promise.resolve(MATRIX_ROWS) })
    show(windowControl, service)
    await waitFor(() => expect(service.calls).toContain('Shortcuts'))
    expect(service.calls).not.toContain('GetMatrix')

    cleanup()
    const other = daemon({ GetMatrix: () => Promise.resolve(MATRIX_ROWS) })
    show(allControl, other)
    await waitFor(() => expect(other.calls).toContain('GetMatrix'))
    expect(other.calls).not.toContain('Shortcuts')
  })

  it('offers the window table an editor and the registry only a switch', async () => {
    // CommandsPage declares editLinks:"owner": the registry is listed and
    // switched, but its per-rule content is edited on the page that owns it.
    const service = daemon({ Shortcuts: () => Promise.resolve(WINDOW_ROWS) })
    show(windowControl, service)
    expect(await screen.findByRole('button', { name: /Save shortcuts/ })).toBeTruthy()

    cleanup()
    show(allControl, daemon({ GetMatrix: () => Promise.resolve(MATRIX_ROWS) }))
    expect(await screen.findByRole('checkbox', { name: /Turn off Left Half/ })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Save shortcuts/ })).toBeNull()
    expect(screen.getByText(/Edits to these shortcuts are made on the page that owns them/)).toBeTruthy()
  })

  it('writes the switch through the bound rule toggle', async () => {
    const service = daemon({ GetMatrix: () => Promise.resolve(MATRIX_ROWS) })
    show(allControl, service)
    fireEvent.click(await screen.findByRole('checkbox', { name: /Turn on Interrupt/ }))
    await waitFor(() => expect(service.calls).toContain('SetRuleEnabled'))
  })

  it('says so when either table is empty, rather than showing nothing', async () => {
    show(windowControl, daemon())
    expect(await screen.findByText(/No window shortcuts are configured/)).toBeTruthy()

    cleanup()
    show(allControl, daemon())
    expect(await screen.findByText(/No shortcuts are configured/)).toBeTruthy()
  })

  it('says the daemon failed, and does not call an empty list that answer', async () => {
    show(
      allControl,
      daemon({ GetMatrix: () => Promise.reject(new Error('daemon unreachable')) }),
    )
    expect(await screen.findByText(/daemon unreachable/)).toBeTruthy()
    // A failed read must not read as "nothing is configured".
    expect(screen.queryByText(/No shortcuts are configured/)).toBeNull()
  })
})

// --- 2 & 3. the keymap editor ----------------------------------------------

describe('the keymap editor', () => {
  const KEYMAP: Control = { kind: 'keymapEditor', id: 'keymap', source: 'core:behaviorMatrix' }

  function keymapService(over: Partial<ServiceApi> = {}): ServiceApi & { calls: string[] } {
    return daemon({ GetMatrix: () => Promise.resolve(MATRIX_ROWS), Traces: () => Promise.resolve([trace()]), ...over })
  }

  it('lists the served shortcuts, one row per rule', async () => {
    show(KEYMAP, keymapService())
    const list = await screen.findByRole('list')
    expect(list.textContent).toContain('Left Half')
    expect(list.textContent).toContain('Interrupt')
  })

  it('searches: typing a word narrows the list to the rules that match', async () => {
    show(KEYMAP, keymapService())
    const list = await screen.findByRole('list')
    fireEvent.change(screen.getByLabelText('Search shortcuts'), { target: { value: 'interrupt' } })
    await waitFor(() => expect(list.textContent).not.toContain('Left Half'))
    expect(list.textContent).toContain('Interrupt')
  })

  it('says when a search matches nothing, instead of showing an empty pane', async () => {
    show(KEYMAP, keymapService())
    await screen.findByRole('list')
    fireEvent.change(screen.getByLabelText('Search shortcuts'), { target: { value: 'zzz' } })
    expect(await screen.findByText(/No shortcut matches/)).toBeTruthy()
  })

  it('captures a chord by pressing it, in the daemon spelling', async () => {
    // The recorder reads the DOM's key names and stores CrossOS's. "ArrowLeft"
    // is the browser's name for the key the rule table calls "Left"; a capture
    // that stored the browser's spelling would save a chord that renders as a
    // raw keycode in the matrix beside it and never fires.
    const service = keymapService()
    show(KEYMAP, service)
    await screen.findByRole('list')
    fireEvent.click(screen.getByRole('button', { name: 'Record a shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'ArrowLeft',
      code: 'ArrowLeft',
      ctrlKey: true,
    })
    // "ArrowLeft" in, "Left" out — the daemon's own spelling, which is what
    // userrules.KeyCode resolves and the matrix chord renderer names back.
    expect(await screen.findByText('ctrl+Left')).toBeTruthy()
  })

  it('refuses a key it cannot name, instead of storing a chord that cannot fire', async () => {
    const service = keymapService()
    show(KEYMAP, service)
    await screen.findByRole('list')
    fireEvent.click(screen.getByRole('button', { name: 'Record a shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'Unidentified',
      code: 'Unidentified',
    })
    expect(
      await screen.findByText(/CrossOS has no name for that key/),
    ).toBeTruthy()
    expect(service.calls).not.toContain('SetUserRule')
  })

  it('picks the action from the served catalog, and never from a text field', async () => {
    show(KEYMAP, keymapService())
    await screen.findByRole('list')
    // A <select> over the actions the served rules name. A free-text action
    // field would offer actions nothing can run.
    const picker = screen.getByLabelText('Action')
    expect(picker.tagName).toBe('SELECT')
    const options = Array.from(picker.querySelectorAll('option')).map((option) => option.textContent)
    expect(options).toContain('Left Half')
    expect(options).toContain('Interrupt')
  })

  it('saves the picked chord and action as a rule', async () => {
    const sent: UserRuleRow[] = []
    const service = keymapService({
      SetUserRule: (rule: UserRuleRow) => {
        sent.push(rule)
        return Promise.resolve('r-new')
      },
    })
    show(KEYMAP, service)
    await screen.findByRole('list')
    fireEvent.click(screen.getByRole('button', { name: 'Record a shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'k',
      code: 'KeyK',
      ctrlKey: true,
      shiftKey: true,
    })
    fireEvent.change(screen.getByLabelText('Action'), { target: { value: 'Left Half' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }))

    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0].key).toBe('K')
    expect(sent[0].modifiers).toEqual(['Ctrl', 'Shift'])
    expect(sent[0].capability).toBe('Left Half')
    expect(await screen.findByText(/Saved the rule/)).toBeTruthy()
  })

  it('refuses to save without a chord or an action, naming which is missing', async () => {
    const service = keymapService()
    show(KEYMAP, service)
    await screen.findByRole('list')

    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }))
    expect(await screen.findByText(/Record the shortcut first/)).toBeTruthy()
    expect(service.calls).not.toContain('SetUserRule')
  })

  it('shows a contested chord as a contest, never as a save-time error', async () => {
    const service = keymapService({
      Traces: () =>
        Promise.resolve([
          trace({ event: { keys: 'Ctrl+C', source: 'keyboard', key_code: 0x43 }, winner: 'copy.rules', losers: ['terminal.interrupt'] }),
        ]),
    })
    show(KEYMAP, service)
    await screen.findByRole('list')
    fireEvent.click(screen.getByRole('button', { name: 'Record a shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'c',
      code: 'KeyC',
      ctrlKey: true,
    })

    // The winner and the loser are named, and the sentence says the ranking is
    // the engine's. This is the state a duplicate chord produces — not a refusal.
    expect(await screen.findByText(/Ctrl\+C is already claimed by copy.rules/)).toBeTruthy()
    expect(screen.getByText(/terminal.interrupt/)).toBeTruthy()
    // And saving still works: the user is told what will happen, not stopped.
    expect(screen.queryByText(/duplicate chord/i)).toBeNull()
  })

  it('reads an empty registry as empty, not as a broken editor', async () => {
    show(KEYMAP, daemon({ GetMatrix: () => Promise.resolve([]) }))
    expect(await screen.findByText(/No shortcuts are configured/)).toBeTruthy()
    // The flow is still on screen: the recorder and the picker read the served
    // emptiness rather than disappearing.
    expect(screen.getByRole('button', { name: 'Record a shortcut' })).toBeTruthy()
    expect(screen.getByLabelText('Action')).toBeTruthy()
  })
})

// --- 4. the rule builder ----------------------------------------------------

describe('the rule builder', () => {
  const BUILDER: Control = { kind: 'ruleBuilder', id: 'builder', source: 'core:userRules' }

  function builderService(over: Partial<ServiceApi> = {}): ServiceApi & { calls: string[] } {
    return daemon({
      Apps: () => Promise.resolve(APPS),
      GetMatrix: () => Promise.resolve(MATRIX_ROWS),
      UserRules: () => Promise.resolve([]),
      ...over,
    })
  }

  it('assembles IF/AND/THEN from pickers, with no free text anywhere', async () => {
    show(BUILDER, builderService())
    await screen.findByText(/IF App = any app/)

    // Every dimension is a select or a capture. A text input would be a way to
    // save an app id or an action name the daemon has no record of.
    expect(screen.getByLabelText('IF the front app is').tagName).toBe('SELECT')
    expect(screen.getByLabelText('THEN').tagName).toBe('SELECT')
    expect(screen.getByRole('button', { name: 'AND press the shortcut' })).toBeTruthy()
    for (const input of Array.from(document.querySelectorAll('input'))) {
      expect(input.getAttribute('type'), 'no free-text field in the builder').toBe('checkbox')
    }
  })

  it('offers the apps and actions the daemon served, and nothing else', async () => {
    show(BUILDER, builderService())
    await screen.findByText(/IF App = any app/)
    const apps = Array.from(screen.getByLabelText('IF the front app is').querySelectorAll('option'))
    expect(apps.map((option) => option.getAttribute('value'))).toEqual([
      '',
      'com.apple.Terminal',
    ])
    const actions = Array.from(screen.getByLabelText('THEN').querySelectorAll('option'))
    expect(actions.map((option) => option.getAttribute('value'))).toEqual([
      '',
      'Left Half',
      'Interrupt',
    ])
  })

  it('draws the sentence as the picks are made', async () => {
    show(BUILDER, builderService())
    await screen.findByText(/IF App = any app/)
    fireEvent.change(screen.getByLabelText('IF the front app is'), {
      target: { value: 'com.apple.Terminal' },
    })
    fireEvent.change(screen.getByLabelText('THEN'), { target: { value: 'Interrupt' } })
    // The spec's own example, built from the served lists.
    expect(await screen.findByText(/IF App = Terminal AND press a shortcut THEN Interrupt/)).toBeTruthy()
  })

  it('writes the picked dimensions and nothing derived', async () => {
    const sent: UserRuleRow[] = []
    const service = builderService({
      SetUserRule: (rule: UserRuleRow) => {
        sent.push(rule)
        return Promise.resolve('r-1')
      },
    })
    show(BUILDER, service)
    await screen.findByText(/IF App = any app/)
    fireEvent.change(screen.getByLabelText('IF the front app is'), {
      target: { value: 'com.apple.Terminal' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'AND press the shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'c',
      code: 'KeyC',
      ctrlKey: true,
    })
    fireEvent.change(screen.getByLabelText('THEN'), { target: { value: 'Interrupt' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }))

    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0].app_ids).toEqual(['com.apple.Terminal'])
    expect(sent[0].key).toBe('C')
    expect(sent[0].modifiers).toEqual(['Ctrl'])
    expect(sent[0].capability).toBe('Interrupt')
    // The daemon recomputes the derived fields, so the editor never sends a
    // value for one.
    expect(sent[0].priority).toBe(0)
    expect(sent[0].scope).toBe('')
  })

  it('refuses a rule with no action, and changes nothing', async () => {
    const service = builderService()
    show(BUILDER, service)
    await screen.findByText(/IF App = any app/)
    fireEvent.click(screen.getByRole('button', { name: 'AND press the shortcut' }))
    fireEvent.keyDown(screen.getByRole('button', { name: 'Press the shortcut…' }), {
      key: 'c',
      code: 'KeyC',
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }))
    expect(await screen.findByText(/Choose an action from the list/)).toBeTruthy()
    expect(service.calls).not.toContain('SetUserRule')
  })

  it('reads empty collections as empty rather than broken', async () => {
    show(BUILDER, daemon({ Apps: () => Promise.resolve([]) }))
    // An empty rule table, an empty app list, an empty action list: all three
    // are facts about the machine, and all three read that way.
    expect(await screen.findByText(/No rules have been written/)).toBeTruthy()
    expect(screen.getByLabelText('IF the front app is').querySelectorAll('option')).toHaveLength(1)
    expect(screen.getByLabelText('THEN').querySelectorAll('option')).toHaveLength(1)
  })

  it('lists the rules already written and removes one through the bound call', async () => {
    const stored: UserRuleRow = {
      id: 'r-7',
      key: 'C',
      modifiers: ['Ctrl'],
      app_modes: [],
      app_ids: ['com.apple.Terminal'],
      device_id: '',
      capability: 'terminal.interrupt',
      emit: true,
      chord: 'Ctrl+C',
      action: 'Interrupt',
      priority: 10,
      specificity: 2,
      scope: 'app',
    }
    const service = builderService({ UserRules: () => Promise.resolve([stored]) })
    show(BUILDER, service)
    expect(await screen.findByText('ctrl+C')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Remove r-7' }))
    await waitFor(() => expect(service.calls).toContain('DeleteUserRule'))
    expect(await screen.findByText(/Removed r-7/)).toBeTruthy()
  })
})

// --- the conflict resolver --------------------------------------------------

describe('the conflict resolver', () => {
  const RESOLVER: Control = { kind: 'conflictResolver', id: 'conflicts', source: 'core:conflicts' }

  // One contested chord, as the daemon's own source serves it: the chord, the
  // verdict, and every claimant with the plugin and human action the matrix
  // above renders for the same rule. The resolver re-derives none of it.
  function contest(over: Partial<ConflictRow> = {}): ConflictRow {
    return {
      keys: 'Win+Left',
      winner: 'window.left',
      losers: ['app.left'],
      rules: [
        { rule_id: 'window.left', plugin: 'win-wm', action: 'Left half' },
        { rule_id: 'app.left', plugin: 'win-kb', action: 'Copy' },
      ],
      ...over,
    }
  }

  it('names the winner and the losers in the words the matrix uses', async () => {
    show(RESOLVER, daemon({ Conflicts: () => Promise.resolve([contest()]) }))
    expect(await screen.findByText('Win+Left')).toBeTruthy()
    // The plugin and the action travel with the row, so a person reading a
    // collision here and the same rule three controls up is reading the same
    // words both times. The trace path this replaced carried the id alone.
    expect(screen.getByText('Left half (window.left, win-wm)')).toBeTruthy()
    expect(screen.getByText('loses to it: Copy (app.left, win-kb)')).toBeTruthy()
  })

  it('reports a chord nobody has pressed, which a trace could not', async () => {
    // The whole reason the verdict stopped coming from the recorder: a trace
    // row only exists after a decision, so the collision worth warning about —
    // the one that has not fired — was invisible.
    const service = daemon({ Conflicts: () => Promise.resolve([contest()]) })
    show(RESOLVER, service)
    expect(await screen.findByText('Win+Left')).toBeTruthy()
    expect(service.calls).not.toContain('Traces')
  })

  it('resolves a contest by switching the losing rule off, not by refusing', async () => {
    const service = daemon({ Conflicts: () => Promise.resolve([contest()]) })
    show(RESOLVER, service)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Turn off app.left so Win+Left resolves to window.left' }),
    )
    await waitFor(() => expect(service.calls).toContain('SetRuleEnabled'))
  })

  it('says so when nothing is contested', async () => {
    show(RESOLVER, daemon())
    expect(await screen.findByText(/No chord is claimed by two rules/)).toBeTruthy()
  })

  it('names a refused source rather than calling it empty', async () => {
    // "Nothing is contested" and "the daemon would not say" are different
    // answers, and on this page the second one matters: a resolver that draws
    // an empty list when the source failed is telling a person their
    // collisions are resolved when nobody checked.
    const service = daemon({ Conflicts: () => Promise.reject(new Error('the daemon refused')) })
    show(RESOLVER, service)
    expect(await screen.findByText(/the daemon refused/)).toBeTruthy()
    expect(screen.queryByText(/No chord is claimed by two rules/)).toBeNull()
  })
})

// --- the Explorer controls --------------------------------------------------

describe('the Explorer controls', () => {
  it('names the manifest it loaded, and says so when it loaded none', async () => {
    // The metadata line and its EMPTY state, both ported from Windhawk's
    // ModCard: the facts go on one line (ModMetadataLine singleLine) and the
    // slot says NO DESCRIPTION in italics rather than sitting blank (:344-357).
    // The empty half is the one that matters here, because the daemon really
    // does have nothing for most of these rows — and a blank slot reads as a
    // card that has nothing to say rather than one nobody has said anything to.
    const svc = daemon({
      PluginMeta: () =>
        Promise.resolve([
          { id: 'window-keys', name: 'Windows Keyboard', version: '1.2.0', permissions: [], loaded: true },
        ]),
    })
    const status = {
      Plugins: [
        { ID: 'window-keys', Enabled: true, Healthy: 'healthy', Origin: 'builtin' },
        { ID: 'finder-actions', Enabled: false, Healthy: 'disabled', Origin: 'builtin' },
      ],
      Interception: false,
    } as unknown as Status
    show({ kind: 'pluginList', id: 'plugins', label: 'Extensions' }, svc, status)
    expect(await screen.findByText('Windows Keyboard 1.2.0 — loaded')).toBeTruthy()
    // The row the daemon said nothing about says so, in its own words.
    expect(screen.getByText('No manifest has been loaded for this one.')).toBeTruthy()
  })

  it('registers all three kinds, so the page stops rendering placeholders', () => {
    const kinds = rendererKinds()
    for (const kind of ['packList', 'actionSettings', 'gateBadge']) {
      expect(kinds, `${kind} is registered`).toContain(kind)
    }
  })

  it('lists the installed packs and reports that no manifest is loaded', async () => {
    const meta: PluginMetaRow[] = [
      {
        id: 'finder-actions',
        name: '',
        version: '',
        permissions: ['finder.modify'],
        loaded: false,
        reason: 'no manifest is loaded — the builtin plugins are compiled from the rule table',
      },
    ]
    show({ kind: 'packList', id: 'packs', source: 'core:finderPacks' }, daemon({ PluginMeta: () => Promise.resolve(meta) }))
    expect(await screen.findByText('Finder actions')).toBeTruthy()
    // The daemon's own reason is quoted, not a prettified guess at a menu.
    expect(screen.getByText(/no manifest is loaded/)).toBeTruthy()
  })

  it('reads an empty pack list as empty rather than broken', async () => {
    show({ kind: 'packList', id: 'packs', source: 'core:finderPacks' }, daemon())
    expect(await screen.findByText(/No extension packs are installed/)).toBeTruthy()
  })

  it('draws the settings fields the page declared, and says no value is served', async () => {
    show(
      {
        kind: 'actionSettings',
        id: 'actionSettings',
        source: 'core:packAction',
        fields: ['placement', 'variants', 'timeoutSeconds'],
        readOnly: ['targets', 'utis'],
      },
      daemon({
        PluginMeta: () =>
          Promise.resolve([{ id: 'finder-actions', name: '', version: '', permissions: [], loaded: false, reason: 'no manifest is loaded' }]),
      }),
    )
    const pack = await screen.findByLabelText('Pack')
    await waitFor(() => expect(pack.querySelectorAll('option')).toHaveLength(2))
    fireEvent.change(pack, { target: { value: 'finder-actions' } })
    // The page's own vocabulary, in the page's order — never a list kept here.
    expect(await screen.findByText('placement')).toBeTruthy()
    expect(screen.getByText('Timeout Seconds')).toBeTruthy()
    expect(screen.getByText('utis')).toBeTruthy()
    // And no value is invented for any of them.
    expect(screen.getAllByText('Not reported by the daemon')).toHaveLength(3)
    expect(screen.getAllByText('Set by the pack manifest · not editable here')).toHaveLength(2)
  })

  it('reads an empty settings source as empty rather than broken', async () => {
    show({ kind: 'actionSettings', id: 'actionSettings', source: 'core:packAction' }, daemon())
    expect(await screen.findByText(/No extension pack is installed/)).toBeTruthy()
  })

  it('badges a shell-granted extension as Level B and a native one by capability', async () => {
    show(
      { kind: 'gateBadge', id: 'levelB', source: 'core:packActionGate' },
      daemon({
        PluginMeta: () =>
          Promise.resolve([
            { id: 'shelled', name: 'Shelled', version: '', permissions: ['shell.execute'], loaded: false },
            { id: 'native', name: 'Native', version: '', permissions: ['finder.modify'], loaded: false },
          ]),
      }),
    )
    expect(await screen.findByText('Level B — gated')).toBeTruthy()
    expect(screen.getByText('Native capabilities: finder.modify')).toBeTruthy()
  })

  it('reads an empty gate source as empty rather than broken', async () => {
    show({ kind: 'gateBadge', id: 'levelB', source: 'core:packActionGate' }, daemon())
    expect(await screen.findByText(/No extension is installed/)).toBeTruthy()
  })
})
