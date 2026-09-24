// The display formatters, which are the only place the shell decides how a
// daemon value reads. Each case below is a value the daemon actually serves,
// and the assertion is the sentence a person should see.

import { describe, expect, it } from 'vitest'
import {
  formatChord,
  formatCountdown,
  humanize,
  plural,
  relativeTime,
  splitLogSource,
} from '../lib/format'
import { asBool, asList, asNumber, asRecord, asText, describeError, failedTo, splitModifiers } from '../lib/wire'

describe('formatChord', () => {
  it('shortens modifiers and capitalizes single keys', () => {
    expect(formatChord('control + shift + K')).toBe('ctrl+shift+K')
    expect(formatChord('cmd+c')).toBe('cmd+C')
  })

  it('keeps a modifier it does not recognise rather than dropping it', () => {
    expect(formatChord('fn + hyper + A')).toBe('fn+hyper+A')
  })

  it('answers an empty chord with an empty string', () => {
    expect(formatChord('  +  ')).toBe('')
  })
})

describe('humanize', () => {
  it('turns daemon ids into words', () => {
    expect(humanize('leftHalf')).toBe('Left Half')
    expect(humanize('windows-keyboard')).toBe('Windows keyboard')
  })

  it('leaves text that is not an identifier alone', () => {
    expect(humanize('Finder menu items')).toBe('Finder menu items')
  })
})

describe('plural', () => {
  it('agrees with the count', () => {
    expect(plural(1, 'rule')).toBe('1 rule')
    expect(plural(2, 'rule')).toBe('2 rules')
    expect(plural(0, 'entry', 'entries')).toBe('0 entries')
  })
})

describe('relativeTime', () => {
  const now = Date.parse('2026-09-24T12:00:00Z')

  it('reads a daemon timestamp as an age', () => {
    expect(relativeTime('2026-09-24T11:57:00Z', now)).toBe('3 minutes ago')
    expect(relativeTime('2026-09-24T11:59:58Z', now)).toBe('just now')
  })

  it('says so when the daemon sent no time, rather than guessing one', () => {
    expect(relativeTime('', now)).toBe('time not reported')
    expect(relativeTime('never', now)).toBe('never')
  })
})

describe('formatCountdown', () => {
  it('renders milliseconds as m:ss', () => {
    expect(formatCountdown(9_000)).toBe('0:09')
    expect(formatCountdown(125_000)).toBe('2:05')
  })

  it('floors a lapsed countdown at zero instead of showing a negative time', () => {
    expect(formatCountdown(-4_000)).toBe('0:00')
  })
})

describe('splitLogSource', () => {
  it('separates the call from what happened', () => {
    expect(splitLogSource('GetMatrix: daemon unreachable')).toEqual({
      source: 'GetMatrix',
      detail: 'daemon unreachable',
    })
  })

  it('keeps a line without that shape whole', () => {
    expect(splitLogSource('the daemon went away')).toEqual({ source: '', detail: 'the daemon went away' })
  })
})

describe('the wire narrowers', () => {
  it('answers a missing or wrongly-typed field without leaking undefined', () => {
    expect(asText(undefined, 'not reported')).toBe('not reported')
    expect(asNumber('3')).toBeNull()
    expect(asNumber(Number.NaN)).toBeNull()
    expect(asList({})).toEqual([])
    expect(asRecord([])).toBeNull()
    expect(asBool('true')).toBeNull()
  })

  it('drops an empty modifier entry rather than reading it as a modifier', () => {
    expect(splitModifiers('ctrl, shift,')).toEqual(['ctrl', 'shift'])
  })

  it('keeps the daemon own wording when a call is rejected', () => {
    expect(describeError('duplicate chord ctrl+c')).toBe('duplicate chord ctrl+c')
    expect(failedTo('Saving the shortcut table', 'duplicate chord ctrl+c')).toBe(
      'Saving the shortcut table did not go through: duplicate chord ctrl+c',
    )
    expect(describeError(undefined)).toBe('the shell got no answer from the daemon')
  })
})
