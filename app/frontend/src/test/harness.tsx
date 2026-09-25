// The shell's test harness: one fake bridge, a page factory, and a mount
// helper, so a test states what the daemon SENT rather than how the shell is
// put together.
//
// The fake replaces src/lib/service, which is the shell's only module that
// names the generated bindings. Two things follow from that and both are the
// point: a test never has to load a Wails binding (so the suite runs on a
// checkout where the bindings were never generated), and a page, a control and
// the shell all keep crossing the bridge through the same single seam the
// build checks.
//
// The fixtures use invented ids on purpose. A page id is daemon vocabulary
// (§3.6c), so a test that pasted a real one into a fixture would put a page id
// back into src/ — the exact thing src.test.tsx asserts is absent.

import { render, type RenderResult } from '@testing-library/react'
import { vi } from 'vitest'
import type { ServiceApi, Status } from '../types/controls'

/** What the fake daemon answers with, per call. Mutable per test. */
export const bridge = {
  pages: [] as unknown[],
  status: null as Status | null,
  eventLogs: [] as string[],
  uiLogs: [] as string[],
  /** Everything the shell called, in order, for assertions about the cadence. */
  calls: [] as string[],
  /** A call to reject, by method name. Absent means the call resolves. */
  faults: {} as Record<string, unknown>,
}

function answer<T>(name: string, value: () => T): Promise<T> {
  bridge.calls.push(name)
  const fault = name in bridge.faults ? bridge.faults[name] : undefined
  return fault ? Promise.reject(fault) : Promise.resolve(value())
}

// Only the four calls the shell makes itself. A control that needs a source
// reads it through ControlContext.service, which is this same object, so a test
// that adds a control kind extends the table below rather than the mock.
const fake: ServiceApi = {
  Pages: () => answer('Pages', () => bridge.pages),
  GetStatus: () => answer('GetStatus', () => bridge.status),
  GetEventLogs: () => answer('GetEventLogs', () => bridge.eventLogs),
  UILogs: () => answer('UILogs', () => bridge.uiLogs),
} as unknown as ServiceApi

vi.mock('../lib/service', () => ({ Service: fake, service: fake }))

/** A page as the Host serves it, with the nav fields a test can vary. */
export function page(fields: {
  id: string
  title: string
  group?: string
  order?: number
  symbol?: string
  firstRun?: boolean
  description?: string
  controls?: unknown[]
}): unknown {
  return {
    ID: fields.id,
    Title: fields.title,
    Group: fields.group ?? '',
    Symbol: fields.symbol ?? '',
    Order: fields.order ?? 0,
    FirstRun: fields.firstRun ?? false,
    Schema: {
      type: 'page',
      description: fields.description ?? '',
      ...(fields.firstRun ? { firstRun: true } : {}),
      controls: fields.controls ?? [],
    },
  }
}

/** Clears the fake between tests: a leaked page list is a test that lies. */
export function serve(pages: unknown[], over: Partial<typeof bridge> = {}): void {
  bridge.pages = pages
  bridge.status = over.status ?? null
  bridge.eventLogs = over.eventLogs ?? []
  bridge.uiLogs = over.uiLogs ?? []
  bridge.calls = []
  bridge.faults = over.faults ?? {}
}

/** Mounts the shell and returns the render result plus the fake's call log. */
export async function mount(): Promise<RenderResult & { calls: string[] }> {
  const { default: App } = await import('../App')
  const result = render(<App />)
  return Object.assign(result, { get calls() { return bridge.calls } })
}

/** Waits for the shell's first poll to land. React state needs a turn. */
export async function settle(): Promise<void> {
  const { act } = await import('@testing-library/react')
  await act(async () => {
    await Promise.resolve()
  })
}

/** The visible text of the nav, in DOM order — what a person reads. */
export function navText(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.sections .section-label')).map(
    (node) => node.textContent ?? '',
  )
}

/** The nav group caps, in DOM order. */
export function groupText(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.nav-group-title')).map(
    (node) => node.textContent ?? '',
  )
}
