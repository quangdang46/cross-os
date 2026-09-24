// The installed-plugin list (bead cross-os-itq).
//
// The rows are the daemon's own status payload, which App.tsx already keeps
// fresh — so this fetches nothing of its own. Health is shown as a word, not a
// dot: "degraded" and "disabled" mean different things to a user deciding
// whether to trust a shortcut, and a coloured circle cannot say which.
//
// The toggle calls TogglePlugin, which fails closed on an unknown id
// (bridge.go) and returns the error rather than a silent no-op — so a rejected
// toggle here says so instead of leaving the box looking like it worked.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { failedTo } from '../lib/wire'
import { humanize } from '../lib/format'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

export function PluginListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
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

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={error}>
      {plugins.length === 0 ? (
        <EmptyState>No plugins are installed yet.</EmptyState>
      ) : (
        <ul className="ctl-list">
          {plugins.map((plugin) => (
            <li className="ctl-item" key={plugin.ID}>
              <span className="ctl-label">{humanize(plugin.ID)}</span>
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
      )}
    </ControlFrame>
  )
}
