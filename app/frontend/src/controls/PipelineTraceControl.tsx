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
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements), whose
// simple-modification editor draws one row per rule with each end of the
// remapping in its own labelled control and an arrow between them:
//
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:47-72
//     the row shape — from-entry, arrow, to-entry, each carrying its own value
//     and each independently legible
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:9-21
//     the fixed-width list beside the detail pane: the decisions on the left,
//     the one being read on the right
//
// No code was copied: the reference is SwiftUI on macOS and this is React over
// a declared schema, so the row shape and the list-beside-detail layout
// transfer and the drawing does not. Tracked in
// third_party/Karabiner-Elements/ATTRIBUTION.md.

import type { ReactElement } from 'react'
import { plural, relativeTime } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function PipelineTraceControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const traces = useResource(() => ctx.service.Traces(), ctx.refreshToken, [], ctx.note)
  const rows = traces.data ?? []
  // Newest first: the recorder appends, and the decision a person came here to
  // read is the one CrossOS just made.
  const newestFirst = [...rows].reverse()

  return (
    <ControlFrame
      label={control.label ?? control.id}
      // format is the page's own pipeline caption ("Key → App → Rule → …"), so
      // the page still owns the words; note stays the control's own hint.
      note={typeof control.format === 'string' ? control.format : control.note}
      error={traces.error}
    >
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
