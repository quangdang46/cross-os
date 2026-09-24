// CrossOS settings shell.
//
// Port source: rectangle (ramonwessels/rectangle), whose preferences window
// is the closest analog to a CrossOS settings surface — one scrolling form of
// grouped rows, each row a label beside its control, closed by an About block
// carrying the version.
//
//   tmp/research/rectangle/Rectangle/PrefsWindow/SettingsViewController.swift
//     :307-311  vertical, leading-aligned stack with uniform row spacing
//     :10, :15   version label + check-for-updates button (About block)
//     :60        a launch-on-login style checkbox per setting
//   tmp/research/rectangle/Rectangle/PrefsWindow/PrefsViewController.swift
//     one control per action, laid out in the same row shape
//
// No code was copied: the reference is AppKit and this is React, so the form's
// STRUCTURE transfers, not its implementation. Tracked in
// third_party/rectangle/ATTRIBUTION.md.
//
// What is CrossOS's own: the rows are not hardcoded. The Go Host discovers the
// pages and their controls (§3.6c) and this file renders whatever the Service
// serves, so a new settings page is a Go change and never a UI change.

import { useCallback, useEffect, useState } from 'react'
import { Service } from '../bindings/crossos/app/backend'
import type { Page, Status } from '../bindings/crossos/app/backend/models'

// One control as the Go schema describes it. Mirrors the §3.6c declarative
// tier: the UI renders these, it does not interpret them.
interface Control {
  kind: string
  id: string
  label?: string
  text?: string
  note?: string
  action?: string
  source?: string
  steps?: string[]
  items?: string[]
}

function controlsOf(page: Page): Control[] {
  // Schema crosses the Wails bridge as json.RawMessage, which arrives
  // already decoded (an object) — the previous version assumed a string
  // and called JSON.parse on it, so every page threw into the catch and
  // rendered with no controls at all. Accept either shape.
  const raw = (page as unknown as { Schema?: unknown }).Schema
  let schema: unknown = raw
  if (typeof raw === 'string') {
    try {
      schema = JSON.parse(raw)
    } catch {
      return []
    }
  }
  if (schema && typeof schema === 'object') {
    const controls = (schema as { controls?: Control[] }).controls
    return Array.isArray(controls) ? controls : []
  }
  return []
}

export default function App() {
  const [pages, setPages] = useState<Page[]>([])
  const [active, setActive] = useState<string>('')
  const [status, setStatus] = useState<Status | null>(null)
  const [logs, setLogs] = useState<string[]>([])
  const [note, setNote] = useState<string>('')

  const refresh = useCallback(() => {
    Service.Pages().then((p) => {
      const list = p ?? []
      setPages(list)
      setActive((cur) => cur || list[0]?.ID || '')
    })
    Service.GetStatus().then((s) => {
      if (s) setStatus(s)
    })
    Service.UILogs().then((l) => {
      if (l) setLogs(l)
    })
  }, [])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  const page = pages.find((p) => p.ID === active)
  const lastLog = logs[logs.length - 1]

  return (
    <div className="window">
      <header className="masthead">
        <h1>CrossOS</h1>
        <p className="status">
          {status ? (status.Running ? 'Running' : 'Daemon not running') : 'Connecting…'}
          {status?.SafeMode ? ' · Safe mode' : ''}
          {status && !status.Interception ? ' · Remapping off' : ''}
        </p>
        {status?.TapError ? <p className="status is-error">{status.TapError}</p> : null}
      </header>

      <nav className="sections">
        {pages.map((p) => (
          <button
            key={p.ID}
            className={p.ID === active ? 'section is-active' : 'section'}
            onClick={() => setActive(p.ID)}
          >
            {p.Title}
          </button>
        ))}
      </nav>

      <main className="form">
        {page ? (
          <>
            <h2 className="group-title">{page.Title}</h2>
            {controlsOf(page).map((ctl) => (
              <ControlRow
                key={ctl.id}
                control={ctl}
                status={status}
                logs={logs}
                onNote={setNote}
                onDone={refresh}
              />
            ))}
          </>
        ) : (
          <p className="empty">No settings pages discovered.</p>
        )}
        {note ? <p className="note">{note}</p> : null}
      </main>

      {status?.Plugins?.length ? (
        <section className="form">
          <h2 className="group-title">Plugins</h2>
          {status.Plugins.map((pl) => (
            <div className="row" key={pl.ID}>
              <span className="row-label">{pl.ID}</span>
              <button
                className="control"
                onClick={() => {
                  Service.TogglePlugin(pl.ID, !pl.Enabled)
                    .then(() => setNote(`${pl.ID} → ${pl.Enabled ? 'disabled' : 'enabled'}`))
                    .then(refresh)
                    .catch((e) => setNote(String(e)))
                }}
              >
                {pl.Enabled ? 'Enabled' : 'Disabled'}
              </button>
            </div>
          ))}
        </section>
      ) : null}

      {/* About block: version plus the newest activity line, mirroring
          rectangle's version label and check-for-updates row. */}
      <footer className="about">
        {/* Version is served by the daemon (core.CurrentVersion); the
            fallback only shows before the first status lands. */}
        <span className="version">CrossOS {status?.Version || '—'}</span>
        {lastLog ? <span className="activity">{lastLog}</span> : null}
      </footer>
    </div>
  )
}

// ControlRow renders one discovered control. A kind with no renderer shows
// its label and the raw kind rather than guessing a widget for it.
function ControlRow(props: {
  control: Control
  status: Status | null
  logs: string[]
  onNote: (s: string) => void
  onDone: () => void
}) {
  const { control: c, status, onNote, onDone } = props

  if (c.kind === 'button') {
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <button
          className="control"
          onClick={() => {
            if (c.action === 'safety.panicStop') {
              Service.PanicStop()
                .then(() => onNote('Interception and plugin actions stopped.'))
                .then(onDone)
                .catch((e) => onNote(String(e)))
            } else if (c.action === 'safety.resume') {
              Service.Resume()
                .then(() => onNote('PANIC STOP cleared — re-installing the keyboard tap.'))
                .then(onDone)
                .catch((e) => onNote(String(e)))
            } else if (c.action === 'safety.reset') {
              Service.ResetEverything()
                .then((steps) => onNote(`Reset plan: ${(steps ?? []).join(' → ')}`))
                .catch((e) => onNote(String(e)))
            } else if (c.action === 'safety.confirmTrial') {
              // TODO(ui-port): a prompt is a stand-in, not a designed picker.
              // The real control arrives from the schema once the Safety page
              // declares a trialList control; until then the field name is
              // the honest minimum.
              const id = window.prompt('Plugin to confirm?', 'launcher')
              if (id) {
                Service.ConfirmTrial(id, true, true)
                  .then((state) => onNote(`${id} → ${state ?? '?'}`))
                  .then(onDone)
                  .catch((e) => onNote(String(e)))
              }
            } else if (c.action === 'safety.rollbackTrial') {
              // TODO(ui-port): see the confirmTrial note above.
              const id = window.prompt('Plugin to roll back?', 'launcher')
              if (id) {
                Service.RollbackTrial(id, 'user rollback')
                  .then((state) => onNote(`${id} → ${state ?? '?'}`))
                  .then(onDone)
                  .catch((e) => onNote(String(e)))
              }
            } else {
              onNote(`${c.action ?? c.id} runs from its own page.`)
            }
          }}
        >
          {c.label ?? c.id}
        </button>
      </div>
    )
  }

  if (c.kind === 'enableFlow') {
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <ol className="steps">
          {(c.steps ?? []).map((s) => (
            <li key={s}>{s}</li>
          ))}
        </ol>
      </div>
    )
  }

  if (c.kind === 'note') {
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <span className="control is-text">{c.text ?? c.note ?? ''}</span>
      </div>
    )
  }

  if (c.kind === 'traceList') {
    // Activity wants the actual log lines, not the data-source label.
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <span className="control is-text">{props.logs.join(' · ') || c.source || ''}</span>
      </div>
    )
  }

  if (c.kind === 'checklist') {
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <span className="control is-text">{(c.items ?? []).join(' · ') || c.source || ''}</span>
      </div>
    )
  }

  if (c.kind === 'pluginList') {
    return (
      <div className="row">
        <span className="row-label">{c.label ?? c.id}</span>
        <span className="control is-text">
          {(status?.Plugins ?? []).map((p) => p.ID).join(' · ')}
        </span>
      </div>
    )
  }

  return (
    <div className="row">
      <span className="row-label">{c.label ?? c.id}</span>
      <span className="control is-text is-unhandled">{c.kind}</span>
    </div>
  )
}
