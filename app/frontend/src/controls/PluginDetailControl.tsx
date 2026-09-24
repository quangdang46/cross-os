// The extension detail (bead w6-frontend-setup-renderers).
//
// What an extension is allowed to do, in the extension's own words: the
// manifest's name and version, the permissions its rules are authorized
// against, and whether the daemon loaded it. Those are the three questions a
// person asks before deciding to trust a shortcut, and the daemon already holds
// all three answers (core:pluginMeta) — so this control reads them rather than
// inferring a display name from an id.
//
// Name and Version are NOT defaulted to the id. The builtin plugins are
// compiled straight from the rule table and ship no manifest, so both are
// empty today and the row carries the reason why; a shell that printed the id
// in the name field would be claiming a manifest exists.
//
// The permissions are the GRANT vocabulary the router authorizes those rules
// against, not a list this shell collected: a permission shown here that the
// daemon does not enforce would be a promise the router does not keep, and one
// missing is a rule that silently cannot fire.
//
// Which plugin is shown is the control's declaration (`plugin`), so one page
// draws as many detail cards as it declares and a page that names none draws
// them all. That is the discovery rule at card level: the id is data.
//
// Port source: Karabiner-Elements (pqrs-org/Karabiner-Elements) — the editor
// flow that pairs a list of what is installed with the detail of the one being
// read:
//
//   src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:9-21
//     a fixed-width selector column beside the detail pane, so the item being
//     read is shown in full without leaving the list
//   src/apps/SettingsWindow/src/View/ProfileEditView.swift:12-18
//     the detail side of that split: the selected item's own fields, each as a
//     label beside its value row
//
// No code was copied: the reference is SwiftUI and this is React over a
// declared schema, so the list-beside-detail shape and the one-field-per-row
// rule transfer and the drawing does not. Tracked in
// third_party/Karabiner-Elements/ATTRIBUTION.md.

import type { ReactElement } from 'react'
import { humanize, plural } from '../lib/format'
import { asText } from '../lib/wire'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function PluginDetailControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const meta = useResource(() => ctx.service.PluginMeta(), ctx.refreshToken, [], ctx.note)

  const wanted = asText(control.plugin)
  const all = meta.data ?? []
  // No plugin named means the page wants them all: an empty card that names no
  // extension is a hole, and picking "all" is the only reading that fills it.
  const shown = wanted ? all.filter((row) => row.id === wanted) : all

  return (
    <ControlFrame
      label={control.label ?? (wanted ? `${humanize(wanted)} details` : 'Extension details')}
      note={control.note}
      error={meta.error}
    >
      {shown.length === 0 ? (
        <EmptyState>
          {wanted
            ? 'The daemon reported no manifest for this extension.'
            : 'The daemon reported no manifests.'}
        </EmptyState>
      ) : (
        <ul className="ctl-list">
          {shown.map((row) => (
            <li className="ctl-item is-block" key={row.id}>
              <span className="ctl-label">{row.name || humanize(row.id)}</span>
              <span className="ctl-chip">{row.loaded ? 'Loaded' : 'Not loaded'}</span>
              <span className="ctl-value">
                {row.version ? `Version ${row.version}` : 'No version in the manifest'}
                {row.reason ? ` — ${row.reason}` : ''}
              </span>
              {row.permissions.length === 0 ? (
                <span className="ctl-value">No permissions requested.</span>
              ) : (
                <span className="ctl-value">
                  {plural(row.permissions.length, 'permission')}: {row.permissions.join(', ')}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </ControlFrame>
  )
}
