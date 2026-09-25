// While a shortcut is being recorded, the machine stops acting on it.
//
// A structural port of Rectangle's rule, and the single most important thing
// its recorder does. Rectangular's ShortcutRecordingObserver exists for no other
// purpose: it watches every recorder view, and the moment the FIRST one starts
// recording the app calls unbindShortcuts(), and when the LAST one stops it
// calls bindShortcuts() again (ShortcutManager.swift:288-308). Without it,
// pressing Ctrl+Shift+C to RECORD that chord would also RUN whatever
// Ctrl+Shift+C is bound to — the recorder would fire the very thing it is
// trying to capture, and the person would be left wondering why their desktop
// rearranged mid-sentence.
//
// CrossOS has the same problem for the same reason. ChordRecorder calls
// preventDefault(), which stops the WEBVIEW from acting on the key; it cannot
// stop the daemon, which taps the keyboard at the system level. So a chord
// recorded on this page really does fire.
//
// The daemon's equivalent of unbinding is observe mode: SetDryRun stages each
// action onto the trace instead of performing it (core/cmd/crossos/main.go,
// handleSetObserve). So recording stands the machine down, and the last
// recorder to stop lets it back up.
//
// It lives in lib/ and not in controls/ on purpose. controls/index.tsx states
// the rule for that directory: a control is "a function of what the daemon said
// and never of module-level state". The state here is about the MACHINE, not
// about one button — two recorders on two pages must share it, which is exactly
// why Rectangle needed an observer class rather than a flag on the view — and it
// is therefore state the controls read rather than state they own.

import { failedTo } from './wire'
import type { ServiceApi } from '../types/controls'

/** How many recorders are open. Zero is the only value that lets the machine act. */
let open = 0
/** Whether THIS sentinel is the reason observe mode is on. */
let ours = false

/**
 * Stand the machine down for the duration of a recording.
 *
 * Returns whether the suspension took. The caller shows different copy when it
 * did not, because a recorder that could not stand the machine down is still
 * useful and still honest — it just has to say the chord may also have fired
 * rather than implying it did not.
 */
export async function beginRecording(
  service: ServiceApi,
  note: (message: string) => void,
): Promise<boolean> {
  open += 1
  if (open > 1) return true // someone else already stood it down

  let observing = false
  try {
    const state = await service.ObserveState()
    observing = state?.observe === true
  } catch (reason) {
    // If the position cannot be read we do NOT flip: we would not know whether
    // we are putting back a state the person chose. Recording still works.
    note(
      `CrossOS could not read where observe mode stands, so the machine was left as it is: ${failedTo(
        'reading observe mode',
        reason,
      )} The shortcut you press to record may also do what it is bound to.`,
    )
    return false
  }
  if (observing) return true // the person turned it on deliberately; it stays on

  try {
    await service.SetObserve(true)
    ours = true
    return true
  } catch (reason) {
    note(
      `CrossOS could not stand the machine down while recording: ${failedTo(
        'switching observe mode on',
        reason,
      )} The shortcut you press to record may also do what it is bound to.`,
    )
    return false
  }
}

/**
 * Let the machine act again, if this sentinel is why it stopped.
 *
 * Two guards matter here and both come from the reference's own shape. The count
 * must reach zero, so one recorder closing does not un-suspend a page that has
 * another open. And `ours` must be true, so a person who switched observe mode
 * on themselves is never switched back off by a recorder closing.
 */
export async function endRecording(
  service: ServiceApi,
  note: (message: string) => void,
): Promise<void> {
  if (open === 0) return
  open -= 1
  if (open > 0) return
  if (!ours) return
  ours = false
  try {
    await service.SetObserve(false)
  } catch (reason) {
    // Fail loudly rather than silently: the machine is left standing down,
    // which a person can see on the Observe page, and the note is the only
    // other place it will be said.
    note(
      `CrossOS could not switch observe mode back off: ${failedTo(
        'switching observe mode off',
        reason,
      )} The machine is still showing what it would do instead of doing it. Open Observe to turn it off.`,
    )
  }
}

/** For tests: a machine with nothing recording. */
export function resetRecordingSentinel(): void {
  open = 0
  ours = false
}
