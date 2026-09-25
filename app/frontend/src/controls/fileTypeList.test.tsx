// The file-type library, asserted against the catalog the daemon serves
// (bead fe-library-control).
//
// The three states, because "the daemon has nothing" and "the daemon is gone"
// must never look alike — a blank section is the failure this file exists to
// prevent:
//
//   1. FULL — a built-in and a custom row draw the five fields the reference's
//      row carries, in its order, and a person can read each one: the toggle,
//      the extension, the label Finder will show, the filename that will be
//      created, and the template affordance.
//   2. EMPTY — an empty catalog says so in words rather than rendering nothing.
//   3. WRITE-REJECTED — the daemon's own refusal reaches the screen verbatim.
//      Both writes answer with the whole catalog, so the control re-renders
//      from the reply; a row left showing a state the daemon refused is a row
//      that now lies about the New menu.
//
// And the three rules the reference is explicit about, each of which is a
// behaviour a person would otherwise only find out about by losing work:
//
//   - An invalid extension keeps its row, disabled, WITH the reason. It is
//     never dropped out from under the person editing it.
//   - The template editor hints and still saves. An unparseable .json template
//     says so and is kept.
//   - The reorder writes the COMPLETE id list, because a partial list is a
//     move the daemon cannot see the end of.
//
// Fixtures use invented display names on purpose: a page id is daemon
// vocabulary (§3.6c), so a real one in a fixture would put a page id back into
// src/ — the exact thing test/shell.test.tsx asserts is absent.

import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ServiceApi } from '../types/controls'
import { renderControl, type ControlContext } from './index'
import type { Control } from '../types/controls'

afterEach(() => cleanup())

interface Row {
  ext: string
  baseName: string
  displayName: string
  template: string
  enabled: boolean
  builtIn: boolean
  menuTitle: string
}

/** One catalog row as the daemon's wire row carries it. Every field is named,
 *  including menuTitle, because two rows sharing a menu title would share an
 *  accessible name — and telling rows apart is the whole reason the row is
 *  drawn field by field rather than summarised. */
function row(fields: Partial<Row> = {}): Row {
  return {
    ext: 'md',
    baseName: 'Untitled',
    displayName: 'New Markdown',
    template: '',
    enabled: true,
    builtIn: true,
    menuTitle: 'New Markdown',
    ...fields,
  }
}

/** A custom row, off by default — which is also the state that makes its
 *  toggle read "Turn on", so a test can name it without hardcoding a verb. */
function custom(fields: Partial<Row> = {}): Row {
  return row({
    ext: 'toml',
    baseName: 'config',
    displayName: 'New TOML',
    menuTitle: 'New TOML',
    builtIn: false,
    enabled: false,
    ...fields,
  })
}

/** The stubbed daemon. A call is recorded by name AND its payload kept, so a
 *  test can assert which write ran and what it sent — "the editor saved the
 *  template" is only worth asserting if the daemon was handed the text. */
function daemon(over: {
  rows?: Row[]
  failOn?: string
  why?: string
} = {}): ServiceApi & { calls: string[]; writes: unknown[] } {
  const calls: string[] = []
  const writes: unknown[] = []
  const rows = over.rows ?? []
  const record = (name: string, answer: () => unknown) => (...args: unknown[]) => {
    calls.push(name)
    if (name !== 'FileTypes') writes.push(args[0])
    if (name === over.failOn) return Promise.reject(new Error(over.why ?? 'refused'))
    return Promise.resolve(answer())
  }
  return {
    calls,
    writes,
    FileTypes: record('FileTypes', () => rows),
    SetFileType: record('SetFileType', () => rows),
    ReorderFileTypes: record('ReorderFileTypes', () => rows),
  } as unknown as ServiceApi & { calls: string[]; writes: unknown[] }
}

function context(service: ServiceApi): ControlContext {
  return {
    service,
    status: null,
    logs: [],
    refreshToken: 0,
    note: () => {},
    refresh: () => {},
    pageId: '',
  }
}

const CONTROL: Control = { kind: 'fileTypeList', id: 'probe', label: 'File Types' }

function show(service: ServiceApi) {
  return render(<>{renderControl(CONTROL, context(service))}</>)
}

async function turn(): Promise<void> {
  const { act } = await import('@testing-library/react')
  await act(async () => {
    await Promise.resolve()
  })
}

describe('the file-type library', () => {
  it('draws the five fields the row carries, in the reference order', async () => {
    show(daemon({ rows: [row()] }))
    await turn()

    // The toggle, the extension, the label, the filename, the template — each
    // named on screen, so a person reads the row rather than decoding it.
    expect(
      screen.getByRole('checkbox', { name: 'Turn off New Markdown' }),
      'the toggle is named after the row and the state it would move to',
    ).toBeTruthy()
    expect(screen.getByText('.md'), 'the extension is shown').toBeTruthy()
    expect(
      screen.getByLabelText('Menu label for New Markdown'),
      'the menu label is its own field',
    ).toBeTruthy()
    expect(
      screen.getByLabelText('Default filename for New Markdown'),
      'the default filename is its own field',
    ).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Add the template for New Markdown' }),
      'the template is reachable from the row',
    ).toBeTruthy()
  })

  it('names the columns in the order the row carries its fields', async () => {
    show(daemon({ rows: [row()] }))
    await turn()

    const captions = Array.from(document.querySelectorAll('.ctl-item .ctl-label')).map(
      (node) => node.textContent,
    )
    // The reference's captions mirror fixed pixel widths; the shell's rows wrap
    // rather than hold a width, so what transfers is the ORDER.
    expect(captions.slice(0, 6)).toEqual([
      'Order',
      'Enabled',
      'Extension',
      'Menu Label',
      'Default Filename',
      'Template',
    ])
  })

  it('shows the filename the type will create, and the label Finder will show', async () => {
    show(daemon({ rows: [row()] }))
    await turn()

    expect(
      (screen.getByLabelText('Default filename for New Markdown') as HTMLInputElement).value,
    ).toBe('Untitled')
    expect((screen.getByLabelText('Menu label for New Markdown') as HTMLInputElement).value).toBe(
      'New Markdown',
    )
  })

  it('says so in words when the catalog is empty', async () => {
    const { container } = show(daemon({ rows: [] }))
    await turn()

    const text = (container.querySelector('section.ctl')?.textContent ?? '')
      .replace('File Types', '')
      .trim()
    expect(text, 'an empty catalog is not a blank section').not.toBe('')
    expect(text, 'and it names what the list was').toMatch(/file types/i)
    expect(container.querySelector('input'), 'no row is drawn for nothing').toBeNull()
    expect(container.querySelector('.ctl-error'), 'empty is not a failure').toBeNull()
  })

  it('offers a built-in no delete, and a custom type one', async () => {
    show(daemon({ rows: [row(), custom()] }))
    await turn()

    expect(
      screen.queryByRole('button', { name: 'Delete the custom type New Markdown' }),
      'a built-in is not deletable',
    ).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Delete the custom type New TOML' }),
      'a custom type is',
    ).toBeTruthy()
  })

  it('marks where the custom types begin', async () => {
    show(daemon({ rows: [row(), custom()] }))
    await turn()

    expect(screen.getByText('Custom Types'), 'the built-ins and customs are divided').toBeTruthy()
  })

  it('keeps a row with an invalid extension on screen, disabled, with the reason', async () => {
    show(
      daemon({
        rows: [custom({ ext: 'a/b', baseName: 'x', displayName: 'Broken', menuTitle: 'Broken' })],
      }),
    )
    await turn()

    // The row is the point: dropping it would take a half-typed extension out
    // from under the person who is typing it.
    expect(screen.getByText('.a/b'), 'the row is still drawn').toBeTruthy()
    const toggle = screen.getByRole('checkbox', { name: 'Turn on Broken' }) as HTMLInputElement
    expect(toggle.disabled, 'and its toggle is off limits').toBe(true)
    expect(
      screen.getByText('Allowed: a-z, 0-9, . _ -'),
      'with the reason beside it, so it is fixable',
    ).toBeTruthy()
  })

  it('hints at an unparseable .json template and still saves it', async () => {
    const service = daemon({
      rows: [
        row({ ext: 'json', baseName: 'data', displayName: 'New JSON', menuTitle: 'New JSON' }),
      ],
    })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('button', { name: 'Add the template for New JSON' }))
    fireEvent.change(screen.getByLabelText('Template for New JSON'), {
      target: { value: '{ not json' },
    })

    // The hint is present, and Save is NOT disabled by it: the reference's
    // editor is a hint and not a validation wall.
    expect(screen.getByText('Not valid JSON yet — you can still save.')).toBeTruthy()
    const save = screen.getByRole('button', { name: 'Save Template' }) as HTMLButtonElement
    expect(save.disabled, 'an unparseable template does not block the save').toBe(false)

    fireEvent.click(save)
    await turn()

    expect(service.calls, 'the write went to the daemon').toContain('SetFileType')
    expect(
      (service.writes[0] as { template: string }).template,
      'and it carried the text the daemon could not parse',
    ).toBe('{ not json')
  })

  it('writes the whole order as an explicit id list', async () => {
    const service = daemon({ rows: [row(), custom()] })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('button', { name: 'Move New Markdown down' }))
    await turn()

    // The action layer unwraps the control's {ids} to the bare list before the
    // bound call, so the daemon is handed the order itself.
    expect(service.writes[0], 'both rows are named, not just the moved one').toEqual([
      'config.toml',
      'Untitled.md',
    ])
  })

  it('names a row by the filename the preset creates', async () => {
    // A blank base name is a dotfile, so the id carries its dot — the shape
    // the daemon's own reorder uses to recognise the row.
    const service = daemon({
      rows: [
        row({ ext: 'env', baseName: '', displayName: 'New .env', menuTitle: 'New .env' }),
        custom(),
      ],
    })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('button', { name: 'Move New .env down' }))
    await turn()

    // Moving the dotfile DOWN puts it second — and its id is ".env", not "env":
    // a blank base name is a dotfile, and the id is the filename created.
    expect(service.writes[0]).toEqual(['config.toml', '.env'])
  })

  it('shows the daemon refusal verbatim when a write is rejected', async () => {
    const service = daemon({ rows: [row()], failOn: 'SetFileType', why: 'the extension is dotted' })
    const { container } = show(service)
    await turn()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Turn off New Markdown' }))
    await turn()

    // The daemon's own words, kept whole: they name the rule that refused the
    // edit, which is the only part that makes it fixable.
    expect(container.querySelector('.ctl-error')?.textContent, 'the refusal is on screen').toMatch(
      /the extension is dotted/,
    )
  })

  it('leaves the toggle on the state the daemon refused', async () => {
    const service = daemon({ rows: [row()], failOn: 'SetFileType', why: 'nope' })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Turn off New Markdown' }))
    await turn()

    // The rows are re-rendered from the daemon's whole-catalog reply, so a
    // refusal leaves the control on the state the daemon still holds — a toggle
    // showing the refused state would be a lie about the New menu.
    const toggle = screen.getByRole('checkbox', { name: 'Turn off New Markdown' }) as HTMLInputElement
    expect(toggle.checked, 'the refused state is not shown as the live one').toBe(true)
    expect(service.calls).toContain('SetFileType')
  })

  it('refuses in words when this build carries no binding for the catalog', async () => {
    // A missing binding is a defect somebody can go and fix, so it is a
    // sentence — not a thrown TypeError, which would be a blank settings page.
    const { container } = render(<>{renderControl(CONTROL, context({} as ServiceApi))}</>)
    await turn()

    expect(container.textContent ?? '').toMatch(/no binding/i)
    expect(container.querySelector('.ctl-error'), 'a refusal is not an error row').toBeNull()
  })

  it('persists a typed field off a debounce, not once per letter', async () => {
    // Ported from newfile App/PreferencesView.swift:20-28, whose own comment
    // records the rule: every edit used to encode and write synchronously from
    // the row's onChange. A person typing a label is not asking the daemon to
    // write once per letter.
    const { act } = await import('@testing-library/react')
    vi.useFakeTimers()
    try {
      const svc = daemon({ rows: [row()] })
      show(svc)
      await turn()

      const field = screen.getByLabelText('Menu label for New Markdown')
      for (const text of ['N', 'Ne', 'New Markdow', 'New Markdown 2']) {
        fireEvent.change(field, { target: { value: text } })
      }
      expect(svc.writes.length, 'no write before the debounce elapses').toBe(0)

      await act(async () => {
        vi.advanceTimersByTime(300)
      })
      await turn()
      expect(svc.writes.length, 'four keystrokes, one write').toBe(1)
      expect(
        (svc.writes[0] as { displayName: string }).displayName,
        'and the write carries the LAST keystroke, not the first',
      ).toBe('New Markdown 2')
    } finally {
      vi.useRealTimers()
    }
  })
})
