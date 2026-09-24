// The landing card (bead w6-frontend-setup-renderers).
//
// One card that answers "what is CrossOS doing right now", built from the three
// answers the daemon already serves and NOTHING it fetches of its own beyond
// the two rows that have no other home:
//
//   - the status payload the shell keeps fresh (App.tsx owns that poll), so the
//     card can never say "running" while the masthead above it says stopped;
//   - the profile list, so the active bundle is named rather than implied by
//     which switches happen to be on;
//   - the readiness rollup, because "running" and "usable" are different
//     questions and the card is the one place both belong.
//
// A landing card with a source of its own would be the first place this window
// could show two answers to the same question, which is the failure the whole
// discovery design exists to prevent (§3.6c).
//
// Port source: Amethyst (ianyh/Amethyst), the window manager whose settings are
// a list of facts about the machine rather than a list of knobs.
//
//   .amethyst.sample.yml:255-262   each window fact is one line carrying its own
//                                  explanation, so a reader never has to know
//                                  the key to know what the value means
//   .amethyst.sample.yml:271-275   the per-application list beside the global
//                                  facts: the rollup, then the items
//
// No code was copied: the reference is a YAML file a person edits and this is a
// read-only card the daemon fills, so the layout and the "say what the value
// MEANS" rule transfer and the implementation does not. Tracked in
// third_party/Amethyst/ATTRIBUTION.md.

import type { ReactElement } from 'react'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/**
 * extensionSentence is the count, in words. "1 of 2" on its own is a number a
 * person has to interpret; the sentence says what it means, which is the rule
 * this whole card follows and the one Amethyst's settings file already keeps.
 */
function extensionSentence(total: number, on: number): string {
  if (total === 0) return 'No extensions are installed yet.'
  if (on === total) return 'Every installed extension is switched on.'
  return `${plural(total - on, 'extension')} switched off.`
}

export function HomeSummaryControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const profiles = useResource(() => ctx.service.Profiles(), ctx.refreshToken, [], ctx.note)
  const readiness = useResource(() => ctx.service.Readiness(), ctx.refreshToken, [], ctx.note)

  const status = ctx.status
  const plugins = status?.Plugins ?? []
  const enabled = plugins.filter((plugin) => plugin.Enabled)
  const rows = readiness.data ?? []
  const ready = rows.filter((row) => row.ready)
  const failing = rows.filter((row) => !row.ready)
  const active = (profiles.data ?? []).find((profile) => profile.active)

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={profiles.error || readiness.error}
    >
      {/* The daemon's own words first. A status the shell wrote would be a
          second answer to the running question, and this is the page a person
          lands on after the wizard. */}
      <ul className="ctl-list">
        <li className="ctl-item">
          <span className="ctl-chip">{status ? (status.Running ? 'Running' : 'Stopped') : 'Connecting'}</span>
          <span className="ctl-label">Daemon</span>
          <span className="ctl-value">
            {status?.Version ? `CrossOS ${status.Version}` : 'The shell has not heard from the daemon yet.'}
            {status?.SafeMode ? ' Safe mode is on, so nothing acts until you say so.' : ''}
            {status?.Killed ? ' PANIC STOP is latched.' : ''}
          </span>
        </li>
        <li className="ctl-item">
          <span className="ctl-chip">
            {status ? (status.Interception ? 'On' : 'Off') : 'Unknown'}
          </span>
          <span className="ctl-label">Keyboard interception</span>
          <span className="ctl-value">
            {status?.Interception
              ? 'CrossOS can read the keyboard and act on it.'
              : 'Shortcuts do nothing until interception is installed.'}
            {/* The tap error is the line that teaches the user WHICH permission to
                grant, so it is quoted here verbatim rather than summarised. */}
            {status?.TapError ? ` ${status.TapError}` : ''}
          </span>
        </li>
        <li className="ctl-item">
          <span className="ctl-chip">{active ? 'Active' : 'None'}</span>
          <span className="ctl-label">Profile</span>
          <span className="ctl-value">
            {active
              ? active.label
              : 'No profile is active, so shortcuts run one at a time as you set them.'}
          </span>
        </li>
        <li className="ctl-item">
          <span className="ctl-chip">
            {plugins.length === 0 ? 'None' : `${enabled.length} of ${plugins.length}`}
          </span>
          <span className="ctl-label">Extensions</span>
          <span className="ctl-value">{extensionSentence(plugins.length, enabled.length)}</span>
        </li>
      </ul>

      {profiles.loading || readiness.loading ? <p className="ctl-value">Reading the daemon…</p> : null}

      {rows.length === 0 && !readiness.loading ? (
        <EmptyState>The daemon reported no readiness checks, so this card cannot say what is usable.</EmptyState>
      ) : null}

      {rows.length > 0 ? (
        <p className="ctl-value">
          {failing.length === 0
            ? `All ${plural(rows.length, 'check')} are ready.`
            : `${plural(ready.length, 'check')} of ${rows.length} ready. To do: ${failing
                .map((row) => row.label || humanize(row.id))
                .join(', ')}.`}
        </p>
      ) : null}
    </ControlFrame>
  )
}
