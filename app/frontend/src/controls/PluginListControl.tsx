// The installed-plugin list (bead cross-os-itq; detail added by
// w6-frontend-setup-renderers).
//
// The rows are the daemon's own status payload, which App.tsx already keeps
// fresh — so this fetches nothing of its own. Health is shown as a word, not a
// dot: "degraded" and "disabled" mean different things to a user deciding
// whether to trust a shortcut, and a coloured circle cannot say which.
//
// The toggle calls TogglePlugin, which fails closed on an unknown id
// (bridge.go) and returns the error rather than a silent no-op — so a rejected
// toggle here says so instead of leaving the box looking like it worked.
//
// Opening a row shows what the extension may do, read from the daemon's own
// manifest facts (PluginDetailControl) rather than guessed from the id. The
// focused id is the only state here, and it names nothing the daemon did not
// send: an extension that uninstalls simply stops having a detail, because the
// row it belonged to is gone with it.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { Control } from '../types/controls'
import { failedTo } from '../lib/wire'
import { humanize } from '../lib/format'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'
import { PluginDetailControl } from './PluginDetailControl'

export function PluginListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const [focused, setFocused] = useState('')
  const plugins = ctx.status?.Plugins ?? []

  async function toggle(id: string, next: boolean): Promise<void> {
    setPending(id)
    setError('')
    try {
      await ctx.service.TogglePlugin(id, next)
      ctx.note(`${id} ${next ? 'enabled' : 'disabled'}.`)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`${next ? 'Enabling' : 'Disabling'} ${id}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setPending('')
    }
  }

  // The detail is a control of its own kind, so the card the person opens is
  // drawn by the same renderer a page would get for a pluginDetail control. The
  // object is assembled from the row the daemon sent, which is why nothing in it
  // is a page id or a plugin id written here.
  const detail: Control | null = focused
    ? { kind: 'pluginDetail', id: focused, label: `${humanize(focused)} — what it may do`, plugin: focused }
    : null

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error}>
      {plugins.length === 0 ? (
        <EmptyState>No plugins are installed yet.</EmptyState>
      ) : (
        <>
          <ul className="ctl-list">
            {plugins.map((plugin) => (
              <li className="ctl-item" key={plugin.ID}>
                <button
                  type="button"
                  className="ctl-link"
                  aria-expanded={focused === plugin.ID}
                  onClick={() => setFocused(focused === plugin.ID ? '' : plugin.ID)}
                >
                  {humanize(plugin.ID)}
                </button>
                <span className="ctl-chip">{plugin.Healthy || 'unknown health'}</span>
                <span className="ctl-value">{plugin.Enabled ? 'Enabled' : 'Disabled'}</span>
                <Toggle
                  name={`${plugin.Enabled ? 'Disable' : 'Enable'} ${plugin.ID}`}
                  checked={plugin.Enabled}
                  disabled={pending === plugin.ID}
                  onToggle={(next) => void toggle(plugin.ID, next)}
                />
              </li>
            ))}
          </ul>
          {detail ? <PluginDetailControl control={detail} ctx={ctx} /> : null}
        </>
      )}
    </ControlFrame>
  )
}
