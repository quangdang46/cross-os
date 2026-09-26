// Row furniture shared by every control renderer (bead cross-os-itq).
//
// The class names used here are the entire styling vocabulary the shell
// stylesheet defines: ctl, ctl-head, ctl-label, ctl-value, ctl-input,
// ctl-select, ctl-slider, ctl-toggle, ctl-list, ctl-item, ctl-chip, ctl-empty,
// ctl-error, ctl-off, ctl-actions, ctl-countdown, ctl-steps, ctl-step,
// ctl-step-button, ctl-step-mark, ctl-step-index, ctl-step-label, ctl-rollup,
// and — for the detail pane
// below — ctl-facts, ctl-fact, ctl-fact-key, ctl-fact-body, ctl-fact-value,
// ctl-fact-lock, ctl-fact-hint, ctl-facts-foot. A control that needs a look it
// cannot get from these is a control whose design has not been decided yet, and
// inventing a class name is how the stylesheet and the renderers drift apart.

import type { ReactElement, ReactNode } from 'react'
import { useId } from 'react'
import type { Control } from '../types/controls'
import type { ControlContext } from './index'

/** What every renderer receives. Both halves are required: a control with no
 *  service cannot be discovered UI, and a control with no page id cannot report
 *  where a failure came from. */
export interface ControlProps {
  control: Control
  ctx: ControlContext
}

/**
 * EmptyState is the "never a blank page" rule in one element. A list or value
 * that can legitimately arrive empty says so in words, so that "the daemon has
 * nothing" never looks identical to "the daemon is gone" — collapsing those
 * two is exactly how a settings window becomes a blank page.
 */
export function EmptyState(props: { children: ReactNode }): ReactElement {
  return <p className="ctl-empty">{props.children}</p>
}

export function ControlFrame(props: {
  label: string
  note?: string
  error?: string
  children: ReactNode
}): ReactElement {
  // The label is the section's accessible name, so every control is announced
  // by what it is rather than by a pile of unlabelled inputs.
  const id = useId()
  return (
    <section className="ctl" aria-labelledby={id}>
      <div className="ctl-head">
        <span className="ctl-label" id={id}>
          {props.label}
        </span>
      </div>
      {props.note ? <p className="ctl-value">{props.note}</p> : null}
      {props.children}
      {props.error ? (
        <p className="ctl-error" role="alert">
          {props.error}
        </p>
      ) : null}
    </section>
  )
}

/**
 * FactList / FactRow are a DETAIL PANE, ported from menumate's pack detail
 * (App/UI/MenuHubPanels.swift). The reference is the one file in that tree
 * whose layout is a set of stated facts about a thing rather than a form for
 * changing it, which is what a summary screen and an extension detail both
 * are:
 *
 *   MenuHubPanels.swift:620-633  lockRow — the row. A LABEL in a fixed 70pt
 *     column, right-aligned, beside a value drawn as a read-only field, with a
 *     lock glyph after it and the whole row at opacity 0.7.
 *   MenuHubPanels.swift:27-45  PvField — the same 70pt right-aligned label
 *     column, and the row's optional `hint`: a second, subordinate line under
 *     the value at 11pt in the third text tone, 3pt below it.
 *   DesignSystem.swift:451-482  MMField — what the value is drawn as. A sunken
 *     field background, 9x4 padding, the control corner radius and a hairline
 *     stroke; 13pt, and `.monospaced` design when the value is a token.
 *   DesignSystem.swift:519-537  Badge — the small semibold capsule a state
 *     word is drawn in, beside what it qualifies at 7pt spacing (the shape the
 *     panel's own header uses at :641-645).
 *   MenuHubPanels.swift:637,657 the two spacings, and why they differ: 13
 *     between the pane's LEVELS (title, description, rows) and 10 between the
 *     ROWS inside the level. One uniform gap everywhere is what makes a pane
 *     read as a wall.
 *   DesignSystem.swift:576-582  the footer: one 11.5pt line under the rows.
 *
 * The one thing the port has to be faithful about is `mono`. lockRow takes it
 * as a flag and the caller sets it per row, and the caller sets it for exactly
 * one reason: whether the value is an identifier. The UTI row is monospaced
 * because "public.image" is a token; the target row is not, because "files
 * only" is words. That is a rule about legibility — a reader can tell from the
 * face alone that a value is machine vocabulary rather than something a person
 * chose to name something — and it is why `mono` here is not a style option a
 * caller passes to taste. A human name never gets it.
 *
 * No code was copied: the reference is SwiftUI and this is React over a
 * declared schema. The row shape, the two spacings, the label alignment and
 * the mono rule transfer; the drawing does not. Tracked in
 * third_party/menumate/ATTRIBUTION.md.
 */

/** The lock glyph, standing in for the SF Symbol `lock.fill` the reference
 *  draws at the end of every fact row.
 *
 *  It was U+1F512 plus U+FE0E, relying on the text-presentation selector to
 *  make the webview draw it monochrome. Chromium ignored the selector and
 *  painted a full-colour yellow padlock — the only saturated non-accent colour
 *  in a window that is otherwise three greys and one blue, sitting at the end of
 *  every row on the summary screen. An inline SVG has no such negotiation with
 *  the platform emoji font: it is `currentColor` and the hairline it draws is
 *  the same hairline every other mark here is. */
const LOCK = (
  <svg viewBox="0 0 12 14" width="10" height="12" aria-hidden="true">
    <rect x="1.5" y="6.5" width="9" height="6.5" rx="1.5" fill="none" stroke="currentColor" strokeWidth="1.2" />
    <path d="M3.75 6.5V4.25a2.25 2.25 0 0 1 4.5 0V6.5" fill="none" stroke="currentColor" strokeWidth="1.2" />
  </svg>
)

export function FactList(props: { children: ReactNode }): ReactElement {
  return <div className="ctl-facts">{props.children}</div>
}

export function FactRow(props: {
  /** The row's key. It names the fact, so a reader never has to know the value
   *  to know what is being read. */
  label: string
  /** True only when `children` is an identifier — an id, a path, a version. */
  mono?: boolean
  /** The row's state word, drawn as a badge beside what it qualifies. */
  badge?: string
  /** A subordinate line under the value: the consequence, the reason, the
   *  permission to grant. Never a second value. */
  hint?: ReactNode
  children: ReactNode
}): ReactElement {
  return (
    <div className="ctl-fact">
      <span className="ctl-fact-key">{props.label}</span>
      <div className="ctl-fact-body">
        {props.badge ? <span className="ctl-chip">{props.badge}</span> : null}
        <span className={props.mono ? 'ctl-fact-value is-mono' : 'ctl-fact-value'}>
          {props.children}
        </span>
        {/* The lock is what says these are stated facts, so a reader can tell
            a fact from a control without trying. Decoration, never the signal:
            the state word beside it is the signal. */}
        <span className="ctl-fact-lock" aria-hidden="true">
          {LOCK}
        </span>
      </div>
      {props.hint ? <span className="ctl-fact-hint">{props.hint}</span> : null}
    </div>
  )
}

/** The one line under a pane's rows, in the reference's footer register. */
export function FactFoot(props: { children: ReactNode }): ReactElement {
  return <div className="ctl-facts-foot">{props.children}</div>
}

/**
 * Toggle is a real checkbox, not a styled div. That is not a detail: a checkbox
 * is focusable, spacebar-activatable and announced as a switch by every screen
 * reader without a single ARIA attribute written by hand — all three of which a
 * div would have to re-implement and would get wrong.
 *
 * The state word sits beside the box because colour must never be the only
 * signal (§accessibility): a row that is only "greyed out" tells a colour-blind
 * user and a screen-reader user nothing about whether the rule is on.
 */
export function Toggle(props: {
  name: string
  checked: boolean
  disabled?: boolean
  /**
   * Why the box cannot be moved, for the window while it cannot.
   *
   * A disabled control that names an action and no reason reads as a control
   * that is broken. Ported from Windhawk's ModCard.tsx:441-450, whose switch
   * takes its own title explaining the block, so the box is never just greyed.
   * A `title` attribute rather than a class: it needs no stylesheet rule, every
   * existing caller is unaffected, and a caller that does not pass it renders
   * no title at all.
   */
  reason?: string
  onToggle: (next: boolean) => void
}): ReactElement {
  return (
    <input
      className="ctl-toggle"
      type="checkbox"
      checked={props.checked}
      disabled={props.disabled}
      aria-label={props.name}
      title={props.reason}
      onChange={(event) => props.onToggle(event.currentTarget.checked)}
    />
  )
}

/**
 * SelectField is the same wrapping-label trick as TextField, for the two places
 * a choice rather than a value is the honest input: picking the plugin to try,
 * and a schema property that declares an enum.
 */
export function SelectField(props: {
  label: string
  value: string
  options: { value: string; text: string }[]
  onChange: (next: string) => void
  disabled?: boolean
}): ReactElement {
  return (
    <label className="ctl-item">
      <span className="ctl-label">{props.label}</span>
      <select
        className="ctl-select"
        value={props.value}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.currentTarget.value)}
      >
        {props.options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.text}
          </option>
        ))}
      </select>
    </label>
  )
}

/**
 * TextField pairs a label with an input by wrapping rather than by id. The
 * association is then structural — it cannot drift out of sync with the text
 * the user sees — and every field in every control is keyboard reachable
 * without a single tabIndex.
 */
export function TextField(props: {
  label: string
  value: string
  onChange: (next: string) => void
  placeholder?: string
  disabled?: boolean
  type?: 'text' | 'number'
  step?: string
}): ReactElement {
  return (
    <label className="ctl-item">
      <span className="ctl-label">{props.label}</span>
      <input
        className="ctl-input"
        type={props.type ?? 'text'}
        step={props.step}
        value={props.value}
        placeholder={props.placeholder}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.currentTarget.value)}
      />
    </label>
  )
}
