// One pack action's settings (bead w7-frontend-remap).
//
// THE PAGE DECLARES THE VOCABULARY; THE DAEMON SERVES THE VALUES. The Explorer
// page names its editable fields (placement, variants, timeoutSeconds) and its
// read-only ones (targets, utis) in the control's own schema, and this control
// draws exactly those — the page's list, in the page's order, never a list kept
// here. A settings pane that showed its own field names would be a second
// declaration of the same thing, and a pack action with a `variants` field the
// pane did not know about would have nowhere to go.
//
// The values are not served, and that is said rather than filled in. A pack
// action's placement, variants and timeout live in the pack manifest
// (core/pkg/plugin/pack.go), and no pack manifest is loaded at runtime, so there
// is no value to report. Each field therefore renders as its name beside "not
// reported" — the shell's standing rule for a value the daemon did not send
// (lib/wire.ts) — instead of an input pre-filled with a plausible default. A
// pre-filled placement would be a guess about the user's Finder menu, and a guess
// that SAVES is a guess that moves their menu.
//
// The read-only fields are separated from the editable ones because they are a
// different KIND of row, not a disabled input: targets and utis are filters the
// pack manifest declares and the runtime matches against (pack.Match), so
// editing them is not this pane's business at any state. Rendering them as
// disabled inputs would invite the question "why can't I change this", and
// answering it needs the paragraph above rather than a greyed-out box.
//
// An empty registry reads as empty, never broken: with no extension installed
// there is no action to configure, and that is a fact about the machine.
//
// Port source: rectangle (ramonwessels/rectangle), whose preferences window
// names each setting in words beside its own control rather than identifying it
// by its key.
//
//   Rectangle/PrefsWindow/SnapAreaViewController.swift:75-86  a name in words
//     beside its own control
//
// No code was copied: the reference is AppKit on macOS and this is React over a
// declared schema, so the label-beside-value row transfers and the drawing does
// not. Tracked in third_party/rectangle/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { PluginMetaRow } from '../types/controls'
import { humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, SelectField } from './common'
import type { ControlProps } from './common'

/** A value the daemon did not send, said rather than invented. */
const NOT_REPORTED = 'Not reported by the daemon'

export function ActionSettingsControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const packs = useResource(() => ctx.service.PluginMeta(), ctx.refreshToken, [], ctx.note)
  const [focused, setFocused] = useState('')
  const [error] = useState('')
  const rows: PluginMetaRow[] = packs.data ?? []

  // The field names are the page's, read from the control's own schema. A page
  // that declares none gets the vocabulary its own schema already carries
  // rather than an empty pane: these are settings a pack action can have, and
  // the daemon has not sent a value for any of them yet.
  const editable = Array.isArray(control.fields)
    ? control.fields.filter((field): field is string => typeof field === 'string')
    : []
  const readOnly = Array.isArray(control.readOnly)
    ? control.readOnly.filter((field): field is string => typeof field === 'string')
    : []

  const pack = rows.find((row) => row.id === focused) ?? null

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || packs.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No extension pack is installed, so there are no action settings to show.</EmptyState>
      ) : (
        <>
          <SelectField
            label="Pack"
            value={focused}
            options={[
              { value: '', text: 'choose a pack' },
              ...rows.map((row) => ({ value: row.id, text: row.name || humanize(row.id) })),
            ]}
            onChange={setFocused}
          />

          {pack === null ? (
            <EmptyState>Choose a pack to see what its actions can be set to.</EmptyState>
          ) : (
            <>
              <p className="ctl-value">
                {pack.reason ||
                  'These settings come from the pack manifest, and the daemon reports no value for any of them.'}
              </p>
              <ul className="ctl-list">
                {editable.map((field) => (
                  <li className="ctl-item" key={field}>
                    {/* The label is humanized and the page's own key stays
                        reachable in the title, which is the standing contract
                        for a formatter (lib/format.ts): a reader who wants the
                        exact field the daemon and the manifest both spell gets
                        it, and one who wants English gets that too. */}
                    <span className="ctl-label" title={field}>
                      {humanize(field)}
                    </span>
                    <span className="ctl-value">{NOT_REPORTED}</span>
                  </li>
                ))}
                {readOnly.map((field) => (
                  <li className="ctl-item" key={field}>
                    <span className="ctl-label" title={field}>
                      {humanize(field)}
                    </span>
                    <span className="ctl-value">Set by the pack manifest · not editable here</span>
                  </li>
                ))}
              </ul>
              {editable.length === 0 && readOnly.length === 0 ? (
                <EmptyState>
                  This page declares no settings fields, so there is nothing to show for{' '}
                  {pack.name || humanize(pack.id)}.
                </EmptyState>
              ) : null}
            </>
          )}
        </>
      )}
    </ControlFrame>
  )
}
