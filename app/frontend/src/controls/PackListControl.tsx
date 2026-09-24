// The Explorer pack list (bead w7-frontend-remap).
//
// What this control shows, and why it is not a list of menu items.
//
// The Explorer page declares source core:finderPacks — the Finder extpacks, each
// carrying the menu actions its manifest declares (core/pkg/plugin/pack.go:
// PackManifest → Actions). Nothing loads those manifests at runtime: there is
// no pack loader on the serving path, the builtin plugins are compiled straight
// from the rule table, and the daemon's own pluginMeta handler says so in the
// `reason` it sends with every row.
//
// So the rows here are the INSTALLED EXTENSIONS — PluginMeta, the registry that
// does answer — and each row carries the daemon's own reason rather than a
// prettified guess at what a pack would contain. A pack list that invented menu
// items would be the one thing worse than an empty one: a Finder menu the user
// can see and click that runs nothing.
//
// The page's own boundary note is respected rather than blurred: finder.go says
// the generic extension install/enable lifecycle belongs to the Extensions page
// and is NOT duplicated here. So this control lists and reports; the per-action
// writes it declares (pack.enableAction and the rest) are dispatched through the
// action registry, which refuses them in words naming the call they wait on —
// the same refusal every other unbound action gets, and the reason a row does
// not carry buttons that would silently do nothing.
//
// An empty registry reads as EMPTY. "No extension packs are installed" is a
// sentence about the machine, and it is what someone who has installed nothing
// should see; it is never an error, because nothing failed.
//
// Port source: menumate (Hibrielle/menumate), whose menu-hub screen draws a
// section cap over a list in which every entry is handed to its own row, rather
// than summarising the entries into the one above them.
//
//   App/UI/MenuHubScreen.swift:183-188, :246-247  a ForEach in which every
//     entry becomes its own row via actionRow
//
// No code was copied: the reference is a SwiftUI app computing live menu state
// from an extension manager, and this is React over a declared schema reading
// the daemon's registry, so the one-row-per-extension layout transfers and the
// drawing does not. Tracked in third_party/menumate/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { PluginMetaRow } from '../types/controls'
import { failedTo } from '../lib/wire'
import { humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

export function PackListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const packs = useResource(() => ctx.service.PluginMeta(), ctx.refreshToken, [], ctx.note)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const rows: PluginMetaRow[] = packs.data ?? []

  async function run(action: string, id: string, label: string): Promise<void> {
    const command = commandFor(action)
    if (!command) {
      const message = `"${action}" is declared on this page, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(`${action}/${id}`)
    setError('')
    try {
      await command(ctx.service, { id })
      ctx.note(`${label} done.`)
      packs.reload()
      ctx.refresh()
    } catch (reason) {
      // Every pack action is an unbound capability today, so the refusal is the
      // expected answer and it names the call it is waiting for rather than
      // reading as a fault on the machine.
      const message = failedTo(label, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  const actions = Array.isArray(control.rowActions) ? control.rowActions : []

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || packs.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No extension packs are installed. Ones you install appear here.</EmptyState>
      ) : (
        <>
          <p className="ctl-value">
            These are the installed extensions. Their menu items come from a pack manifest, and
            none is loaded yet — each row says so below.
          </p>
          <ul className="ctl-list">
            {rows.map((row) => (
              <li className="ctl-item" key={row.id}>
                <span className="ctl-label">{row.name || humanize(row.id)}</span>
                <span className="ctl-value">{row.version ? `Version ${row.version}` : 'No version in a manifest'}</span>
                <span className="ctl-value">
                  {row.loaded ? 'Loaded' : 'Not loaded'}
                  {row.reason ? ` · ${row.reason}` : ''}
                </span>
                <span className="ctl-value">
                  {row.permissions.length > 0
                    ? `May use: ${row.permissions.join(', ')}`
                    : 'No permissions requested.'}
                </span>
                {actions.length > 0 ? (
                  <div className="ctl-actions">
                    {actions.map((action) => (
                      <button
                        key={action}
                        type="button"
                        className="ctl-input"
                        disabled={busy !== ''}
                        onClick={() => void run(action, row.id, `${action} on ${row.id}`)}
                      >
                        {busy === `${action}/${row.id}`
                          ? 'Working…'
                          : `${humanize(action.split('.').pop() ?? action)} ${humanize(row.id)}`}
                      </button>
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        </>
      )}
    </ControlFrame>
  )
}
