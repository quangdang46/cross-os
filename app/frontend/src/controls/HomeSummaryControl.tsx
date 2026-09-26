// The landing card (bead w6-frontend-setup-renderers).
//
// One card that answers "what is CrossOS doing right now", built from the three
// answers the daemon already serves and NOTHING it fetches of its own beyond
// the two rows that have no other home:
//
//   - the status payload the shell keeps fresh (App.tsx owns that poll), so the
//     card can never say "running" while the status strip above it says stopped;
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
//
// The rows are drawn as a DETAIL PANE rather than as a bordered list, which is a
// second port — of menumate's pack detail rather than of its manifest, since
// that is the one file in that tree laid out as a set of stated facts about a
// thing rather than as a form for changing it, which is what this card is:
//
//   App/UI/MenuHubPanels.swift:620-633  lockRow: the row. A label in a fixed
//     70pt column, right-aligned, beside a read-only field, with a lock glyph
//     after it and the whole row at opacity 0.7.
//   App/UI/MenuHubPanels.swift:27-45  PvField: the same label column, and the
//     row's hint — a subordinate line under the value at 11pt in the third tone.
//   App/UI/DesignSystem.swift:451-482  MMField: the value as a sunken, hairline
//     field at 13pt, `.monospaced` when the value is a token rather than words.
//   App/UI/MenuHubPanels.swift:637,657 the two spacings, and why they differ:
//     13 between the pane's levels, 10 between the rows inside one level.
//
// The rule that port exists to carry is `mono`. lockRow takes it per row, and
// the caller sets it for exactly one reason: whether the value is an identifier.
// So the Daemon row's version is monospaced and the Profile row's name is not,
// and a reader can tell from the face alone which of these values are machine
// vocabulary and which are words somebody chose. That distinction is the point
// of the flag; without it every value would read as the same kind of thing.
//
// Nothing else changed. The four fields, their order, and the words beside each
// are the ones this card always had, and which rows exist at all is still the
// daemon's decision — this file adds no metric of its own.
//
// No code was copied from either reference. Tracked in
// third_party/menumate/ATTRIBUTION.md.

import type { ReactElement } from 'react'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, FactFoot, FactList, FactRow } from './common'
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
  const loading = profiles.loading || readiness.loading

  // The two conditions that QUALIFY the version rather than being it. They used
  // to be appended to the same sentence, which put a claim and its consequences
  // in one string and so left the version with nowhere to be set in the
  // monospace face its own kind deserves. The hint is the reference's slot for
  // exactly that (PvField, MenuHubPanels.swift:37-42).
  const daemonHint = [
    status?.SafeMode ? 'Safe mode is on, so nothing acts until you say so.' : '',
    status?.Killed ? 'PANIC STOP is latched.' : '',
  ]
    .filter((line) => line !== '')
    .join(' ')

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={profiles.error || readiness.error}
    >
      {/* The daemon's own words first. A status the shell wrote would be a
          second answer to the running question, and this is the page a person
          lands on after the wizard. */}
      <FactList>
        <FactRow
          label="Daemon"
          badge={status ? (status.Running ? 'Running' : 'Stopped') : 'Connecting'}
          // A version is an identifier, so it is the one value on this card
          // that takes the monospace face. "CrossOS" is the product's own name
          // and not a token of its own, but lockRow's flag is per row (:620)
          // and not per fragment, so the cell takes the face as it stands.
          mono
          hint={daemonHint || undefined}
        >
          {status?.Version ? `CrossOS ${status.Version}` : 'The shell has not heard from the daemon yet.'}
        </FactRow>
        <FactRow
          label="Keyboard interception"
          badge={status ? (status.Interception ? 'On' : 'Off') : 'Unknown'}
          // The tap error is the line that teaches the user WHICH permission to
          // grant, so it is quoted here verbatim rather than summarised — as a
          // hint, because it qualifies the sentence above it rather than
          // standing in for it.
          hint={status?.TapError || undefined}
        >
          {status?.Interception
            ? 'CrossOS can read the keyboard and act on it.'
            : 'Shortcuts do nothing until interception is installed.'}
        </FactRow>
        {/* A profile label is a name a person chose, so it keeps the text face.
            The rule is the whole reason the flag exists: monospace here would
            claim "machine vocabulary" about something that is not. */}
        <FactRow label="Profile" badge={active ? 'Active' : 'None'}>
          {active
            ? active.label
            : 'No profile is active, so shortcuts run one at a time as you set them.'}
        </FactRow>
        <FactRow
          label="Extensions"
          badge={plugins.length === 0 ? 'None' : `${enabled.length} of ${plugins.length}`}
        >
          {extensionSentence(plugins.length, enabled.length)}
        </FactRow>

        {rows.length === 0 && !readiness.loading ? (
          <EmptyState>The daemon reported no readiness checks, so this card cannot say what is usable.</EmptyState>
        ) : null}
      </FactList>

      {/* The rollup belongs under the rows rather than among them: it is a
          footer on the pane, not a fifth fact (MMGroup's footer,
          DesignSystem.swift:576-582). Rendered only when it has a line to
          draw, because the pane's 13pt level gap would otherwise reserve the
          space for a footer that is not there. */}
      {loading || rows.length > 0 ? (
        <FactFoot>
          {loading ? <p>Reading the daemon…</p> : null}
          {rows.length > 0 ? (
            <p>
              {failing.length === 0
                ? `All ${plural(rows.length, 'check')} are ready.`
                : `${plural(ready.length, 'check')} of ${rows.length} ready. To do: ${failing
                    .map((row) => row.label || humanize(row.id))
                    .join(', ')}.`}
            </p>
          ) : null}
        </FactFoot>
      ) : null}
    </ControlFrame>
  )
}
