// The command palette list (bead cross-os-itq).
//
// Commands are data, which is the point: a plugin that registers a command
// appears in this list with no change to Core and none to the shell. The
// filter box is there because the list is the union of every plugin's commands
// and a user looking for one does not want to scroll past thirty of someone
// else's to reach it.
//
// Running a command is the part this build cannot do, and the row says so.
// core.shortcuts declares a command.execute permission and the daemon serves the
// list, but no bound method executes one — so a Run button here would be a
// button that does nothing while looking exactly like one that works. The id is
// shown instead, because that is what a person needs in order to run the
// command the way the daemon does support today.

import { useState } from 'react'
import type { ReactElement } from 'react'
import { humanize, plural } from '../lib/format'
import { useResource } from '../lib/useResource'
import { ControlFrame, EmptyState, TextField } from './common'
import type { ControlProps } from './common'

export function PaletteControl(props: ControlProps): ReactElement {
  const { control, ctx } = props
  const commands = useResource(() => ctx.service.Commands(), ctx.refreshToken, [], ctx.note)
  const [filter, setFilter] = useState('')

  const rows = commands.data ?? []
  const needle = filter.trim().toLowerCase()
  const shown =
    needle === ''
      ? rows
      : rows.filter((row) =>
          `${row.title} ${row.plugin} ${row.id}`.toLowerCase().includes(needle),
        )

  return (
    <ControlFrame label={control.label ?? control.id} note={control.note} error={commands.error}>
      {rows.length === 0 ? (
        <EmptyState>No plugin has registered a command yet.</EmptyState>
      ) : (
        <>
          <TextField
            label="Filter commands"
            value={filter}
            onChange={setFilter}
            placeholder="Type part of a name"
          />
          <p className="ctl-value">
            {plural(shown.length, 'command')} shown of {rows.length}
          </p>
          {shown.length === 0 ? (
            <EmptyState>No command matches “{filter.trim()}”.</EmptyState>
          ) : (
            <ul className="ctl-list">
              {shown.map((row) => (
                <li className="ctl-item" key={row.id}>
                  <span className="ctl-label">{row.title || row.id}</span>
                  <span className="ctl-chip">{humanize(row.plugin)}</span>
                  <span className="ctl-value">{row.id}</span>
                </li>
              ))}
            </ul>
          )}
          <p className="ctl-empty">
            Running a command from this list is not available in this build.
          </p>
        </>
      )}
    </ControlFrame>
  )
}
