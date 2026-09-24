// The Level B gating badge (bead w7-frontend-remap).
//
// WHAT THE BADGE IS. §5.4 gives a pack action one of two bodies: a `native`
// capability CrossOS runs in-process, or a `process` action that runs a Level B
// child process and therefore needs the shell gate. The page's own note says the
// rule it wants drawn: "Shell actions show Level B gating; native actions show
// the capability name." So a badge is never a colour and never a bare "gated" —
// it is either the capability the action runs, or the gate it waits on, in
// words, because a person deciding whether to trust a right-click menu item
// needs to know WHICH thing is being asked for.
//
// THE GATE IS THE PERMISSION, NOT A LOCAL FLAG. `shell.execute` is the daemon's
// own permission constant (core/pkg/intent: PermShellExecution), and
// plugin.pack.Match returns MatchShellGated for a process action until that
// permission is granted — so "asks for shell execution" is the same fact the
// matcher gates on, read from the grants the router authorises the extension's
// rules against (builtin.Grants, served as core:pluginMeta permissions). The
// badge is not deciding whether an action is gated; it is reporting the grant
// that decides it, which is why a badge can never disagree with the runtime.
//
// The per-action body is not served — no pack manifest is loaded, so there is no
// action whose `type` this control could read, and inventing one would badge an
// action that does not exist. What IS served is each extension's granted
// permissions, and that is what the badge reports: an extension holding
// shell.execute is one whose actions would sit behind the Level B gate, and one
// that does not holds only native capabilities, named.
//
// An empty registry reads as empty. "No extension is installed" is a fact about
// the machine, and it is never an error.
//
// Port source: rectangle (ramonwessels/rectangle), for the RULE the badge
// follows rather than for a gating model — a state that decides whether the
// user trusts a control is said in words beside that control, never as a colour
// alone.
//
//   Rectangle/PrefsWindow/SnapAreaViewController.swift:75-86  a name in words
//     beside its own control
//
// What is CrossOS's own is the gate itself: `shell.execute` is the daemon's
// intent permission and the one plugin.pack.Match gates a Level B process action
// on, so the badge reports the grant rather than carrying a privilege model of
// its own. No code was copied — the reference is AppKit on macOS with window
// settings CrossOS does not have. Tracked in
// third_party/rectangle/ATTRIBUTION.md.

import type { ReactElement } from 'react'
import type { PluginMetaRow } from '../types/controls'
import { humanize } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState } from './common'
import type { ControlProps } from './common'

/** The permission a process (Level B) action is gated on. */
const SHELL_PERMISSION = 'shell.execute'

export function GateBadgeControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const packs = useResource(() => ctx.service.PluginMeta(), ctx.refreshToken, [], ctx.note)
  const rows: PluginMetaRow[] = packs.data ?? []

  return (
    <ControlFrame
      label={control.label ?? control.id}
      note={control.note}
      error={packs.error}
    >
      {rows.length === 0 ? (
        <EmptyState>No extension is installed, so there is nothing to gate.</EmptyState>
      ) : (
        <>
          <p className="ctl-value">
            An action that runs a shell command is held back until that extension is granted
            permission to execute one. An action that runs in CrossOS shows the capability it uses
            instead.
          </p>
          <ul className="ctl-list">
            {rows.map((row) => {
              const gated = row.permissions.includes(SHELL_PERMISSION)
              const native = row.permissions.filter((grant) => grant !== SHELL_PERMISSION)
              return (
                <li className="ctl-item" key={row.id}>
                  <span className="ctl-label">{row.name || humanize(row.id)}</span>
                  <span className="ctl-chip">{gated ? 'Level B — gated' : 'Native'}</span>
                  <span className="ctl-value">
                    {gated
                      ? `Its shell actions wait on the ${SHELL_PERMISSION} permission.`
                      : native.length > 0
                        ? `Native capabilities: ${native.map((grant) => humanize(grant)).join(', ')}`
                        : 'No capability is granted, so no action can run.'}
                  </span>
                </li>
              )
            })}
          </ul>
          <p className="ctl-empty">
            Which of an extension's actions is native and which is a shell command comes from its
            pack manifest, and no manifest is loaded yet — so the badge reports the grant that
            decides it rather than guessing at an action.
          </p>
        </>
      )}
    </ControlFrame>
  )
}
