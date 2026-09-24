// The shell's own rules, asserted against the shell rather than against a
// story about it: the nav follows the wire, the opening page is the page the
// host ordered first, and the first-run declaration survives the parse.
//
// Fixtures are invented pages (§ the note in harness.tsx): a real page id in a
// test would be a page id in src/, which is what the last test forbids.

import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, screen } from '@testing-library/react'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { bridge, groupText, mount, navText, page, serve, settle } from './harness'

afterEach(() => cleanup())

// The seven destinations the product's information architecture names, in the
// order a person should meet them. The shell has no list of its own, so this is
// the wire's answer being checked, not a frontend constant restated.
const DESTINATIONS = ['Home', 'Profiles', 'Keyboard', 'Windows', 'Explorer', 'Activity', 'Advanced']

// The Host's served order for a fresh profile: the first-run page leads, then
// one group per section, two pages sharing the lowest order inside the first.
const SERVED = [
  page({ id: 'setup', title: 'Welcome', group: 'home', firstRun: true }),
  page({ id: 'home', title: 'Home', group: 'home' }),
  page({ id: 'profiles', title: 'Profiles', group: 'home', order: 10 }),
  page({ id: 'keyboard', title: 'Keyboard', group: 'shortcuts', order: 20 }),
  page({ id: 'windows', title: 'Windows', group: 'shortcuts', order: 30 }),
  page({ id: 'all', title: 'Shortcuts', group: 'shortcuts', order: 40 }),
  page({ id: 'finder', title: 'Explorer', group: 'shortcuts', order: 50 }),
  page({ id: 'activity', title: 'Activity', group: 'activity', order: 60 }),
  page({ id: 'observe', title: 'Observe', group: 'activity', order: 70 }),
  page({ id: 'extensions', title: 'Extensions', group: 'advanced', order: 80 }),
  page({ id: 'safety', title: 'Safety', group: 'advanced', order: 100 }),
  page({ id: 'about', title: 'About', group: 'advanced', order: 110 }),
]

/**
 * True when `wanted` appears in `seen` in that order, ignoring anything else.
 * Case is folded because the group caps are uppercased by the stylesheet, not
 * by the markup — a destination the daemon names "advanced" is the same word as
 * the "Advanced" a person reads, and the test is about the order, not the case.
 */
function inOrder(seen: string[], wanted: string[]): boolean {
  let at = 0
  for (const entry of seen) {
    if (entry.toLowerCase() === wanted[at].toLowerCase()) at += 1
    if (at === wanted.length) return true
  }
  return at === wanted.length
}

describe('nav', () => {
  it('groups the served pages under a cap per group, in served order', async () => {
    serve(SERVED)
    const { container } = await mount()
    await settle()

    // The cap carries the group's own value; the stylesheet is what turns it
    // into a section label, so the DOM text is the daemon's word.
    expect(groupText(container)).toEqual(['home', 'shortcuts', 'activity', 'advanced'])
    expect(navText(container)).toEqual([
      'Welcome',
      'Home',
      'Profiles',
      'Keyboard',
      'Windows',
      'Shortcuts',
      'Explorer',
      'Activity',
      'Observe',
      'Extensions',
      'Safety',
      'About',
    ])
  })

  it('meets the seven destinations in the order the wire serves them', async () => {
    serve(SERVED)
    const { container } = await mount()
    await settle()

    const seen = Array.from(
      container.querySelectorAll('.nav-group-title, .sections .section-label'),
    ).map((node) => node.textContent ?? '')
    expect(inOrder(seen, DESTINATIONS)).toBe(true)
  })

  it('serves an unnamed group as a plain list, with no placeholder cap', async () => {
    serve([page({ id: 'loose', title: 'Loose' })])
    const { container } = await mount()
    await settle()

    expect(groupText(container)).toEqual([])
    expect(navText(container)).toEqual(['Loose'])
  })
})

describe('the opening page', () => {
  it('is the first page the host served, not the alphabetically first id', async () => {
    // "alpha" sorts before "setup" and is served last: a shell that ordered by
    // id would open on the wrong page here, and the old alphabetical list did.
    serve([
      page({ id: 'setup', title: 'Welcome', group: 'home', firstRun: true }),
      page({ id: 'alpha', title: 'Activity', group: 'activity', order: 60 }),
    ])
    await mount()
    await settle()

    expect(screen.getByRole('heading', { name: 'Welcome', level: 2 })).toBeTruthy()
    const active = screen.getAllByRole('button').filter((b) => b.getAttribute('aria-current'))
    expect(active.map((b) => b.textContent)).toEqual(['WelcomeStart here'])
  })

  it('reads the first-run flag out of the schema rather than dropping it', async () => {
    serve(SERVED)
    const { container } = await mount()
    await settle()

    const flagged = Array.from(container.querySelectorAll('.section')).filter((node) =>
      node.querySelector('.section-flag'),
    )
    expect(flagged.map((node) => node.querySelector('.section-label')?.textContent)).toEqual([
      'Welcome',
    ])
  })

  it('keeps the page the person is on when a later poll still serves it', async () => {
    serve(SERVED)
    const { container } = await mount()
    await settle()

    const keyboard = Array.from(container.querySelectorAll('.section')).find(
      (node) => node.querySelector('.section-label')?.textContent === 'Keyboard',
    ) as HTMLButtonElement
    keyboard.click()
    await settle()

    // A second poll with the same pages must not pull the person back to the
    // landing page, which is the whole reason active is kept in state.
    serve(SERVED)
    const again = await mount()
    await settle()
    expect(again.calls).toContain('Pages')
    expect(
      screen.getByRole('heading', { name: 'Keyboard', level: 2 }),
    ).toBeTruthy()
  })
})

describe('the shell source', () => {
  it('names no page, control or plugin id anywhere in src', async () => {
    // Assembled at runtime so this test file does not itself contain the
    // needle it is looking for.
    const needle = ['core', '.'].join('')
    const root = join(process.cwd(), 'src')
    const files: string[] = []
    const walk = (dir: string): void => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const path = join(dir, entry.name)
        if (entry.isDirectory()) walk(path)
        else if (/\.(ts|tsx|css)$/.test(entry.name)) files.push(path)
      }
    }
    walk(root)

    const hits = files.filter((path) => readFileSync(path, 'utf8').includes(needle))
    expect(hits).toEqual([])
  })

  it('polls the four sources the shell owns, once each', async () => {
    serve(SERVED)
    await mount()
    await settle()

    expect(bridge.calls.slice(0, 4).sort()).toEqual([
      'GetEventLogs',
      'GetStatus',
      'Pages',
      'UILogs',
    ])
  })
})
