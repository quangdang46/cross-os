// Row furniture shared by every control renderer (bead cross-os-itq).
//
// The class names used here are the entire styling vocabulary the shell
// stylesheet defines: ctl, ctl-head, ctl-label, ctl-value, ctl-input,
// ctl-select, ctl-slider, ctl-toggle, ctl-list, ctl-item, ctl-chip, ctl-empty,
// ctl-error, ctl-off, ctl-actions, ctl-countdown. A control that needs a look it cannot
// get from these is a control whose design has not been decided yet, and
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
  onToggle: (next: boolean) => void
}): ReactElement {
  return (
    <input
      className="ctl-toggle"
      type="checkbox"
      checked={props.checked}
      disabled={props.disabled}
      aria-label={props.name}
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
