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
import { useResource } from '../lib/useResource'
import type { ReactElement } from 'react'
import type { Control, PluginMetaRow, PluginState } from '../types/controls'
import { failedTo } from '../lib/wire'
import { humanize } from '../lib/format'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'
import { PluginDetailControl } from './PluginDetailControl'

/** What a person reads about where a plugin came from. */
const ORIGIN_WORDS: Record<string, string> = {
  builtin:
    'Built into CrossOS. These are compiled from the rule table, so there is no separate process to install, remove, or crash.',
}

/** The served rows, grouped by origin, in the order the origins first appear. */
function groupBy(plugins: PluginState[]): { origin: string; label: string; rows: PluginState[] }[] {
  const order: string[] = []
  const byOrigin = new Map<string, PluginState[]>()
  for (const plugin of plugins) {
    const origin = plugin.Origin || 'unknown'
    if (!byOrigin.has(origin)) {
      byOrigin.set(origin, [])
      order.push(origin)
    }
    byOrigin.get(origin)!.push(plugin)
  }
  return order.map((origin) => ({
    origin,
    label: ORIGIN_WORDS[origin] ?? `From ${origin}.`,
    rows: byOrigin.get(origin)!,
  }))
}

/**
 * One line of facts, or the sentence that there are none.
 *
 * The facts are whatever the daemon loaded: a name and version when a manifest
 * was read, and the reason it gave when one was not. The reason is quoted
 * rather than prettified, because it is the daemon's own account of why a row
 * is the shape it is.
 */
function metaLine(row: PluginMetaRow | undefined): string {
  if (row === undefined) return 'No manifest has been loaded for this one.'
  const named = [row.name, row.version].filter((part) => part !== '').join(' ')
  if (named !== '') return `${named} — ${row.loaded ? 'loaded' : 'not loaded'}`
  return row.reason !== undefined && row.reason !== ''
    ? row.reason
    : 'No manifest has been loaded for this one.'
}

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
  // Grouped by where each plugin came from, the structure pi's resource
  // registry is built on (config-selector.ts:57-64, ResourceGroup carrying
  // scope and origin). A person reading a list mixing compiled-in parts with
  // installed ones cannot tell them apart from the id, and the two behave
  // differently: a built-in cannot be removed, only switched off.
  //
  // The health chip that stood here is gone, and that is the other half of the
  // same port. The daemon sent the literal string "healthy" for every id while
  // no supervisor ran and no plugin was a child process (main.go,
  // handlePluginList) — so the chip asserted something nobody had measured, in
  // the same word the trial gate uses to mean "I checked". It has been removed
  // from this list rather than restated: a registry that repeats a claim it
  // cannot verify is decoration, and the Enabled word beside the toggle is the
  // fact a person can act on.
  // One line of facts per row, and a line that says so when there are none.
  //
  // Ported from Windhawk's ModCard, which draws its facts on a
  // ModMetadataLine with singleLine set (:344-350) and falls back to an
  // explicit italic "no description" rather than leaving the slot blank
  // (:357). Both halves matter here, because the daemon genuinely has nothing
  // to say for most of these rows today: the plugin-meta source loads no manifest, so
  // name and version come back empty. A card that quietly carried no facts
  // read as a card with none to carry, which is the same class of thing as the
  // health chip — an empty slot is read as a fact.
  const meta = useResource(() => ctx.service.PluginMeta(), ctx.refreshToken, [], ctx.note)
  const groups = groupBy(plugins)
  const metaFor = (id: string): PluginMetaRow | undefined =>
    (meta.data ?? []).find((row) => row.id === id)

  const detail: Control | null = focused
    ? { kind: 'pluginDetail', id: focused, label: `${humanize(focused)} — what it may do`, plugin: focused }
    : null

  return (
    <ControlFrame
        label={control.label ?? control.id}
        note={control.note}
        // Both reads are named on the frame: a source that failed has to say so
        // rather than leaving the row looking like it simply has no facts.
        error={error || meta.error}
      >
      {plugins.length === 0 ? (
        <EmptyState>No plugins are installed yet.</EmptyState>
      ) : (
        <>
          {groups.map((group) => (
            <section key={group.origin} aria-label={group.label}>
              <p className="ctl-value">{group.label}</p>
              <ul className="ctl-list">
                {group.rows.map((plugin) => (
                  <li className="ctl-item" key={plugin.ID}>
                    <button
                      type="button"
                      className="ctl-link"
                      aria-expanded={focused === plugin.ID}
                      onClick={() => setFocused(focused === plugin.ID ? '' : plugin.ID)}
                    >
                      {humanize(plugin.ID)}
                    </button>
                    <span className="ctl-value">{plugin.Enabled ? 'Enabled' : 'Disabled'}</span>
                    <span className="ctl-value">{metaLine(metaFor(plugin.ID))}</span>
                    <Toggle
                      name={`${plugin.Enabled ? 'Disable' : 'Enable'} ${plugin.ID}`}
                      checked={plugin.Enabled}
                      disabled={pending === plugin.ID}
                      onToggle={(next) => void toggle(plugin.ID, next)}
                    />
                  </li>
                ))}
              </ul>
            </section>
          ))}
          {detail ? <PluginDetailControl control={detail} ctx={ctx} /> : null}
        </>
      )}
    </ControlFrame>
  )
}
