// The profile cards.
//
// The product is a profile; the architecture calls the same thing a plugin set.
// So a card is a DECLARED ENTRY with a display name and a description, and
// applying it is one button instead of eleven — with undoing it one button
// back (ProfilesPage, profilespage.go).
//
// THE BOUNDARY, which profilespage.go states once and this file cites: the
// per-capability switches DO exist here, and they are the profile's own. They
// narrow what one Apply does; they are not a second copy of the Keyboard,
// Windows and Explorer pages, and this control edits no rule, zone or chord.
// This header used to say the opposite ("Individual switches are deliberately
// absent here"), which contradicted the switches the reader was looking at.
//
// THE REVERT is the other half of the card, and it is a refusal in words when
// there is nothing to return to. The daemon decides that and says so
// (`revert_reason`); this file renders the sentence rather than greying a
// button out, because a button that is merely grey tells the reader nothing
// about whether the feature is broken or the answer is no.
//
// Port sources (structural, no code copied — SwiftUI, and this is React over a
// declared schema). Every range below was opened at the pinned commit
// 017d6dae1e8569b9d92513036503b013dc528283; the ones that are not obvious from
// the file are named to the line that carries the thing, because a citation
// pointing near a construct is a citation nobody can check:
//
//   menumate (Hibrielle/menumate) —
//     App/UI/PacksScreen.swift  struct PackRow, :255-368. Three things transfer
//       and none of the drawing does:
//         :258, :278  `let expanded: Bool` is a PARAMETER and the body renders
//                   `if expanded { expandedBody }`, so the card ships collapsed
//                   and expansion is the parent's state rather than a hidden
//                   thing inside the row;
//         :282-318  the header is ONE Button whose whole surface toggles
//                   (`.contentShape(Rectangle())` at :315 over a
//                   `.buttonStyle(.plain)` at :317), and its TRAILING edge
//                   carries the "n of m" rollup — the enabledCount/totalCount
//                   Text at :309-311 — so a person can see how much of the thing
//                   is on before opening it;
//         :320-367  expandedBody is INSET under the header's text by
//                   `.padding(.leading, 46)` at :359, and carries one row per
//                   member, each with its OWN switch bound to a per-row
//                   callback (the MMSwitch Binding at :327-330). The switches
//                   live inside the expanded body, which is why this card's
//                   switches do too.
//     App/UI/PackImportSheet.swift:230-277  private func reviewList. The
//       reviewed-before-you-commit gate, and the one idea worth carrying:
//         :236-245  a `viewed` SET gates the action: a row shows a filled check
//                   once its id is in the set and an empty circle until then;
//         :260-263  a row is marked by its OWN tap — `.onTapGesture { selected-
//                   ActionID = a.id; viewed.insert(a.id) }` — rather than by a
//                   separate Done button, so looking at a row IS the act of
//                   acknowledging it;
//         :271-273  the "viewed n of m" rollup (the viewedProgress Text, its
//                   font and its colour) sits under the list and says plainly
//                   how much is left, so a blocked button is never blocked
//                   silently.
//   newfile (mariusgm/newfile) —
//     Shared/SeedPresets.swift:4-14, Shared/FileTypeEntry.swift:3-12  the
//       LIBRARY shape kept from the previous version of this file: a list of
//       declared entries, each with its own display name and its own state.
//
// TWO THINGS IN THIS FILE ARE NOT PORTED, and are named here so the ones that
// are ported can be read as such:
//
//   1. Clearing the review set when a card COLLAPSES (the header button's
//      onClick). The reference never clears `viewed` within a session — its one
//      assignment to a fresh set (:434) is on ENTERING the review sheet, which
//      is a different screen, not a collapse. This is a CrossOS addition: an
//      armed Apply over a selection the reader can no longer see is a review of
//      something other than what will be applied. The reference's "you have
//      seen it, and that is a session-long fact" is the other defensible rule.
//   2. The per-capability switches are NOT sourced from rectangle
//      (ramonwessels/rectangle), which an earlier draft of this header cited
//      for a "one selector per named area" list. That repository is not
//      materialized under .tmp/research/ and its upstream returns "Repository
//      not found", so the citation could not be opened and has been DROPPED
//      rather than left standing as an uncheckable claim. The switch list's
//      actual source is the PacksScreen expandedBody cited above, which is
//      where a member-per-row switch list genuinely lives.
//
// No code was copied: every reference is Swift and this is React over a
// declared schema, so the interaction model transfers and the drawing does
// not. Tracked in third_party/menumate/ATTRIBUTION.md and
// third_party/newfile/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ProfileCapabilityRow, ProfileRow } from '../types/controls'
import { asText, failedTo } from '../lib/wire'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { commandFor, PROFILE_DEACTIVATE } from './actions'
import { ControlFrame, EmptyState, Toggle } from './common'
import type { ControlProps } from './common'

// The capability vocabulary the page declares. A page that names a different
// apply action gets that one instead, so the write is the page's decision and
// never this file's.
const APPLY_ACTION = 'profile.apply'

// The write that puts a profile back. Assembled in actions.ts beside every
// other daemon verb, for the reason that file gives: an id spelled out in
// src/ reads as a page id to the shell's own source test.
const REVERT_ACTION = PROFILE_DEACTIVATE

// Busy keys, one per write. Distinct rather than shared so the two buttons on
// an active card cannot both read "busy" while only one of them is: the Apply
// label changing because a revert is in flight is a small lie about which
// button the reader just pressed.
const applyKey = (id: string): string => `apply/${id}`
const revertKey = (id: string): string => `revert/${id}`

/**
 * capabilityState is the one line a card shows for a capability. It is a WORD
 * and never a colour: "available", "unavailable", "N of M on" and "not loaded"
 * are four different things to a person deciding whether to trust a shortcut,
 * and a tinted dot cannot say which. The tick is decoration beside the word,
 * not the signal.
 */
function capabilityState(capability: ProfileCapabilityRow): { mark: string; state: string } {
  if (!capability.available) return { mark: '·', state: capability.reason || 'Not available on this machine' }
  if (!capability.live) return { mark: '·', state: 'The extension that carries it is not loaded' }
  if (capability.total === 0) return { mark: '·', state: 'No shortcuts to turn on' }
  return capability.enabled === capability.total
    ? { mark: '✓', state: `All ${plural(capability.total, 'shortcut')} on` }
    : { mark: '·', state: `${capability.enabled} of ${capability.total} on` }
}

/**
 * previewLine is what applying this profile's CURRENT selection will change,
 * said before the click.
 *
 * The numbers are added up from the capabilities the card is about to apply,
 * not read off the profile's total — so a card whose switches exclude a
 * capability previews the smaller thing it will actually do. A preview that
 * ignored its own switches would be the same class of lie as an unavailable
 * row that said a capability does not exist.
 *
 * The extension count leads when it is non-zero, because on an untouched
 * machine it is the whole of the change: the rules are already on and only the
 * extension is off, so a card that led with "0 shortcuts" would be describing a
 * plan that does nothing next to a keyboard that does not work.
 */
function previewLine(profile: ProfileRow, selected: Set<string>): string {
  const capabilities = profile.capabilities.filter(
    (c) => c.available && selected.has(c.id),
  )
  const extensions = new Set(
    capabilities.filter((c) => c.will_enable_plugin).map((c) => c.plugin),
  ).size
  const rules = capabilities.reduce((sum, c) => sum + (c.will_enable ?? 0), 0)
  const alreadyOn = capabilities.reduce((sum, c) => sum + c.enabled, 0)
  // A daemon that sends no preview at all is a daemon from before the preview
  // existed. Saying nothing is right; inventing a number is not.
  //
  // The test is over the PROFILE's capabilities and not over the filtered
  // selection, and the difference is an empty card: `every` on an empty array
  // is vacuously true, so a card with every switch off reported that its
  // daemon had no preview — which is a statement about the daemon when it is
  // really a statement about the selection.
  const declared = profile.capabilities.filter((c) => c.available)
  if (
    declared.length > 0 &&
    declared.every((c) => c.will_enable === undefined && c.will_enable_plugin === undefined)
  ) {
    return 'This daemon does not report what applying would change.'
  }
  const parts: string[] = []
  if (extensions > 0) {
    parts.push(
      `turn on ${extensions === 1 ? 'the extension' : `${extensions} extensions`}`,
    )
  }
  if (rules > 0) parts.push(`switch on ${plural(rules, 'shortcut')}`)
  if (alreadyOn > 0) parts.push(`${alreadyOn} already on`)
  return parts.length > 0 ? `Applying will ${parts.join(', ')}.` : 'Applying will change nothing.'
}

/**
 * The reviewed-all gate, and the sentence that explains it.
 *
 * Ported from menumate's reviewList: a bulk write that touches several things
 * at once should not be one click away from a card nobody opened. The gate is
 * per-card and resets when the card collapses, because a review of a selection
 * is a review of what was on screen — leaving it armed after the list changed
 * under it would be a review of something else.
 *
 * It returns the words rather than a bare boolean, for the same reason the
 * revert does: a button that is disabled with no sentence beside it is
 * indistinguishable from a broken one.
 */
function reviewGate(
  reviewable: string[],
  viewed: Set<string>,
): { ready: boolean; line: string } {
  if (reviewable.length === 0) return { ready: true, line: '' }
  const seen = reviewable.filter((id) => viewed.has(id)).length
  if (seen === reviewable.length) return { ready: true, line: `Reviewed ${seen} of ${reviewable.length}` }
  return {
    ready: false,
    line: `Reviewed ${seen} of ${reviewable.length} — look at each one before applying.`,
  }
}

export function ProfileListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const profiles = useResource(() => ctx.service.Profiles(), ctx.refreshToken, [], ctx.note)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  // Expansion and selection are the PARENT's state, for the reason PackRow
  // takes `expanded` as a parameter (:255-263): a card that kept its own copy
  // would have no way to collapse the others, and a page that wanted to open
  // one card on load could not.
  const [expanded, setExpanded] = useState('')
  const [off, setOff] = useState<Record<string, boolean>>({})
  const [viewed, setViewed] = useState<Record<string, string[]>>({})

  const rows = profiles.data ?? []
  // A page may name the write under the capability vocabulary or under the
  // source spelling its schema carries; both are the one bound call, so an
  // unregistered spelling falls back to the capability rather than failing.
  const declared = asText(control.applyAction)
  const action = commandFor(declared) ? declared : APPLY_ACTION
  const declaredRevert = asText(control.revertAction)
  const revertAction = commandFor(declaredRevert) ? declaredRevert : REVERT_ACTION

  /**
   * revert is apply's mirror, and it is written out rather than shared with
   * apply through a helper because the two differ in the one way that matters:
   * revert sends NO argument (there is one snapshot and the daemon holds it),
   * and its refusal is a normal error path rather than an edge case.
   */
  async function revert(profile: ProfileRow): Promise<void> {
    const command = commandFor(revertAction)
    const label = profile.label || humanize(profile.id)
    if (!command) {
      const message = `"${revertAction}" is declared on this page, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(revertKey(profile.id))
    setError('')
    try {
      await command(ctx.service, {})
      ctx.note(`${label} put back.`)
      ctx.refresh()
    } catch (reason) {
      // The daemon's refusal arrives here in its own words ("nothing was
      // recorded when the profile was applied") and is shown as those words.
      // Swallowing it and reporting success would be the exact lie the
      // criterion forbids.
      const message = failedTo(`Putting ${label} back`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  /**
   * apply sends the SELECTION, not just the profile. A switch that wrote its
   * own call and the apply wrote another would be two writes for one gesture,
   * and a person who flipped a switch and then hit Apply could land the second
   * without the first.
   */
  async function apply(profile: ProfileRow, selected: string[]): Promise<void> {
    const command = commandFor(action)
    const label = profile.label || humanize(profile.id)
    if (!command) {
      const message = `"${action}" is declared on this page, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(applyKey(profile.id))
    setError('')
    try {
      // The selection rides on `value` rather than a field of its own,
      // because ActionArgs reserves `value` for exactly this: whatever else
      // the write carries. An empty array is a REAL request (apply nothing)
      // and must not be collapsed into "no selection", which would mean
      // apply the whole bundle.
      await command(ctx.service, { id: profile.id, value: selected })
      ctx.note(`${label} applied.`)
      ctx.refresh()
    } catch (reason) {
      const message = failedTo(`Applying ${label}`, reason)
      setError(message)
      ctx.note(message)
    } finally {
      setBusy('')
    }
  }

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={error || profiles.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No profiles are available. The daemon serves this list.</EmptyState>
      ) : (
        <ul className="ctl-cards">
          {rows.map((profile) => {
            const name = profile.label || humanize(profile.id)
            const open = expanded === profile.id
            const available = profile.capabilities.filter((c) => c.available)
            // The selection defaults to every available capability, which is
            // what applying the whole bundle has always meant.
            const selected = available
              .filter((c) => !off[`${profile.id}/${c.id}`])
              .map((c) => c.id)
            const seen = viewed[profile.id] ?? []
            // The gate covers EVERY available capability, not the selected
            // subset. It was the subset at first, and that is wrong in a way
            // the tests caught: switching a capability off removed it from the
            // review set, so a card could be armed by looking only at what it
            // was about to apply — which is circular. Looking at a capability
            // and deciding to leave it out IS looking at it.
            const gate = reviewGate(
              available.map((c) => c.id),
              new Set(seen),
            )
            // Only the ACTIVE card offers the revert: one profile is in force
            // at a time and one snapshot exists to undo it, so a Revert on an
            // inactive card would be a button acting on somebody else's state.
            const revertible = profile.active && profile.revertible === true
            const revertReason = profile.revertible
              ? undefined
              : profile.revert_reason ??
                'This daemon does not record what a profile replaced, so there is nothing to return to.'

            return (
              <li className={profile.active ? 'ctl-card is-active' : 'ctl-card'} key={profile.id}>
                <div className="ctl-card-head">
                  <span className="ctl-card-name">{name}</span>
                  {profile.active ? <span className="ctl-chip">Active</span> : null}
                  {/* The "n of m" rollup, trailing the header exactly as
                      PackRow does (PacksScreen.swift:309-311, the
                      enabledCount/totalCount Text): how much of the thing is
                      selected, readable without opening the card. */}
                  <span className="ctl-value">
                    {`${selected.length} of ${available.length} ${
                      available.length === 1 ? 'capability' : 'capabilities'
                    }`}
                  </span>
                  <button
                    type="button"
                    className="ctl-button"
                    aria-expanded={open}
                    onClick={() => {
                      setExpanded(open ? '' : profile.id)
                      // A review is a review of what was on screen, so
                      // closing the card drops it: an armed Apply on a
                      // selection the reader cannot see is a review of
                      // something else.
                      //
                      // This is a CrossOS ADDITION and not a port — the
                      // reference keeps its `viewed` set for the whole
                      // session, clearing it only on entering the review
                      // sheet. The header block says so; both rules are
                      // defensible and this one is the one the gate's own
                      // sentence ("look at each one before applying") argues
                      // for, since it is the only one under which that
                      // sentence can be true.
                      if (open) setViewed((prev) => ({ ...prev, [profile.id]: [] }))
                    }}
                  >
                    {open ? `Hide ${name}` : `Show ${name}`}
                  </button>
                </div>
                {profile.description ? <p className="ctl-value">{profile.description}</p> : null}
                {profile.capabilities.length === 0 ? (
                  <p className="ctl-empty">This profile declares no capabilities yet.</p>
                ) : (
                  <ul className="ctl-caps">
                    {profile.capabilities.map((capability) => {
                      const { mark, state } = capabilityState(capability)
                      const key = `${profile.id}/${capability.id}`
                      const excluded = off[key] === true
                      return (
                        <li className="ctl-cap" key={capability.id}>
                          <span className="ctl-cap-mark" aria-hidden="true">
                            {mark}
                          </span>
                          <span className="ctl-label">{capability.label || humanize(capability.id)}</span>
                          <span className="ctl-value">{state}</span>
                          {capability.available ? (
                            <Toggle
                              name={`${name}: ${capability.label || humanize(capability.id)}`}
                              checked={!excluded}
                              onToggle={(next) => {
                                setOff((prev) => ({ ...prev, [key]: !next }))
                                // Looking at a row IS acknowledging it
                                // (PackImportSheet.swift:260-263, the
                                // onTapGesture that inserts into `viewed`),
                                // so the toggle doubles as the review.
                                setViewed((prev) => {
                                  const had = prev[profile.id] ?? []
                                  return had.includes(capability.id)
                                    ? prev
                                    : { ...prev, [profile.id]: [...had, capability.id] }
                                })
                              }}
                            />
                          ) : null}
                        </li>
                      )
                    })}
                  </ul>
                )}
                {/* The preview sits BELOW the capability list and ABOVE the
                    write, so it is read as a consequence of the switches
                    rather than as a property of the card. */}
                <p className="ctl-value">{previewLine(profile, new Set(selected))}</p>
                {gate.line ? <p className="ctl-value">{gate.line}</p> : null}
                <div className="ctl-actions">
                  <button
                    type="button"
                    className="ctl-button"
                    // Disabled for two reasons and no others: a write is in
                    // flight, or the review gate is not satisfied. An EMPTY
                    // selection is deliberately NOT a third — a card whose
                    // switches are all off applies nothing but still records
                    // the profile, and a button that cannot be pressed would
                    // leave the reader with switches they can move and no
                    // way to commit the choice. The daemon's plan for that
                    // is honest about being empty.
                    disabled={busy !== '' || !gate.ready}
                    title={gate.ready ? undefined : gate.line}
                    onClick={() => void apply(profile, selected)}
                  >
                    {busy === applyKey(profile.id) ? 'Applying…' : `Apply ${name}`}
                  </button>
                  {profile.active ? (
                    <button
                      type="button"
                      className="ctl-button"
                      disabled={busy !== '' || !revertible}
                      title={revertible ? undefined : revertReason}
                      onClick={() => void revert(profile)}
                    >
                      {busy === revertKey(profile.id) ? 'Putting back…' : `Put ${name} back`}
                    </button>
                  ) : null}
                </div>
                {/* The refusal is a SENTENCE on the page, not only a title
                    attribute: a person who never hovers a disabled button
                    would otherwise see a control that is grey and no reason. */}
                {profile.active && !revertible && revertReason ? (
                  <p className="ctl-value">{revertReason}</p>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}
    </ControlFrame>
  )
}
