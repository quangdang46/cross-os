// The snap-zone editor's named places (cross-os-1uu follow-on; the eight
// directions are ported from Rectangle's SnapAreaViewController.swift:16-23).
//
// The port's whole value is that a person NAMES where a window goes instead of
// working out what 0.5 means. These tests pin that, and pin the two things it
// must not do while doing it: overwrite a name a person typed, or pretend an
// exact rectangle is one of the eight.
//
// Fixtures use invented ids on purpose — a page or control id in src/ is daemon
// vocabulary, and shell.test.tsx greps for it.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { renderControl } from './index'
import type { ControlContext } from './index'
import type { Control, ServiceApi, Status, ZoneRow } from '../types/controls'

const CONTROL: Control = { kind: 'zoneEditor', id: 'probe', label: 'Snap zones' }

function zone(over: Partial<ZoneRow> = {}): ZoneRow {
  return { id: '', name: '', x: 0, y: 0, w: 0, h: 0, ...over }
}

function daemon(rows: ZoneRow[]): ServiceApi & { writes: unknown[] } {
  const writes: unknown[] = []
  return {
    writes,
    GetZones: () => Promise.resolve(rows),
    SetZones: (next: unknown) => {
      writes.push(next)
      return Promise.resolve((next as ZoneRow[]).length)
    },
  } as unknown as ServiceApi & { writes: unknown[] }
}

function context(service: ServiceApi): ControlContext {
  return {
    service,
    status: null as Status | null,
    logs: [],
    refreshToken: 0,
    note: () => {},
    refresh: () => {},
    pageId: '',
  }
}

function show(service: ServiceApi): void {
  // Cleared here as well as in afterEach: two mounts left in the document make
  // getByLabelText answer from the first one, which reads as a component bug.
  cleanup()
  render(<>{renderControl(CONTROL, context(service))}</>)
}

afterEach(() => cleanup)

describe('the snap-zone editor', () => {
  it('offers the eight named places, and says so when a row is an exact one', async () => {
    show(daemon([zone({ id: 'left', name: 'Left half', x: 0, y: 0, w: 0.5, h: 1 })]))
    const select = (await screen.findByLabelText('Place, row 1')) as HTMLSelectElement
    const places = Array.from(select.options).map((option) => option.textContent)
    expect(places).toEqual([
      'An exact rectangle',
      'Top left',
      'Top',
      'Top right',
      'Left',
      'Right',
      'Bottom left',
      'Bottom',
      'Bottom right',
    ])
    // A served row that IS one of the eight comes back selected, so the row
    // says what it is rather than presenting four numbers and asking.
    await waitFor(() => expect(select.value).toBe('Left'))
  })

  it('names an exact rectangle as exact rather than guessing which place it is', async () => {
    show(daemon([zone({ id: 'odd', name: 'Odd', x: 0.1, y: 0.2, w: 0.3, h: 0.4 })]))
    const select = (await screen.findByLabelText('Place, row 1')) as HTMLSelectElement
    expect(select.value, 'a rectangle that is none of the eight is not rounded to one').toBe('')
  })

  it('fills the rectangle and a blank name when a place is chosen', async () => {
    const svc = daemon([zone()])
    show(svc)
    fireEvent.change(await screen.findByLabelText('Place, row 1'), {
      target: { value: 'Bottom right' },
    })
    await waitFor(() =>
      expect((screen.getByLabelText('X, row 1') as HTMLInputElement).value).toBe('0.5'),
    )
    expect((screen.getByLabelText('Y, row 1') as HTMLInputElement).value).toBe('0.5')
    expect((screen.getByLabelText('Width, row 1') as HTMLInputElement).value).toBe('0.5')
    expect((screen.getByLabelText('Height, row 1') as HTMLInputElement).value).toBe('0.5')
    // The name and id a person never typed come from the place, because a row
    // with a rectangle and no name is what the daemon refuses first.
    expect((screen.getByLabelText('Name, row 1') as HTMLInputElement).value).toBe('Bottom right')
    expect((screen.getByLabelText('Zone ID, row 1') as HTMLInputElement).value).toBe('bottom-right')
  })

  it('never overwrites a name or an id a person already typed', async () => {
    // The place is geometry. Naming a zone is theirs, and a place picker that
    // renames their zone every time they change its size is a control that
    // fights them.
    show(daemon([zone({ id: 'third', name: 'Third column', x: 0.6, y: 0, w: 0.1, h: 1 })]))
    fireEvent.change(await screen.findByLabelText('Place, row 1'), { target: { value: 'Right' } })
    await waitFor(() =>
      expect((screen.getByLabelText('Width, row 1') as HTMLInputElement).value).toBe('0.5'),
    )
    expect((screen.getByLabelText('Name, row 1') as HTMLInputElement).value).toBe('Third column')
    expect((screen.getByLabelText('Zone ID, row 1') as HTMLInputElement).value).toBe('third')
  })
})
