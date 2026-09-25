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

import type { ReactElement } from 'react'
import type { ReadinessRow } from '../types/controls'
import { useResource } from '../lib/useResource'
import { humanize } from '../lib/format'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * One readiness row as this file has always drawn it: the verdict chip, the
 * daemon's label, and — the part a person can act on — the daemon's own
 * sentence about what to do.
 *
 * It is exported because the wizard's closing hand-off (the last step of a
 * first-run flow) lists the SAME rows, and a hand-off that worded a row
 * differently from the checklist beside it would be the two-answers bug this
 * control already exists to prevent: one number on one side of the window and
 * a different one on the other, for one machine. One component, so the wording
 * is one wording.
 */
export function ReadinessLine(props: { row: ReadinessRow }): ReactElement {
  const { row } = props
  return (
    <li className="ctl-item">
      <span className="ctl-chip">{row.ready ? 'Ready' : 'Not ready'}</span>
      <span className="ctl-label">{row.label || humanize(row.id)}</span>
      {row.detail ? <span className="ctl-value">{row.detail}</span> : null}
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
