// The profile cards (bead w6-frontend-setup-renderers).
//
// The product is a profile; the architecture calls the same thing a plugin set.
// So a card is a DECLARED ENTRY with a display name and a description, not a
// list of switches, and applying it is one button instead of eleven — with
// undoing it one button back (ProfilesPage, profilespage.go). Individual
// switches are deliberately absent here: they stay on the pages that own them
// (Keyboard, Windows, Explorer), the same boundary the Shortcuts page keeps for
// matrix editing.
//
// The per-capability rollup rides on the card rather than behind a second fetch,
// so one answer settles whether the click landed. A capability that is not
// available says WHY (the daemon's own reason), and one whose plugin is not
// loaded reports zero of its rules instead of nothing at all — a capability
// that silently vanished reads as a profile that does not include it.
//
// Individual switches are not duplicated here, and neither is the matrix: the
// same two boundaries the rest of the nav keeps.
//
// Port sources:
//
//   newfile (mariusgm/newfile) — the file-type LIBRARY, not a settings page:
//     Shared/SeedPresets.swift:4-14   a list of declared entries, each with its
//                                     own display name and its own state, so the
//                                     thing a person picks is a thing they can
//                                     read
//     Shared/FileTypeEntry.swift:3-12  the entry carries a name of its own
//                                     rather than one prettified from its key
//
//   rectangle (ramonwessels/rectangle) — the window-ZONE list, where every
//     named area is a row with its own state and none of them is collapsed into
//     the one beside it:
//     Rectangle/PrefsWindow/SnapAreaViewController.swift:16-24   one selector
//       per named area, each its own control
//     Rectangle/PrefsWindow/SnapAreaViewController.swift:75-86   a label beside
//       its own popup, so an area is named in words and not by its key
//     Rectangle/PrefsWindow/SnapAreaViewController.swift:269,279 each area
//       named by its own displayName, listed rather than summarised
//
// No code was copied: the reference is Swift and this is React over a declared
// schema, so the card shape and the one-row-per-capability rule transfer and
// the drawing does not. Tracked in third_party/newfile/ATTRIBUTION.md and
// third_party/rectangle/ATTRIBUTION.md.

import { useState } from 'react'
import type { ReactElement } from 'react'
import type { ProfileCapabilityRow } from '../types/controls'
import { asText, failedTo } from '../lib/wire'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { commandFor } from './actions'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

// The capability vocabulary the page declares. A page that names a different
// apply action gets that one instead, so the write is the page's decision and
// never this file's.
const APPLY_ACTION = 'profile.apply'

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

export function ProfileListControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const profiles = useResource(() => ctx.service.Profiles(), ctx.refreshToken, [], ctx.note)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const rows = profiles.data ?? []
  // A page may name the write under the capability vocabulary or under the
  // source spelling its schema carries; both are the one bound call, so an
  // unregistered spelling falls back to the capability rather than failing.
  const declared = asText(control.applyAction)
  const action = commandFor(declared) ? declared : APPLY_ACTION

  async function apply(id: string, label: string): Promise<void> {
    const command = commandFor(action)
    if (!command) {
      const message = `"${action}" is declared on this page, and this build has no command for it.`
      setError(message)
      ctx.note(message)
      return
    }
    setBusy(id)
    setError('')
    try {
      await command(ctx.service, { id })
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
          {rows.map((profile) => (
            <li className={profile.active ? 'ctl-card is-active' : 'ctl-card'} key={profile.id}>
              <div className="ctl-card-head">
                <span className="ctl-card-name">{profile.label || humanize(profile.id)}</span>
                {profile.active ? <span className="ctl-chip">Active</span> : null}
              </div>
              {profile.description ? <p className="ctl-value">{profile.description}</p> : null}
              {profile.capabilities.length === 0 ? (
                <p className="ctl-empty">This profile declares no capabilities yet.</p>
              ) : (
                <ul className="ctl-caps">
                  {profile.capabilities.map((capability) => {
                    const { mark, state } = capabilityState(capability)
                    return (
                      <li className="ctl-cap" key={capability.id}>
                        <span className="ctl-cap-mark" aria-hidden="true">
                          {mark}
                        </span>
                        <span className="ctl-label">{capability.label || humanize(capability.id)}</span>
                        <span className="ctl-value">{state}</span>
                      </li>
                    )
                  })}
                </ul>
              )}
              <div className="ctl-actions">
                <button
                  type="button"
                  className="ctl-button"
                  disabled={busy !== ''}
                  onClick={() => void apply(profile.id, profile.label || humanize(profile.id))}
                >
                  {busy === profile.id ? 'Applying…' : `Apply ${profile.label || humanize(profile.id)}`}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </ControlFrame>
  )
}
