// The extensions list, asserted on the two rules it now follows.
//
//   1. A row is offered only the write its page declared for it, and the box's
//      name and the write it issues come off ONE value — so a switch cannot
//      claim to do something different from what it does. A row the page
//      declared nothing for draws NO box: not a dead one.
//   2. The detail a reader opened closes with the row it names. A daemon that
//      stops serving that row takes the card with it, rather than leaving a
//      panel open under a list the row is no longer in.
//
// Fixtures use invented ids on purpose — a page, control or plugin id is daemon
// vocabulary, and shell.test.tsx greps for it.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { renderControl, type ControlContext } from './index'
import { rowActionsFor } from './PluginListControl'
import type { Control, PluginMetaRow, PluginState, ServiceApi, Status } from '../types/controls'

afterEach(() => cleanup())

/** One served row. Health is the daemon's own word, and this list draws none. */
function row(over: Partial<PluginState> = {}): PluginState {
  return { ID: 'alpha-keys', Enabled: true, Healthy: 'healthy', Origin: 'builtin', ...over }
}

function status(plugins: PluginState[] | null): Status {
  return {
    Running: true,
    SafeMode: false,
    Killed: false,
    Plugins: plugins,
    Interception: true,
    TapError: '',
    Version: '0.4.0',
  }
}

/** The fake daemon. Calls are recorded by name, so a test can assert WHICH
 *  bound method a row's box actually reached for. */
function daemon(over: Partial<ServiceApi> = {}): ServiceApi & { calls: string[] } {
  const calls: string[] = []
  const record = <T,>(name: string, value: T) => () => {
    calls.push(name)
    return Promise.resolve(value)
  }
  return {
    calls,
    PluginMeta: record('PluginMeta', [] as PluginMetaRow[]),
    TogglePlugin: record('TogglePlugin', undefined),
    ...over,
  } as unknown as ServiceApi & { calls: string[] }
}

function context(service: ServiceApi, served: Status | null): ControlContext {
  return {
    service,
    status: served,
    logs: [],
    refreshToken: 0,
    note: () => {},
    refresh: () => {},
    pageId: '',
  }
}

function list(rowActions: string[]): Control {
  return { kind: 'pluginList', id: 'probe', label: 'Probe', rowActions }
}

/** Mounts the list the way a page would, and hands back the render result so a
 *  test can serve it a second status without unmounting in between. */
function show(control: Control, service: ServiceApi, served: Status | null) {
  return render(<>{renderControl(control, context(service, served))}</>)
}

describe('the action a row may take', () => {
  it('is the one the page declared, with its name read off the same value', () => {
    const on = row({ ID: 'alpha-keys', Enabled: true })
    const both = ['plugin.enable', 'plugin.disable']

    // An enabled row is a target for the write that turns it off, and only for
    // that one: the label is the action's own, not a string spelled again here.
    expect(rowActionsFor(on, both)).toEqual({
      action: 'plugin.disable',
      label: 'Disable alpha-keys',
    })
    // Nothing declared, so nothing is offered. A page that says no is not
    // overruled by a control deciding on its own.
    expect(rowActionsFor(on, []).action).toBeNull()
    // Declaring the one write this row needs is enough; the other direction is
    // not required to be declared for a row that is already on.
    expect(rowActionsFor(on, ['plugin.disable']).action).toBe('plugin.disable')
    // A row that is off needs the other write, and a page that declared only
    // this one does not get it — declared once, in one direction, per row.
    expect(rowActionsFor(row({ Enabled: false }), ['plugin.disable']).action).toBeNull()
  })
})

describe('the list of extensions', () => {
  it('draws no switch for a row the page declared no action for', async () => {
    show(list([]), daemon(), status([row()]))

    await waitFor(() => expect(screen.getByText('Alpha keys')).toBeTruthy())
    // Not a disabled box: no box. A control the owner wired nothing behind is
    // never drawn, because a switch that cannot do what it says is a lie the
    // reader can only discover by pressing it.
    expect(screen.queryByRole('checkbox')).toBeNull()
    // The rest of the row is untouched — the state word is a fact, not a write.
    expect(screen.getByText('Enabled')).toBeTruthy()
  })

  it('writes through the declared action, and offers no route to the other one', async () => {
    const service = daemon()
    show(list(['plugin.enable', 'plugin.disable']), service, status([row()]))

    const box = await screen.findByRole('checkbox', { name: 'Disable alpha-keys' })
    fireEvent.click(box)
    // The resolved action is what reached the daemon, not a call re-derived here.
    await waitFor(() => expect(service.calls).toContain('TogglePlugin'))

    // The same page, declaring only the other direction, for a row that is
    // already on: there is no box, so the write it refused cannot be reached by
    // any route through this list.
    cleanup()
    show(list(['plugin.enable']), daemon(), status([row()]))
    await waitFor(() => expect(screen.getByText('Alpha keys')).toBeTruthy())
    expect(screen.queryByRole('checkbox')).toBeNull()
  })

  it('disables the row being written and re-enables it when the answer lands', async () => {
    let release = (): void => {}
    const held = new Promise<void>((resolve) => {
      release = () => resolve()
    })
    const service = daemon({ TogglePlugin: () => held })

    show(list(['plugin.enable', 'plugin.disable']), service, status([row()]))
    const box = (await screen.findByRole('checkbox', {
      name: 'Disable alpha-keys',
    })) as HTMLInputElement
    fireEvent.click(box)

    // A box that stays live through its own write takes the same edit twice, and
    // the second lands after the first with no way to tell which one stuck.
    await waitFor(() => expect(box.disabled).toBe(true))
    release()
    await waitFor(() => expect(box.disabled).toBe(false))
  })

  it('closes the detail with the row it names', async () => {
    // The manifest still names this id, so the detail can still be DRAWN — the
    // question is whether the list is still offering it a subject.
    const meta: PluginMetaRow[] = [
      { id: 'alpha-keys', name: 'Alpha keys', version: '1.0.0', permissions: [], loaded: true },
    ]
    const service = daemon({ PluginMeta: () => Promise.resolve(meta) })
    const control = list(['plugin.enable', 'plugin.disable'])

    const { rerender } = show(control, service, status([row()]))
    fireEvent.click(await screen.findByRole('button', { name: 'Alpha keys' }))
    expect(await screen.findByText('Version 1.0.0')).toBeTruthy()

    // The daemon no longer serves the row. The card goes with it in the same
    // render, rather than outliving the list it was opened out of.
    rerender(<>{renderControl(control, context(service, status([])))}</>)
    expect(screen.queryByText('Version 1.0.0')).toBeNull()
    expect(screen.getByText('No plugins are installed yet.')).toBeTruthy()
  })

  it('opens the detail without needing a write action on the row', async () => {
    // Reading what an extension may do is a separate question from being able
    // to switch it, so a list that may do neither still opens its detail.
    const service = daemon({
      PluginMeta: () =>
        Promise.resolve([
          {
            id: 'alpha-keys',
            name: 'Alpha keys',
            version: '1.0.0',
            permissions: [],
            loaded: true,
          },
        ]),
    })

    show(list([]), service, status([row()]))
    fireEvent.click(await screen.findByRole('button', { name: 'Alpha keys' }))
    expect(await screen.findByText('Version 1.0.0')).toBeTruthy()
  })
})
