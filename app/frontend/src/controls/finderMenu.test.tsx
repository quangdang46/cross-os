// The Explorer right-click menu list, asserted against the table the daemon
// serves and against the builds that do not serve it.
//
// The claim this file opens on is the one the control's own header was written
// for and did not deliver. A settings window that cannot reach its source must
// SAY SO, in words, naming the build's gap — because "this build cannot" is a
// defect somebody can go and fix, and an exception is a blank window. The
// control carried a sentence for that case and a guard that could not take it:
// the seam was assembled by reaching through to `service.FinderMenu` inside a
// closure, so the function always returned an object, the guard was never
// false, and the read it guarded ran anyway — throwing a TypeError from inside
// the effect the shared loader runs, which no error row catches. The page went
// blank instead of saying one sentence. The first test below is that.
//
// The rest holds the three temptations the control names, because they are
// still the ways this list lies:
//
//   - An unsupported row is disabled AND says why in words. The user is still
//     holding the keyboard the daemon would not accept, so "greyed out" with
//     no reason is the one failure they cannot see from where they are.
//   - A refused write puts the switch back and shows the daemon's own words.
//     The write is optimistic, so a rejection that left the box where the click
//     put it would be a switch that now lies about the menu.
//   - "Open in Editor" is a catalog the daemon orders, never one named editor.
//     A catalog that arrives empty SAYS SO rather than offering a submenu with
//     nothing in it.
//
// Fixtures use invented menu ids and invented capability names on purpose. A
// page id in src/ is daemon vocabulary and test/shell.test.tsx greps every file
// under src for the namespaced prefix, so nothing here may spell one out.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { Control, ServiceApi, Status } from '../types/controls'
import { renderControl, type ControlContext } from './index'

afterEach(() => cleanup())

/** One row as the daemon's wire row carries it: the item, the user's toggle,
 *  and whether this host can run the item at all. */
interface Row {
  id: string
  title: string
  contexts: string[]
  needsPaths: boolean
  capability: string
  enabled: boolean
  supported: boolean
  reason?: string
  editors?: { id: string; name: string }[]
}

function row(fields: Partial<Row> & { id: string }): Row {
  return {
    title: fields.id,
    contexts: ['a file'],
    needsPaths: true,
    capability: 'fs.reveal',
    enabled: true,
    supported: true,
    ...fields,
  }
}

/** The Open-in-Editor row, which the control branches on by capability rather
 *  than by id — a capability name is daemon vocabulary, so the test names the
 *  same vocabulary the daemon does rather than the other way round. */
const OPEN_APP = 'app.open'
const editorRow = (editors?: { id: string; name: string }[]): Row =>
  row({ id: 'openEditor', title: 'Open in Editor', capability: OPEN_APP, needsPaths: true, editors })

const SEVEN: Row[] = [
  row({ id: 'newFolder', title: 'New Folder', needsPaths: false, capability: 'fs.mkdir' }),
  row({ id: 'newFile', title: 'New File', needsPaths: false, capability: 'fs.create' }),
  row({ id: 'copyPath', title: 'Copy Path', capability: 'clipboard.copyPath' }),
  row({ id: 'copyRelativePath', title: 'Copy Path as Relative', capability: 'clipboard.copyRelativePath' }),
  row({ id: 'openTerminal', title: 'Open Terminal', needsPaths: false, capability: 'terminal.openAt' }),
  editorRow([{ id: 'com.example.editor', name: 'Example Editor' }]),
  row({ id: 'duplicate', title: 'Duplicate', capability: 'fs.duplicate' }),
]

/** The daemon, with the read and the write both under this test's control. The
 *  two faults are SEPARATE flags because they are separate facts: a read that
 *  fails and a write that is refused put the page in visibly different states,
 *  and one flag driving both would make a test pass for the wrong reason. The
 *  write answers with the WHOLE table, the way the daemon's does, so a control
 *  that re-renders from the reply is exercised rather than assumed. */
function daemon(over: {
  rows?: Row[]
  failRead?: boolean
  failWrite?: boolean
  why?: string
  /** Drop the read from the seam entirely, as a build that predates it does. */
  withoutBinding?: boolean
} = {}): ServiceApi & { writes: { id: string; enabled: boolean }[] } {
  const writes: { id: string; enabled: boolean }[] = []
  const rows = over.rows ?? []
  const surface: Record<string, unknown> = {
    SetMenuItemEnabled: (id: string, enabled: boolean) => {
      writes.push({ id, enabled })
      if (over.failWrite) return Promise.reject(new Error(over.why ?? 'refused'))
      // The daemon answers with the whole table, re-derived.
      return Promise.resolve(
        rows.map((entry) => (entry.id === id ? { ...entry, enabled } : entry)),
      )
    },
  }
  if (!over.withoutBinding) {
    surface.FinderMenu = () =>
      over.failRead
        ? Promise.reject(new Error(over.why ?? 'refused'))
        : Promise.resolve(rows)
  }
  return { ...surface, writes } as unknown as ServiceApi & {
    writes: { id: string; enabled: boolean }[]
  }
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

const CONTROL: Control = { kind: 'menuList', id: 'probe', label: 'Explorer' }

function show(service: ServiceApi) {
  return render(<>{renderControl(CONTROL, context(service))}</>)
}

async function turn(): Promise<void> {
  const { act } = await import('@testing-library/react')
  await act(async () => {
    await Promise.resolve()
  })
}

describe('the Explorer menu list', () => {
  it('refuses in words on a build that carries no binding for the read', async () => {
    // The gap this control has to name rather than throw on. A daemon older
    // than the menu, or a shell built against bindings that predate it, is a
    // real thing a person can be running — and the answer they need is one
    // sentence about it, not a blank window.
    const { container } = show(daemon({ rows: SEVEN, withoutBinding: true }))
    await turn()

    expect(container.textContent).toContain('no binding for the Explorer menu')
    // The empty sentence, not a crash, and not a claim that the daemon has no
    // items — this build never asked it.
    expect(container.textContent).not.toContain('The daemon served no menu items')
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
  })

  it('draws one row per menu item, with the verb it runs and where it appears', async () => {
    // The rows arrive as data and this file names none of them, so a page that
    // disagreed with the menu the user actually right-clicks cannot happen.
    show(daemon({ rows: SEVEN }))
    await turn()

    const items = screen.getAllByRole('listitem').map((li) => li.textContent ?? '')
    expect(items).toHaveLength(SEVEN.length)
    expect(items[0]).toContain('New Folder')
    expect(items[0]).toContain('fs.mkdir')
    // Where it appears, in words a person reads rather than as a context token.
    expect(items[0]).toContain('Appears when you a file is selected')
    // A row that works on a selection says so.
    expect(items[2]).toContain('Works on a selection of files or folders')
  })

  it('greys an unsupported row and says why in words, never in colour alone', async () => {
    // The user is still holding the keyboard the daemon would refuse, so
    // "disabled" with no reason is the one failure they cannot see from where
    // they are. The daemon's own sentence is used verbatim.
    const rows = [
      ...SEVEN,
      row({
        id: 'paste',
        title: 'Paste',
        capability: 'fs.paste',
        supported: false,
        reason: 'the registry has not gained this capability yet',
      }),
    ]
    show(daemon({ rows }))
    await turn()

    const toggle = screen.getByRole('checkbox', { name: 'Turn off Paste' })
    expect(toggle.hasAttribute('disabled')).toBe(true)
    expect(screen.getByText('the registry has not gained this capability yet')).toBeTruthy()
  })

  it('names the capability when the daemon sends no reason of its own', async () => {
    // A capability name IS the thing that is missing, so it is what the reader
    // needs; a row that said only "unavailable" has told them nothing to act on.
    const rows = [row({ id: 'paste', capability: 'clipboard.copyRelativePath', supported: false })]
    show(daemon({ rows }))
    await turn()

    expect(screen.getByText(/This host cannot run clipboard.copyRelativePath/)).toBeTruthy()
  })

  it('says so when the Open-in-Editor catalog arrives empty', async () => {
    // The submenu is a catalog the daemon owns and orders — never one named
    // editor. An empty catalog is said out loud rather than offered as a
    // submenu with nothing in it.
    show(daemon({ rows: [...SEVEN.filter((entry) => entry.id !== 'openEditor'), editorRow()] }))
    await turn()

    expect(screen.getByText(/No editor catalog was served/)).toBeTruthy()
  })

  it('lists the editor catalog in the daemon’s order, and names no editor itself', async () => {
    show(daemon({ rows: [editorRow([{ id: 'com.example.one', name: 'Example One' }, { id: 'com.example.two', name: 'Example Two' }])] }))
    await turn()

    const line = screen.getByText(/Opens with:/).textContent ?? ''
    expect(line).toContain('Example One, Example Two')
    // The row is the catalog's title, never one entry's name promoted to it.
    expect(screen.getByText('Open in Editor')).toBeTruthy()
  })

  it('re-renders from the whole table a write answers with', async () => {
    // The write is optimistic — the box moves before the daemon is asked — so
    // the reply, not the click, is what decides what the row shows.
    const service = daemon({ rows: SEVEN })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Turn off New Folder' }))
    await turn()

    expect(service.writes).toEqual([{ id: 'newFolder', enabled: false }])
    expect(screen.getByRole('checkbox', { name: 'Turn on New Folder' })).toBeTruthy()
  })

  it('puts the switch back and says the daemon’s words when a write is refused', async () => {
    const service = daemon({ rows: SEVEN, failWrite: true, why: 'the menu is read-only right now' })
    show(service)
    await turn()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Turn off New Folder' }))
    await turn()

    // Back where it was: a switch left on a state the daemon refused is a
    // switch that now lies about the menu.
    expect(screen.getByRole('checkbox', { name: 'Turn off New Folder' })).toBeTruthy()
    const refusal = screen.getByRole('alert').textContent ?? ''
    expect(refusal).toContain('the menu is read-only right now')
  })

  it('disables the row it is writing, and only that row', async () => {
    // A second click during the write takes the same edit twice, and the second
    // lands after the first with no way to tell which one stuck.
    let release: () => void = () => {}
    const held = new Promise<Row[]>((resolve) => {
      release = () => resolve([])
    })
    const surface = {
      FinderMenu: () => Promise.resolve(SEVEN),
      SetMenuItemEnabled: () => held,
    } as unknown as ServiceApi
    show(surface)
    await turn()

    fireEvent.click(screen.getByRole('checkbox', { name: 'Turn off New Folder' }))
    await turn()

    expect(screen.getByRole('checkbox', { name: 'Turn off New Folder' }).hasAttribute('disabled')).toBe(true)
    // The rest of the menu is still a person's to use.
    expect(screen.getByRole('checkbox', { name: 'Turn off Copy Path' }).hasAttribute('disabled')).toBe(false)
    release()
  })

  it('says so when the daemon serves an empty table, rather than rendering nothing', async () => {
    show(daemon({ rows: [] }))
    await turn()

    expect(screen.getByText(/The daemon served no menu items/)).toBeTruthy()
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
  })

  it('names a failed read in words, and does not report it as an empty menu', async () => {
    // A rejected read is not an empty menu. The two are the same shape on the
    // wire and completely different facts: one says the daemon looked and found
    // nothing, the other says nobody looked. Telling a person the menu is empty
    // when the daemon is down is a finding this control did not receive, and
    // the readiness checklist beside it on the first-run page refuses to draw
    // exactly that.
    const view = show(daemon({ rows: SEVEN, failRead: true, why: 'the menu source is not answering' }))
    await turn()

    expect(screen.getByRole('alert').textContent).toContain('the menu source is not answering')
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0)
    expect(view.container.textContent).toContain('The menu source did not answer')
    expect(view.container.textContent).not.toContain('The daemon served no menu items')
  })
})
