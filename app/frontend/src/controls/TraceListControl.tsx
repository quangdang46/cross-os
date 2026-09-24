// The activity trace (bead cross-os-itq).
//
// These lines used to be joined into one string with ' · ' and printed as a
// single value, which is unreadable the moment anything goes wrong: a run of
// ten lines becomes one wall of text with no start, no end and nothing to
// select. Each line is now its own row, split into the call that produced it
// and what it said, so a timeline can place them and a reader can point at one.
//
// ctx.logs is the bridge's own UI log sink (App.tsx owns the fetch), so this
// control adds no polling of its own.

import type { ReactElement } from 'react'
import { plural, splitLogSource } from '../lib/format'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function TraceListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const lines = Array.isArray(ctx.logs) ? ctx.logs : []
  // Newest first: the log sink appends, so the last line is the one the user
  // just caused and the one they are reading the page for.
  const newestFirst = [...lines].reverse()

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={typeof control.format === 'string' ? control.format : control.note}
    >
      {newestFirst.length === 0 ? (
        <EmptyState>Nothing has happened yet. Entries appear here as CrossOS acts.</EmptyState>
      ) : (
        <>
          <p className="ctl-value">{plural(lines.length, 'entry', 'entries')}, newest first</p>
          <ol className="ctl-list">
            {newestFirst.map((line, index) => {
              const { source, detail } = splitLogSource(line)
              return (
                <li className="ctl-item" key={`${index}-${line}`}>
                  {source ? <span className="ctl-chip">{source}</span> : null}
                  <span className="ctl-value">{detail}</span>
                </li>
              )
            })}
          </ol>
        </>
      )}
    </ControlFrame>
  )
}
