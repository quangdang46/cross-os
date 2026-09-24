// Plugin settings rendered from their own config_schema (bead cross-os-itq,
// plan §3.6c).
//
// This is the control the whole contribution model exists for: a plugin that
// ships a new config_schema gets a settings form here with no change to Core
// and none to the shell. The widget tier is the MVP one pages.go names —
// checkbox, select, slider — chosen from the JSON Schema the plugin declared,
// and anything outside that tier says so in words instead of being guessed at.
// Guessing is how a `type: "array"` property turns into a free-text box that
// silently writes a string where the plugin expects a list.
//
// The values are the honest limit of this build. core.pluginSchemas serves the
// SCHEMA, not what the user currently has saved, and no bound method writes a
// plugin's config — so the widgets are drawn disabled under one plain sentence
// per form. Drawing them live would mean inventing a starting value, and a
// checkbox that appears unchecked because the shell had nothing to check is a
// lie the user acts on.

import type { ReactElement, ReactNode } from 'react'
import { asList, asNumber, asRecord, asText } from '../lib/wire'
import { humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/** Describes one widget choice, or the reason there is none. */
type Widget =
  | { kind: 'checkbox' }
  | { kind: 'select'; options: string[] }
  | { kind: 'slider'; min: number; max: number; step: number }
  | { kind: 'none'; because: string }

function widgetFor(spec: Record<string, unknown>): Widget {
  const type = asText(spec.type)
  if (type === 'boolean') return { kind: 'checkbox' }

  if (type === 'string') {
    const options = asList(spec.enum).map((entry) => asText(entry)).filter((entry) => entry !== '')
    if (options.length > 0) return { kind: 'select', options }
  }

  if (type === 'number' || type === 'integer') {
    const min = asNumber(spec.minimum)
    const max = asNumber(spec.maximum)
    if (min !== null && max !== null && max > min) {
      // A step of 1 on a range that spans 0.1 would quantise the user's choice
      // away; an integer schema gets whole numbers back.
      return { kind: 'slider', min, max, step: type === 'integer' ? 1 : (max - min) / 100 }
    }
  }

  return {
    kind: 'none',
    because: type === '' ? 'it declares no type' : `it is a ${type} setting`,
  }
}

function titleFor(name: string, spec: Record<string, unknown>): string {
  return asText(spec.title) || humanize(name)
}

function describe(spec: Record<string, unknown>): ReactNode {
  const described = asText(spec.description)
  const fallback = asText(spec.default)
  const parts: string[] = []
  if (described) parts.push(described)
  if (fallback) parts.push(`default: ${fallback}`)
  if (spec.enum !== undefined) parts.push(`one of: ${asList(spec.enum).map((e) => asText(e)).join(', ')}`)
  const minimum = asNumber(spec.minimum)
  const maximum = asNumber(spec.maximum)
  if (minimum !== null || maximum !== null) {
    parts.push(`range: ${minimum ?? 'any'} to ${maximum ?? 'any'}`)
  }
  return parts.length > 0 ? parts.join(' · ') : ''
}

function PropertyRow(props: { name: string; spec: Record<string, unknown> }): ReactElement {
  const { name, spec } = props
  const title = titleFor(name, spec)
  const widget = widgetFor(spec)
  const fallback = asNumber(spec.default)

  return (
    <li className="ctl-item">
      <span className="ctl-label">{title}</span>
      {widget.kind === 'checkbox' ? (
        <input className="ctl-toggle" type="checkbox" disabled aria-label={`${title} (value not served yet)`} />
      ) : null}
      {widget.kind === 'select' ? (
        <select className="ctl-select" disabled defaultValue="" aria-label={`${title} (value not served yet)`}>
          <option value="">not selected</option>
          {widget.options.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
      ) : null}
      {widget.kind === 'slider' ? (
        <input
          className="ctl-slider"
          type="range"
          min={widget.min}
          max={widget.max}
          step={widget.step}
          value={fallback !== null && fallback >= widget.min && fallback <= widget.max ? fallback : widget.min}
          disabled
          aria-label={`${title} (value not served yet)`}
        />
      ) : null}
      {widget.kind === 'none' ? (
        <span className="ctl-empty">Not editable here — {widget.because}.</span>
      ) : null}
      {describe(spec) ? <span className="ctl-value">{describe(spec)}</span> : null}
    </li>
  )
}

export function SchemaFormControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const schemas = useResource(() => ctx.service.PluginSchemas(), ctx.refreshToken, [], ctx.note)
  const rows = schemas.data ?? []

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={schemas.error}>
      {rows.length === 0 ? (
        <EmptyState>No plugin declares any settings.</EmptyState>
      ) : (
        rows.map((row) => {
          const schema = asRecord(row.schema) ?? {}
          const properties = asRecord(schema.properties) ?? {}
          const names = Object.keys(properties)
          return (
            <section className="ctl" key={row.plugin} aria-label={row.title || row.plugin}>
              <div className="ctl-head">
                <span className="ctl-label">{row.title || humanize(row.plugin)}</span>
              </div>
              <p className="ctl-value">
                These are the settings {humanize(row.plugin)} declares. The bridge does not serve
                the saved values yet, so the form shows the shape and not what is stored.
              </p>
              {names.length === 0 ? (
                <EmptyState>{humanize(row.plugin)} declares no settings.</EmptyState>
              ) : (
                <ul className="ctl-list">
                  {names.map((name) => (
                    <PropertyRow key={name} name={name} spec={asRecord(properties[name]) ?? {}} />
                  ))}
                </ul>
              )}
            </section>
          )
        })
      )}
      {schemas.loading ? <p className="ctl-value">Loading…</p> : null}
    </ControlFrame>
  )
}
