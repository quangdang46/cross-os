// The recording sentinel: while a shortcut is being recorded, the machine stops
// acting on it (lib/recording.ts, a port of Rectangle's ShortcutRecordingObserver
// and ShortcutManager.swift:288-308).
//
// The properties worth pinning are all about NOT doing too much. The reference's
// observer exists because a naive flag gets this wrong in three separate ways,
// and each one is a way to interfere with what a person deliberately chose.

import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { beginRecording, endRecording, resetRecordingSentinel } from './recording'
import type { ServiceApi } from '../types/controls'

interface Recorder {
  service: ServiceApi
  /** Every SetObserve argument, in order. */
  flips: boolean[]
  notes: string[]
}

function recorder(over: { observing?: boolean; stateFails?: boolean; setFails?: boolean } = {}): Recorder {
  const flips: boolean[] = []
  const notes: string[] = []
  const observing = over.observing === true
  const service = {
    ObserveState: () =>
      over.stateFails
        ? Promise.reject(new Error('the daemon is unreachable'))
        : Promise.resolve({ observe: observing, mode: 'metadata-only' }),
    SetObserve: (on: boolean) => {
      if (over.setFails) return Promise.reject(new Error('the recorder refused'))
      flips.push(on)
      return Promise.resolve()
    },
  } as unknown as ServiceApi
  return { service, flips, notes }
}

const note = (messages: string[]) => (message: string) => messages.push(message)

beforeEach(() => {
  resetRecordingSentinel()
})
afterEach(() => {
  resetRecordingSentinel()
})

describe('the recording sentinel', () => {
  it('stands the machine down for the recording and lets it back up after', async () => {
    const r = recorder()
    expect(await beginRecording(r.service, note(r.notes))).toBe(true)
    expect(r.flips, 'observe is on while a chord is being recorded').toEqual([true])
    await endRecording(r.service, note(r.notes))
    expect(r.flips, 'and off when the last recorder stops').toEqual([true, false])
    expect(r.notes, 'a plain recording says nothing').toEqual([])
  })

  it('never switches off an observe mode the person chose', async () => {
    // The guard that matters most: a person who turned observe on to watch
    // their rules must still have it on after a recorder closes.
    const r = recorder({ observing: true })
    expect(await beginRecording(r.service, note(r.notes))).toBe(true)
    await endRecording(r.service, note(r.notes))
    expect(r.flips, 'nothing was flipped — the state was not ours to change').toEqual([])
    expect(r.notes).toEqual([])
  })

  it('two recorders share one suspension, and only the last one lifts it', async () => {
    // Rectangle tracks a SET of recording views and compares wasRecording
    // against isRecordingAnyView, so the first to start and the last to stop are
    // the only two transitions that matter. Two recorders open at once, one
    // closing, must not un-suspend the other.
    const r = recorder()
    await beginRecording(r.service, note(r.notes))
    await beginRecording(r.service, note(r.notes))
    expect(r.flips, 'the second recorder does not stand it down again').toEqual([true])
    await endRecording(r.service, note(r.notes))
    expect(r.flips, 'nor does the first one closing lift it').toEqual([true])
    await endRecording(r.service, note(r.notes))
    expect(r.flips, 'the last one does').toEqual([true, false])
  })

  it('says the chord may also fire when it cannot read where observe stands', async () => {
    // Fail closed, and in the direction that keeps the person informed: with
    // the position unknown, flipping would risk overwriting a state they chose,
    // so it does not flip — and it says so rather than implying the desktop is
    // safe while it is not.
    const r = recorder({ stateFails: true })
    expect(await beginRecording(r.service, note(r.notes))).toBe(false)
    expect(r.flips, 'nothing was flipped on an unknown state').toEqual([])
    expect(r.notes.join(' ')).toMatch(/may also do what it is bound to/)
  })

  it('says the chord may also fire when the daemon refuses to stand down', async () => {
    const r = recorder({ setFails: true })
    expect(await beginRecording(r.service, note(r.notes))).toBe(false)
    expect(r.flips).toEqual([])
    expect(r.notes.join(' ')).toMatch(/may also do what it is bound to/)
  })

  it('leaves a refused restoration visible rather than silent', async () => {
    // The machine is left standing down, which a person can see on the Observe
    // page — but the note is the only other place it will be said, so it says.
    const flips: boolean[] = []
    const notes: string[] = []
    let failOff = false
    const service = {
      ObserveState: () => Promise.resolve({ observe: false, mode: 'metadata-only' }),
      SetObserve: (on: boolean) => {
        if (failOff) return Promise.reject(new Error('refused'))
        flips.push(on)
        return Promise.resolve()
      },
    } as unknown as ServiceApi
    await beginRecording(service, note(notes))
    failOff = true
    await endRecording(service, note(notes))
    expect(flips).toEqual([true])
    expect(notes.join(' ')).toMatch(/still showing what it would do/)
  })
})
