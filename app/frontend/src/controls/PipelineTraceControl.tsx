// The decision pipeline (bead w6-frontend-setup-renderers).
//
// The previous renderer printed one line per decision — "winner action params=…"
// — which is exactly the sentence nobody can debug with. The daemon already
// records the pipeline as it runs it: core:traces serves a row per decision
// whose `stages` are the steps in the order they ran (event, context, rule,
// intent, action, result — record.go:119-124 and the router that appends them).
// This renders those as labelled rows, one per stage, so "which step decided
// this" is a row a reader can point at instead of a clause inside a sentence.
//
// Stages are rendered as the daemon sends them, never as a fixed list: a stage
// the recorder adds tomorrow appears here with no shell change, and a stage this
// shell has never heard of is still shown rather than dropped. That is the same
// discovery rule the rest of the page follows, applied one level down.
//
// The one thing that is NOT rendered from the trace is a clock. At is the
// daemon's own RFC3339 stamp (pagedata.go), so it is shown verbatim; a time this
// shell invented for a decision it only just read would be a fiction.
//
// What this control adds is the two things the reference's event history offers
// and a read-only list does not: a way to take the list with you, and a way to
// make it go away. Both are ports, and both are ported for the same reason — a
// list of every keystroke decision a session made is only a list somebody can
// keep if they can also destroy it.
//
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements), whose event
// history view is the closest thing in the tree to this control: a list of what
// the keyboard did, with a copy menu and a clear button above it.
//
//   src/apps/EventViewer/src/View/InputEventHistoryView.swift:14-23
//     the copy menu — two formats behind one control, and it is a MENU rather
//     than two buttons because the choice is between two representations of the
//     same rows, not between two actions
//   src/apps/EventViewer/src/View/InputEventHistoryView.swift:28-35
//     BOTH the copy menu and the clear button are disabled while the list is
//     empty. That pairing is the point: a clear offered over an empty list, or a
//     copy that pastes "[]" as though that were evidence, is a button that can
//     only ever lie.
//   src/apps/EventViewer/src/EventHistory.swift:299-301
//     the clear is a real erase of what the model holds
//
// No code was copied: the reference is SwiftUI on macOS and this is React over
// a declared schema, so the menu, the disabled-while-empty rule and the
// two-format export transfer and the drawing does not. The formatting itself
// lives in lib/traceExport.ts, which carries its own attribution. Tracked in
// third_party/Karabiner-Elements/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { Clipboard } from '@wailsio/runtime'
import { plural, relativeTime } from '../lib/format'
import { failedTo } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { TRACE_FORMATS, exportTraces } from '../lib/traceExport'
import type { TraceFormat } from '../lib/traceExport'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'
import type { TraceRow } from '../types/controls'

export function PipelineTraceControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const traces = useResource<TraceRow[] | null>(
    () => ctx.service.Traces(),
    ctx.refreshToken,
    null,
    ctx.note,
  )
  const rows = traces.data ?? []
  // Newest first: the recorder appends, and the decision a person came here to
  // read is the one CrossOS just made.
  const newestFirst = [...rows].reverse()

  // The three in-flight states are separate, because they fail differently. A
  // copy that cannot reach the clipboard has not lost anything — the rows are
  // still on screen — so it must not put the list in the same visual state as
  // an erase that failed, where the rows the person believes they destroyed are
  // still on disk. One flag per operation, each disabling only its own button.
  const [copied, setCopied] = useState('')
  const [copyError, setCopyError] = useState('')
  const [cleared, setCleared] = useState('')
  const [clearing, setClearing] = useState(false)
  const [clearError, setClearError] = useState('')

  // Nothing to copy and nothing to clear. Karabiner disables both over an empty
  // list, and the reason is the same here: "[]" on the clipboard reads as a
  // report of what the recorder saw, and an erase offered over nothing is a
  // control that can only ever claim to have done something.
  const hasRows = rows.length > 0

  async function copy(format: TraceFormat): Promise<void> {
    setCopyError('')
    setCopied('')
    // The two confirmations are mutually exclusive, and the reason is not
    // tidiness. "3 decisions copied" and "5 decisions cleared" on one screen are
    // a claim about the clipboard and a claim about the recorder, and a
    // confirmation left over from the operation that came first describes a
    // list that no longer exists.
    setCleared('')
    const label = TRACE_FORMATS.find((entry) => entry.id === format)?.label ?? 'Copy'
    try {
      await Clipboard.SetText(exportTraces(rows, format))
      // The count is in the confirmation because "copied" with no number is
      // indistinguishable from "copied whatever was on screen before you
      // arrived", and a person about to paste into a bug report needs to know
      // they got the whole tail rather than the last row.
      setCopied(`${label} — ${plural(rows.length, 'decision', 'decisions')}.`)
      ctx.note(`${label}: ${plural(rows.length, 'decision', 'decisions')}.`)
    } catch (reason) {
      const message = failedTo(label, reason)
      setCopyError(message)
      ctx.note(message)
    }
  }

  async function clear(): Promise<void> {
    setClearError('')
    setCleared('')
    setClearing(true)
    try {
      // The WRITE, and then the daemon's own answer. Redrawing from the reply
      // rather than emptying a local array is the whole reason this verb
      // returns the list: a page that emptied its own copy would show "cleared"
      // over rows the recorder still holds if the write had failed, and a
      // keystroke log somebody believes they destroyed is not a small lie.
      const left = await ctx.service.TracesClear()
      const remaining = left ?? []
      if (remaining.length > 0) {
        setClearError(
          `The daemon answered the clear with ${plural(remaining.length, 'decision', 'decisions')} still in the list. Nothing was erased.`,
        )
        ctx.note('The clear did not empty the recorder.')
        return
      }
      setCopied('')
      // The confirmation says how many were erased, because the empty list that
      // replaces them is the only evidence a person has, and an empty list is
      // also what a page that never recorded anything looks like. "Cleared" and
      // "there was never anything" must not draw the same screen.
      setCleared(`${plural(rows.length, 'decision', 'decisions')} cleared from the recorder.`)
      traces.reload()
      ctx.refresh()
      ctx.note('The recorded decisions are cleared.')
    } catch (reason) {
      const message = failedTo('Clear the recorded decisions', reason)
      setClearError(message)
      ctx.note(message)
    } finally {
      setClearing(false)
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      // BOTH, never one instead of the other. `format` is the page's one-line
      // caption for the pipeline ("Key → App → Rule → …") and `note` is the
      // page's own disclosure about it — that this is the last 200 decisions,
      // re-read on a poll, and a decision that falls off the end is gone.
      //
      // Choosing between them dropped the disclosure on BOTH pages, because both
      // declare a `format`. The sentence that tells a reader this list is
      // capped and how fresh it is was being discarded in favour of a caption
      // that says neither, on the one screen whose whole job is saying what
      // their rules did.
      note={[control.format, control.note]
        .filter((part): part is string => typeof part === 'string' && part !== '')
        .join(' — ')}
      error={traces.error}
    >
      {/* InputEventHistoryView.swift:14-35 — the two operations over a list
          somebody can only read, disabled together while there is nothing. */}
      <div className="ctl-actions">
        {TRACE_FORMATS.map((format) => (
          <button
            className="ctl-input"
            type="button"
            key={format.id}
            disabled={!hasRows}
            onClick={() => void copy(format.id)}
          >
            {format.label}
          </button>
        ))}
        {/* :30-35 — the clear is styled as destructive, because stopping a
            recorder keeping what the user typed is not the mirror of starting
            one. Same treatment ObserveToggleControl gives its stop button. */}
        <button
          className="ctl-input ctl-stop"
          type="button"
          disabled={!hasRows || clearing}
          onClick={() => void clear()}
        >
          {clearing ? 'Clearing…' : 'Clear recorded decisions'}
        </button>
      </div>
      {copied !== '' ? (
        <p className="ctl-value" role="status">
          {copied}
        </p>
      ) : null}
      {cleared !== '' ? (
        <p className="ctl-value" role="status">
          {cleared}
        </p>
      ) : null}
      {copyError !== '' ? (
        <p className="ctl-error" role="alert">
          {copyError} The decisions are still on screen and nothing was erased.
        </p>
      ) : null}
      {clearError !== '' ? (
        <p className="ctl-error" role="alert">
          {clearError}
        </p>
      ) : null}
      {newestFirst.length === 0 ? (
        <EmptyState>Nothing has been decided yet. Entries appear here as CrossOS acts.</EmptyState>
      ) : (
        <>
          <p className="ctl-value">{plural(rows.length, 'decision', 'decisions')}, newest first</p>
          <ol className="ctl-list">
            {newestFirst.map((trace, index) => (
              <li className="ctl-item is-block" key={`${index}-${trace.at}`}>
                <span className="ctl-label">
                  {trace.event.keys || 'no key'} → {trace.action || trace.decision || 'no action'}
                </span>
                <span className="ctl-value">
                  {trace.context.app_id ? `${trace.context.app_id} (${trace.context.app_mode})` : 'no focused app'}
                  {trace.winner ? ` · won by ${trace.winner}` : ''}
                  {trace.losers.length > 0 ? ` · lost to it: ${trace.losers.join(', ')}` : ''}
                </span>
                <span className="ctl-value">
                  {trace.at ? `${relativeTime(trace.at)}${trace.event.device ? ` · ${trace.event.device}` : ''}` : 'time not reported'}
                  {trace.params ? ` · ${trace.params}` : ''}
                </span>
                {trace.stages.length === 0 ? (
                  <span className="ctl-empty">The recorder logged no stages for this decision.</span>
                ) : (
                  <ol className="ctl-stages">
                    {trace.stages.map((stage, stageIndex) => (
                      <li className="ctl-stage" key={`${stageIndex}-${stage.stage}`}>
                        <span className="ctl-stage-name">{stage.stage}</span>
                        <span className="ctl-value">{stage.detail || 'no detail recorded'}</span>
                      </li>
                    ))}
                  </ol>
                )}
              </li>
            ))}
          </ol>
        </>
      )}
      {traces.loading ? <p className="ctl-value">Reading the recorder…</p> : null}
    </ControlFrame>
  )
}
