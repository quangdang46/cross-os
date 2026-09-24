// Recording a chord by pressing it (bead w7-frontend-remap).
//
// The spec's second step is "nhấn tổ hợp phím" — press the key combination — and
// the reason it is a step at all is that the alternative is a text field. A
// person who wants "Ctrl+Shift+K" does not know that CrossOS spells a modifier
// `Ctrl` rather than `control`, that the key is stored under its Windows
// virtual-key name rather than the browser's, or that the order matters; they
// know which three keys they are holding down. So the capture is the input, and
// the three vocabularies a typed chord would get wrong are the shell's problem.
//
// THE KEY NAMES ARE CROSSOS'S, NOT THE DOM'S. A rule stores a key name that
// userrules.KeyCode resolves and the matrix chord renderer names back
// (`Left`, `Return`, `Num1`, `F7`, `A`), so a KeyboardEvent.code is translated
// into that vocabulary here. Handing the DOM's own spelling to the daemon would
// produce a chord that renders as "VK 0x00" in the matrix beside it — a
// shortcut that is saved, listed, and never fires.
//
// The key this shell does not recognise is NOT guessed at. It comes back with
// an empty key and the reason, because a chord assembled from a name the daemon
// cannot resolve is a chord that will be stored and never fire — the exact
// failure the capture exists to prevent. The same goes for a bare modifier with
// no key beside it: Ctrl on its own is not a shortcut, it is a modifier.
//
// Listeners live on the button, not on the window. A settings window that
// swallowed every keystroke while a capture button merely existed would make
// the whole pane unusable, and the capture is opt-in by definition — the user
// clicked Record to start it.

import { useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, ReactElement } from 'react'
import type { ChordCapture } from '../types/controls'
import { formatChord } from '../lib/format'

/**
 * Named keys, in the spelling userrules.Keys serves. The DOM's `event.key` is
 * the lookup: "ArrowLeft" is the browser's name for the key CrossOS calls
 * "Left", and the mapping between them is data, not a rule anyone should have to
 * remember. Keys the generated ranges cover (F1–F24, A–Z, 0–9) and the numpad
 * are NOT listed — they are matched by shape, in nameFromCode below.
 */
const NAMED_KEYS: Record<string, string> = {
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  Enter: 'Return',
  Tab: 'Tab',
  ' ': 'Space',
  Escape: 'Escape',
  Backspace: 'Backspace',
  CapsLock: 'CapsLock',
  PrintScreen: 'PrintScreen',
  Insert: 'Insert',
  Delete: 'Delete',
  MetaLeft: 'LWin',
  MetaRight: 'RWin',
  // The macOS names for the same physical keys, so a capture on a Mac does not
  // depend on the browser having normalised the event already.
  OS: 'LWin',
}

/**
 * The numpad, from `event.code` rather than `event.key`. The browser reports
 * every numpad digit as "Insert" and every operator as its symbol, because they
 * are the same characters the number row produces; `code` is the only place the
 * physical key is distinguishable, which is exactly the distinction a shortcut
 * binding needs.
 */
const NUMPAD_KEYS: Record<string, string> = {
  Numpad0: 'Num0',
  Numpad1: 'Num1',
  Numpad2: 'Num2',
  Numpad3: 'Num3',
  Numpad4: 'Num4',
  Numpad5: 'Num5',
  Numpad6: 'Num6',
  Numpad7: 'Num7',
  Numpad8: 'Num8',
  Numpad9: 'Num9',
  NumpadMultiply: 'NumMultiply',
  NumpadAdd: 'NumAdd',
  NumpadSubtract: 'NumSubtract',
  NumpadDecimal: 'NumDecimal',
  NumpadDivide: 'NumDivide',
}

/** The modifiers a chord may carry, in the order ModifierMask folds them in. */
const MODIFIERS: readonly string[] = ['Ctrl', 'Shift', 'Alt', 'Win']

/** The four modifier keys on their own, which are not shortcuts. */
const MODIFIER_CODES = new Set([
  'ControlLeft',
  'ControlRight',
  'ShiftLeft',
  'ShiftRight',
  'AltLeft',
  'AltRight',
  'MetaLeft',
  'MetaRight',
  'OSLeft',
  'OSRight',
])

/** The chord as a person reads it, in the daemon's modifier order. */
export function renderChord(capture: ChordCapture | null): string {
  if (!capture || capture.key === '') return ''
  const parts = MODIFIERS.filter((name) => capture.modifiers.includes(name))
  return formatChord([...parts, capture.key].join('+'))
}

/**
 * The chord in the DAEMON's own spelling, for comparing against served rows.
 *
 * This is deliberately not the same string as renderChord, and the difference
 * matters: formatChord rewrites a modifier to its short display name, so
 * "Ctrl+C" renders as "ctrl+C", while the chord the daemon sends is "Ctrl+C"
 * (pagedata chordOf). Comparing a RENDERED chord against a SERVED one
 * therefore never matches — a conflict lookup that quietly finds nothing, which
 * is the failure this function exists to prevent.
 */
export function chordKey(capture: ChordCapture | null): string {
  if (!capture || capture.key === '') return ''
  const parts = MODIFIERS.filter((name) => capture.modifiers.includes(name))
  return [...parts, capture.key].join('+')
}

/**
 * keyFromEvent names the pressed key in the daemon's vocabulary, or '' when this
 * shell does not know it. Returning '' rather than a guess is the point: the
 * caller reports the miss, and the user can pick the key from the served list
 * instead of saving a binding that will never fire.
 */
function keyFromEvent(event: ReactKeyboardEvent<HTMLButtonElement>): string {
  const named = NAMED_KEYS[event.key]
  if (named) return named
  const numpad = NUMPAD_KEYS[event.code]
  if (numpad) return numpad
  // F1–F24, the letters and the top-row digits all share the shape the daemon's
  // own vocabulary uses, so a name that already matches is taken as-is.
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(event.key)) return event.key
  if (/^[a-zA-Z0-9]$/.test(event.key)) return event.key.toUpperCase()
  return ''
}

export interface ChordRecorderProps {
  /** The chord captured so far, or null when nothing has been pressed. */
  value: ChordCapture | null
  onCapture: (chord: ChordCapture) => void
  /** Reports a press this shell cannot turn into a chord, in words. */
  onReject?: (reason: string) => void
  disabled?: boolean
  label?: string
}

export function ChordRecorder(props: ChordRecorderProps): ReactElement {
  const [recording, setRecording] = useState(false)
  const label = props.label ?? 'Record a shortcut'

  function press(event: ReactKeyboardEvent<HTMLButtonElement>): void {
    if (!recording) return
    // The key event is consumed while recording, or the button would also take
    // the browser's own activation and a bare Space would press it.
    event.preventDefault()
    event.stopPropagation()
    if (MODIFIER_CODES.has(event.code)) return

    const key = keyFromEvent(event)
    if (key === '') {
      setRecording(false)
      props.onReject?.(
        `CrossOS has no name for that key, so it cannot be stored as a shortcut. Pick one from the list instead.`,
      )
      return
    }
    const modifiers: string[] = []
    if (event.ctrlKey) modifiers.push('Ctrl')
    if (event.shiftKey) modifiers.push('Shift')
    if (event.altKey) modifiers.push('Alt')
    if (event.metaKey) modifiers.push('Win')
    setRecording(false)
    props.onCapture({ key, modifiers })
  }

  return (
    <span className="ctl-recorder">
      <button
        type="button"
        className="ctl-input"
        disabled={props.disabled}
        aria-pressed={recording}
        onClick={() => setRecording(!recording)}
        onKeyDown={press}
        onBlur={() => setRecording(false)}
      >
        {recording ? 'Press the shortcut…' : label}
      </button>
      {recording ? (
        <span className="ctl-value">Recording. Press the keys you want.</span>
      ) : props.value && props.value.key !== '' ? (
        <span className="ctl-chip">{renderChord(props.value)}</span>
      ) : null}
    </span>
  )
}
