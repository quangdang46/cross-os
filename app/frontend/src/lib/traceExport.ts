// Turning the recorded decisions into something a person can hand to somebody
// else.
//
// The page draws the pipeline one stage per row, which is the right shape for
// reading and the wrong shape for pasting into a bug report: nobody wants a
// screenshot of a settings window, they want the list. So the same rows go out
// as JSON (a machine reads it) or as TSV (a spreadsheet does), and the choice
// between them is a menu rather than a guess — the reference makes the same
// choice the same way.
//
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements), whose event
// viewer copies its captured history out in two formats from one menu:
//
//   src/apps/EventViewer/src/EventHistory.swift:310-342
//     copyToPasteboardJSON — one object per entry, the fields that matter and
//     nothing else. A copy that carried the view's own bookkeeping would put
//     the view's internals in someone else's log.
//   src/apps/EventViewer/src/EventHistory.swift:344-360
//     copyToPasteboardTSV — a header row, then one line per entry, columns
//     joined by tabs. A header is not decoration: a column of bare timestamps is
//     unreadable once it is out of this window, and whoever opens the file is
//     the one who has to know what the columns are.
//   src/apps/EventViewer/src/EventHistory.swift:362-368
//     tsvCell — a tab or a newline inside a value is REPLACED, not escaped. This
//     is the load-bearing part and the easiest to get wrong: TSV has no quoting
//     convention, so a value carrying a tab silently becomes two columns and a
//     value carrying a newline becomes two rows. The file still opens, and the
//     decision it claims to describe now has columns it does not have.
//   src/apps/EventViewer/src/View/InputEventHistoryView.swift:14-23
//     the two-format menu, beside a clear button, BOTH disabled while the list
//     is empty — a copy over an empty list puts "[]" on someone's clipboard as
//     though that were evidence.
//
// No code was copied: the reference is Swift over a different record, so the two
// formats and the cell-sanitising rule transfer and the string building does
// not. The one thing that did NOT transfer is the reference's hand-rolled JSON
// assembly — it exists there because the entry pre-escaped one of its own
// fields, and `JSON.stringify` over a typed object cannot produce the malformed
// document that hand-assembly can. Tracked in
// third_party/Karabiner-Elements/ATTRIBUTION.md.

import type { TraceRow } from '../types/controls'

/** The two formats the menu offers, in the order the reference lists them. */
export type TraceFormat = 'json' | 'tsv'

/** The menu as the page draws it, so the label and the branch cannot disagree. */
export const TRACE_FORMATS: { id: TraceFormat; label: string }[] = [
  { id: 'json', label: 'Copy as JSON' },
  { id: 'tsv', label: 'Copy as TSV (for a spreadsheet)' },
]

/**
 * The exported record is the decision and the steps that produced it — not the
 * whole wire row, and not a rendering of it.
 *
 * Two reasons it is not TraceRow. The wire row nests the event and the context,
 * which is right for a renderer that draws them apart and wrong for a table
 * somebody is going to read column by column. And `losers` as an array is
 * unusable in TSV: a list is either joined into one cell here or flattened into
 * variable-width rows, and joining is the only one of those a spreadsheet can
 * still line up.
 */
export interface TraceExportRecord {
  at: string
  decision: string
  keys: string
  source: string
  app: string
  app_mode: string
  winner: string
  losers: string
  intent: string
  action: string
  params: string
  stages: string
}

/**
 * The TSV columns in the order they are written, each pairing the header a
 * reader sees with the field it is. The two are spelled differently on purpose
 * and are NOT interchangeable: a header is for whoever opens the file, and the
 * field names are the wire's own, so the pairing is written out rather than
 * derived. A column list that indexed the record by its own header would read
 * `undefined` for every column whose label differs from its key, and would put
 * twelve empty cells in the file without failing.
 *
 * A consumer that has to guess which column is the timestamp is a consumer
 * that will eventually sort by it.
 */
const TSV_COLUMNS: { header: string; field: keyof TraceExportRecord }[] = [
  { header: 'Timestamp', field: 'at' },
  { header: 'Decision', field: 'decision' },
  { header: 'Keys', field: 'keys' },
  { header: 'Source', field: 'source' },
  { header: 'App', field: 'app' },
  { header: 'App mode', field: 'app_mode' },
  { header: 'Winner', field: 'winner' },
  { header: 'Losers', field: 'losers' },
  { header: 'Intent', field: 'intent' },
  { header: 'Action', field: 'action' },
  { header: 'Params', field: 'params' },
  { header: 'Stages', field: 'stages' },
]

/**
 * One exported record per row. The stages are joined into one cell rather than
 * repeated across a variable-width row, so a decision is always one line and a
 * stage count never shifts every later row's columns.
 */
function record(row: TraceRow): TraceExportRecord {
  return {
    at: row.at,
    decision: row.decision,
    keys: row.event.keys,
    source: row.event.source,
    app: row.context.app_id,
    app_mode: row.context.app_mode,
    winner: row.winner,
    losers: row.losers.join(', '),
    intent: row.intent,
    action: row.action,
    params: row.params,
    stages: row.stages.map((stage) => `${stage.stage}: ${stage.detail}`).join(' | '),
  }
}

/**
 * A TSV cell with its tabs and newlines replaced, ported from tsvCell
 * (EventHistory.swift:362-368).
 *
 * Replaced rather than escaped, and that is the reference's own choice, not a
 * simplification: there is no escape in TSV to escape INTO, so a quoted cell
 * containing a tab is two columns to every reader that is not the spreadsheet
 * that wrote it. A space is the honest loss — the alternative is a file whose
 * shape depends on what the user typed.
 *
 * The value is not trimmed. Trailing whitespace is not the reference's concern
 * and a trim here would quietly shorten a chord's rendering.
 */
function tsvCell(value: string): string {
  return value
    .replace(/\t/g, ' ')
    .replace(/\r\n/g, ' ')
    .replace(/\n/g, ' ')
    .replace(/\r/g, ' ')
}

/**
 * The rows as JSON. `JSON.stringify` over the typed record rather than the
 * reference's hand-assembled string: the reference pre-escaped one field to
 * make concatenation safe, and this cannot produce a document that fails to
 * parse, which is the failure a hand-built export turns into silently — a
 * half-written object that pastes into a bug tracker as broken JSON.
 *
 * Two spaces of indent, because the thing this is pasted into is read by people
 * as often as by machines.
 */
export function tracesAsJson(rows: TraceRow[]): string {
  return JSON.stringify(rows.map(record), null, 2)
}

/**
 * The rows as TSV: a header, then one line per decision, and a trailing newline
 * so the file ends at a line boundary — a TSV whose last line has no terminator
 * is read by some tools as having one fewer row than it has.
 */
export function tracesAsTsv(rows: TraceRow[]): string {
  const lines = [TSV_COLUMNS.map((column) => column.header).join('\t')]
  for (const row of rows) {
    const cells = record(row)
    lines.push(TSV_COLUMNS.map((column) => tsvCell(cells[column.field])).join('\t'))
  }
  return `${lines.join('\n')}\n`
}

/** The export in the format the menu named. */
export function exportTraces(rows: TraceRow[], format: TraceFormat): string {
  return format === 'tsv' ? tracesAsTsv(rows) : tracesAsJson(rows)
}
