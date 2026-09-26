// The readiness checklist (bead cross-os-itq; ordered by
// w6-frontend-setup-renderers).
//
// Rows come from the readiness source, which is the daemon's own answer about
// what is ready and — the part that matters to a person — what to do about the
// parts that are not. A page also declares the ids it expects (`items`), and
// those lead: a checklist that quietly drops an item the page asked for reads
// as "done" when the truth is "never reported", which on a first-run page is
// the difference between fixing a permission and believing you fixed one.
//
// The declared ids lead for a second reason. The daemon serves more rows than a
// page declares — the core daemon itself, and one row per installed plugin —
// and a first-run page that asked for three items would otherwise open on a
// list of five in which the three it cares about are wherever the daemon's own
// order put them. Declared first, answered second, unreported last: the page
// says which checks it wanted, the daemon says what they are, and anything the
// page wanted and the daemon did not answer is named as unreported rather than
// quietly missing.
//
// That last clause is the one this file is careful about, because "did not
// answer" has three quite different causes and only one of them is the daemon's
// to be reported about:
//
//   - It answered, and the answer left this id out. "Not checked" is the truth.
//   - The read is still in flight. Nothing is known yet.
//   - The read FAILED. Nothing is known, and nothing is coming.
//
// The first-run page is where a person decides whether they are finished, so
// drawing a pending or a failed read as a row of findings tells them to wait
// for an answer that is not on its way — and on a daemon that is down, to wait
// for it indefinitely. The rows the daemon actually filled in are still drawn,
// because those are facts; what is withheld is the verdict on everything the
// daemon did not answer, which is replaced by one line saying that.
//
// The reference this follows is menumate's onboarding diagnosis: a check whose
// state it cannot read carries NO verdict mark at all, rather than a red one
// (App/UI/OnboardingView.swift:210-213, where `granted` is a Bool? and the icon
// is drawn only when it is non-nil). A row with no mark is honest about not
// knowing; a row with a mark is a claim about the machine.
//
// THE ROW IS TWO LINES NOW, and that is the port rather than a redesign. The
// reason used to sit on the label's own line as `ctl-value`, which put a Go
// error value in the same visual rank as the thing it was explaining — and the
// reason it is explaining is a log line: "adapter: tap refused (input-monitoring
// consent missing?)" is `err.Error()` (core/internal/adapter/tap_cgo.go:43) and
// it is not a sentence anybody fixes a permission from.
//
// The reference's row shape is a 13pt semibold title with an 11.5pt label2
// description UNDER it at spacing 1 (OnboardingView.swift:198-213 permissionRow;
// GeneralTab.swift:44-55 and :62-75 the same two lines in a preferences row), so
// the description gets its own line here — `ctl-item is-block`, which is the
// stacking row three other controls already use for a multi-line entry
// (PluginDetailControl.tsx:72, PipelineTraceControl.tsx:242,
// FileTypeListControl.tsx:609) and the only stacking the stylesheet offers
// without a class name this tree may not invent (common.tsx's header).
//
// What goes on that line is lib/readinessReasons.ts's sentence, and it is two
// sentences because the reference uses two: what is wrong, and then the next
// action (GeneralTab.swift's DestructiveRestoreDialog, whose impact text at
// :197-203 is a plain clause followed by the one that has to be believed, each
// in its own voice). The TONE stays on the chip and off the words, which is the
// reference's rule exactly: OnboardingView.swift:262-286 draws the mark in
// orange or green while the text stays in `MMColor.label` and the sub-line in
// `label3`, so a colour-blind reader and a screen reader get the same sentence.

import type { ReactElement } from 'react'
import type { ReadinessRow } from '../types/controls'
import { useResource } from '../lib/useResource'
import { humanize } from '../lib/format'
import { CHIP, explainReadiness } from '../lib/readinessReasons'
import type { ReasonTone } from '../lib/readinessReasons'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * One readiness row: the verdict chip, the daemon's label, and — under them —
 * what is wrong in words, then what to do about it.
 *
 * It is exported because the wizard's closing hand-off (the last step of a
 * first-run flow) lists the SAME rows, and a hand-off that worded a row
 * differently from the checklist beside it would be the two-answers bug this
 * control already exists to prevent: one number on one side of the window and
 * a different one on the other, for one machine. One component, so the wording
 * is one wording.
 *
 * THE RAW LINE IS NOT DELETED. Two reasons, and the second is the rule.
 * First, it is still the truth: the mapping is the shell's, so somebody who
 * needs to know what the daemon actually said needs a way to read it, and the
 * row carries it in `title` for the pointer and draws it in the small line
 * underneath for the screenshot. Second, lib/format.ts's header sets the rule for
 * every formatter in this tree — "a caller that hides the raw value must keep it
 * reachable (a title attribute), because a formatter that guesses wrong would
 * quietly misreport the user's own configuration." A plain sentence over a
 * dropped error string is that exact failure, so the daemon's own words ride
 * along under the paraphrase, dimmer, the way DeclutterSheet.swift:104-109 puts
 * the rejected count under the result sentence and a lastError under that.
 *
 * A detail the table does not know gets NO paraphrase. It is drawn as the
 * daemon's own line, alone, with nothing invented above it — the same decision
 * the reference makes in `onboarding.diag.extNotEnabled` ("Finder extension not
 * yet enabled": a state, and no next action, because there was no door to point
 * at). Shipping a stranger's sentence over an unmapped reason would be worse
 * than shipping the log line with a dimmer font.
 */
export function ReadinessLine(props: { row: ReadinessRow }): ReactElement {
  const { row } = props
  const reason = row.detail ? explainReadiness(row.detail) : undefined
  // The mark follows the reason's tone, and a reason the table does not know
  // gets the plain "Not ready" — an unmapped reason is not an unread one, and
  // drawing it grey would claim the machine had told us nothing when in fact it
  // told us something this build has no words for.
  const tone: ReasonTone = row.ready
    ? 'ready'
    : reason?.tone === 'unread'
      ? 'unread'
      : 'switchedOff'
  return (
    <li className="ctl-item is-block" title={row.detail || undefined}>
      <span className="ctl-chip">{CHIP[tone]}</span>
      <span className="ctl-label">{row.label || humanize(row.id)}</span>
      {reason ? (
        <>
          <span className="ctl-value">{reason.what}</span>
          {reason.next ? <span className="ctl-value">{reason.next}</span> : null}
          {row.detail ? <span className="ctl-empty">{row.detail}</span> : null}
        </>
      ) : row.detail ? (
        // Unmapped, and said to be unmapped rather than dressed in a sentence
        // this shell wrote: the daemon's line, then the note that it has no
        // plain form yet. `ctl-empty` is the dimmer voice the vocabulary has,
        // and the reference's equivalent is a label2 sub-line under a result
        // (OnboardingView.swift:279, DeclutterSheet.swift:108).
        <>
          <span className="ctl-value">{row.detail}</span>
          <span className="ctl-empty">This build has no plainer wording for that yet.</span>
        </>
      ) : null}
    </li>
  )
}

export function ChecklistControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const readiness = useResource(() => ctx.service.Readiness(), ctx.refreshToken, [], ctx.note)
  const declared = Array.isArray(control.items) ? control.items : []
  const reported = readiness.data ?? []
  const rowFor = (id: string) => reported.find((row) => row.id === id)
  const answered = declared.filter((id) => rowFor(id) !== undefined)
  const unreported = declared.filter((id) => rowFor(id) === undefined)
  // Everything the daemon added beyond what the page asked for: the core row
  // and one row per installed plugin.
  const extra = reported.filter((row) => !declared.includes(row.id))

  // Whether a read has LANDED, which is the only state in which a declared id
  // the daemon left out is a fact about the daemon rather than about this page.
  // A load in flight and a load that failed are both "no answer yet", and both
  // render as no verdict rather than as a row of findings.
  const landed = !readiness.loading && readiness.error === ''

  // The line that stands in for the rows a read cannot produce yet. Said in
  // words rather than left silent, because silence next to a list of verdicts
  // reads as "those are all of them" — the missing checks become the checks
  // that passed.
  const noVerdictYet = readiness.error !== ''
    ? 'The daemon was not read, so the checks it did not report have not been looked at. That is a gap in this answer, not a finding about this machine.'
    : 'Asking the daemon now. The checks it has not answered for have not been looked at.'

  if (reported.length === 0 && declared.length === 0) {
    return (
      <ControlFrame
        label={control.label ?? control.id}
        note={control.note}
        error={readiness.error}
      >
        <EmptyState>
          {landed
            ? 'The daemon reported no readiness checks.'
            : noVerdictYet}
        </EmptyState>
        {readiness.loading ? <p className="ctl-value">Checking…</p> : null}
      </ControlFrame>
    )
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={readiness.error}
    >
      <ul className="ctl-list">
        {answered.map((id) => (
          <ReadinessLine key={id} row={rowFor(id)!} />
        ))}
        {landed
          ? unreported.map((id) => (
              <li className="ctl-item" key={`unreported-${id}`}>
                <span className="ctl-chip">Not checked</span>
                <span className="ctl-label">{humanize(id)}</span>
                <span className="ctl-value">The daemon did not report on this one yet.</span>
              </li>
            ))
          : null}
        {extra.map((row) => (
          <ReadinessLine key={row.id} row={row} />
        ))}
      </ul>
      {landed ? null : <p className="ctl-value">{noVerdictYet}</p>}
      {readiness.loading ? <p className="ctl-value">Checking…</p> : null}
    </ControlFrame>
  )
}
