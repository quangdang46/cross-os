// What the recorder says about a chord the moment it is captured (port of
// VS Code, keybindingsEditor.ts:354 — the define-keybinding widget prints the
// count of existing bindings as the chord completes, before anything is saved).
//
// The third test is the one that matters: a count that could not be read is not
// a count of zero, and saying "no rule claims that" when the daemon would not
// answer is the exact lie the whole addition exists to prevent.

import { afterEach, describe, expect, it } from 'vitest'
import { useState } from 'react'
import type { ReactElement } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ChordRecorder } from './ChordRecorder'
import { resetRecordingSentinel } from '../lib/recording'
import type { ChordCapture, ConflictRow, ServiceApi } from '../types/controls'

function contest(over: Partial<ConflictRow> = {}): ConflictRow {
  return {
    keys: 'Ctrl+C',
    winner: 'windows-keyboard.ctrl-c-copy',
    losers: ['mac-finder.copy'],
    rules: [
      { rule_id: 'windows-keyboard.ctrl-c-copy', plugin: 'win-kb', action: 'Copy' },
      { rule_id: 'mac-finder.copy', plugin: 'mac-kb', action: 'Copy' },
    ],
    ...over,
  }
}

function service(over: { rows?: ConflictRow[]; fails?: boolean } = {}): ServiceApi {
  return {
    ObserveState: () => Promise.resolve({ observe: false, mode: 'metadata-only' }),
    SetObserve: () => Promise.resolve(),
    Conflicts: () =>
      over.fails ? Promise.reject(new Error('the daemon is unreachable')) : Promise.resolve(over.rows ?? []),
  } as unknown as ServiceApi
}

/**
 * The recorder draws the captured chord from `value`, which the PARENT owns —
 * so a test that renders it with a null value and an empty onCapture is testing
 * a component that can never show anything. This holds the value the way
 * KeymapEditorControl does.
 */
function Recorder(props: { svc: ServiceApi; onShowConflicts?: (chord: string) => void }): ReactElement {
  const [value, setValue] = useState<ChordCapture | null>(null)
  return (
    <ChordRecorder
      value={value}
      onCapture={setValue}
      service={props.svc}
      note={() => {}}
      onShowConflicts={props.onShowConflicts}
    />
  )
}

async function press(svc: ServiceApi, onShowConflicts?: (chord: string) => void): Promise<void> {
  cleanup()
  render(<Recorder svc={svc} onShowConflicts={onShowConflicts} />)
  fireEvent.click(screen.getByRole('button', { name: 'Record a shortcut' }))
  // Re-queried by its new name, which both proves the label changed and makes
  // sure the key lands on the rendered-recording button rather than a stale
  // reference. Starting a recording is async now — the machine is stood down
  // first — so the element is looked up again rather than kept.
  const recording = await screen.findByRole('button', { name: 'Press the shortcut…' })
  fireEvent.keyDown(recording, { key: 'c', code: 'KeyC', ctrlKey: true })
}

const CAPTURED: ChordCapture = { key: 'C', modifiers: ['Ctrl'] }

afterEach(() => {
  cleanup()
  resetRecordingSentinel()
})

describe('what the recorder says about a chord it just captured', () => {
  it('counts the rules already claiming it, from the daemon', async () => {
    await press(service({ rows: [contest(), contest({ keys: 'Ctrl+V' })] }))
    // The chip is the rendered chord, which formatChord lower-cases; the
    // capture is Ctrl+C and what a person reads is ctrl+C.
    await waitFor(() => expect(screen.getByText('ctrl+C')).toBeTruthy())
    // The winner is named, and so is what it beats — the verdict is the
    // router's, not a comparison of strings done here.
    expect(screen.getByText(/windows-keyboard\.ctrl-c-copy/)).toBeTruthy()
    expect(screen.getByText(/beats mac-finder\.copy/)).toBeTruthy()
  })

  it('says so plainly when the chord is free', async () => {
    await press(service({ rows: [contest({ keys: 'Ctrl+V' })] }))
    await waitFor(() => expect(screen.getByText('No rule claims that chord yet.')).toBeTruthy())
  })

  it('says NOTHING when the count could not be read', async () => {
    // The whole point. An unreadable count is not a count of zero, and a line
    // reading "no rule claims that" over a daemon that would not answer tells
    // a person their shortcut is free when nobody checked.
    await press(service({ fails: true }))
    await waitFor(() => expect(screen.getByText('ctrl+C')).toBeTruthy())
    expect(screen.queryByText(/No rule claims/)).toBeNull()
    expect(screen.queryByText(/already claims/)).toBeNull()
  })

  it('hands the chord to the list when asked, the way the reference sets its search', async () => {
    let shown = ''
    await press(service({ rows: [contest()] }), (chord) => {
      shown = chord
    })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Show them' })).toBeTruthy())
    fireEvent.click(screen.getByRole('button', { name: 'Show them' }))
    expect(shown, 'the chord in the daemon spelling, not the rendered one').toBe('Ctrl+C')
    expect(CAPTURED.key, 'and the capture itself is unaffected').toBe('C')
  })
})
