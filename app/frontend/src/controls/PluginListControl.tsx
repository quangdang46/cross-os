// The installed-plugin list (bead cross-os-itq; detail added by
// w6-frontend-setup-renderers).
//
// The rows are the daemon's own status payload, which App.tsx already keeps
// fresh — so this fetches nothing of its own. The word beside the box is
// Enabled or Disabled, read from the same payload. No health chip is drawn: the
// daemon sends one literal for every id while no supervisor runs, and a chip
// beside the word is a claim nobody measured (see the note on the group label
// below, and main.go handlePluginList).
//
// A row's write is the action the page declared for it, resolved through the
// action registry and refused in words when nothing is bound — never a switch
// that moves and changes nothing. What it lands on fails closed on an unknown id
// (bridge.go) and returns the error rather than a silent no-op, so a rejected
// toggle says so instead of leaving the box looking like it worked.
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
import { commandFor } from './actions'
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

/**
 * The one action a row may take, or null when it may take none.
 *
 * A row is a target for exactly the action that moves it, and only when the
 * page declared that action and this build has a command for it — the two
 * conditions the schema and the registry already carry. The label and the
 * request below are both read off this one value rather than written out
 * twice, which is the whole point: a switch whose name and effect could
 * disagree is a switch that moves and changes nothing.
 *
 * Returns null rather than a disabled control, so a page that cannot do this
 * draws no button instead of a dead one.
 *
 * Ported from Windhawk's actionTargets (mods-browser/selection/selectionActions.ts:31-47)
 * and modDetailsState's `can` / `mod` clauses (mod-details/modDetailsState.ts:96-100, :180-188).
 * Structure and behaviour only; no code copied.
 */
export function rowActionsFor(
  plugin: PluginState,
  declared: string[],
): { action: 'plugin.enable' | 'plugin.disable' | null; label: string } {
  const wanted = plugin.Enabled ? 'plugin.disable' : 'plugin.enable'
  const verb = plugin.Enabled ? 'Disable' : 'Enable'
  const label = `${verb} ${plugin.ID}`
  if (!declared.includes(wanted) || commandFor(wanted) === undefined) {
    return { action: null, label }
  }
  return { action: wanted, label }
}

export function PluginListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const [focused, setFocused] = useState('')
  const plugins = ctx.status?.Plugins ?? []

  async function toggle(id: string, next: boolean, action: string): Promise<void> {
    setPending(id)
    setError('')
    try {
      // The write the page declared, resolved the way every other declared row
      // write in this shell is (FinderMenuControl.tsx:242-248). A row whose
      // action resolves to nothing is not drawn at all, so reaching here means
      // a command exists; the refusal branch stays as the daemon's own answer.
      const command = commandFor(action)
      if (!command) {
        const message = `${next ? 'Enabling' : 'Disabling'} ${id} did not go through: this build has no command for ${action}.`
        setError(message)
        ctx.note(message)
        return
      }
      await command(ctx.service, { id, enabled: next })
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
  // The writes this page declared for its rows, read through the index
  // signature: a control the daemon declared none for is not guessing one.
  const declaredRowActions = Array.isArray(control.rowActions) ? control.rowActions : []
  const metaFor = (id: string): PluginMetaRow | undefined =>
    (meta.data ?? []).find((row) => row.id === id)

  // The detail is drawn only while the row it names is on the list. A refresh
  // that no longer serves it takes the detail with it, rather than leaving a
  // card open under a row that is gone — the same rule Windhawk's selection
  // follows at useModSelection.ts:39-46, resolved during render so the frame
  // on screen already agrees with it.
  const open: PluginState | undefined = plugins.find((plugin) => plugin.ID === focused)
  const detail: Control | null = open
    ? {
        kind: 'pluginDetail',
        id: open.ID,
        label: `${humanize(open.ID)} — what it may do`,
        plugin: open.ID,
      }
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
        // One sentence, because this list has exactly one way to be empty. The
        // moment a filter is ever added, a list the filter emptied has to say
        // so in DIFFERENT words: "you have none" and "your filter hid the one
        // you had" are not the same sentence, and collapsing them is how a
        // person concludes they removed something they never had. Recorded
        // here so that change cannot quietly drop it — there is no filter, and
        // no marketplace to browse, so this is the whole of it today.
        <EmptyState>No plugins are installed yet.</EmptyState>
      ) : (
        <>
          {groups.map((group) => (
            <section key={group.origin} aria-label={group.label}>
              <p className="ctl-value">{group.label}</p>
              <ul className="ctl-list">
                {group.rows.map((plugin) => {
                  // One value, read once: the box's name and the write it issues
                  // are the same answer, so a switch cannot claim to do
                  // something different from what it does.
                  //
                  // A write in flight is the one window this box is disabled,
                  // and it says nothing about itself there: a disabled control
                  // that names an action and no reason reads as one that is
                  // broken. The slot for that is a title on the shared Toggle
                  // (common.tsx), which this control does not own.
                  const { action, label } = rowActionsFor(plugin, declaredRowActions)
                  const inFlight = pending === plugin.ID
                  return (
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
                      {action === null ? null : (
                        <Toggle
                          name={label}
                          checked={plugin.Enabled}
                          disabled={inFlight}
                          // Why it cannot be moved, for as long as it cannot.
                          // A box that is greyed with no reason reads as a
                          // broken control, which is the same class of thing as
                          // the health chip this list used to draw.
                          reason={
                            inFlight
                              ? `${label.replace(/^(Enable|Disable)/, '$1ing')}…`
                              : undefined
                          }
                          onToggle={(next) => void toggle(plugin.ID, next, action)}
                        />
                      )}
                    </li>
                  )
                })}
              </ul>
            </section>
          ))}
          {detail ? <PluginDetailControl control={detail} ctx={ctx} /> : null}
        </>
      )}
    </ControlFrame>
  )
}
