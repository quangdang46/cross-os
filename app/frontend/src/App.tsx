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
// The nav rail below is a second port, of menumate's sidebar rather than its
// manifest: a fixed-width left column with a small section cap over each run of
// items (tmp/research/menumate/App/UI/MenuHubScreen.swift:65 the 326pt rail, and
// :692-710 SectionCap — a 9.5pt semibold, tracked label above a group). Both
// ports transfer layout, not code; menumate's entry is in
// third_party/menumate/ATTRIBUTION.md.
//
// What is CrossOS's own: the rows are not hardcoded. The Go Host discovers the
// pages and their controls (§3.6c) and this file renders whatever the Service
// serves, so a new settings page is a Go change and never a UI change. Control
// kinds live in ./controls; a page id or a control id in a conditional below
// would be a defect, not a convenience. The same rule covers the nav: its
// order, its groups and its opening page all come off the wire, because the
// Host is the only place that knows which page a fresh profile lands on.

import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
// The generated bindings are named in exactly one module (lib/service), so
// this file never reaches into ../bindings by path: a page, a control and the
// shell all cross the bridge through the same seam. The controls get `service`
// — the copy checked against ServiceApi — while this file keeps the raw module
// for the three calls ServiceApi deliberately does not model (page discovery
// and the two log sources); handing it the checked copy instead would just move
// that gap rather than close it.
import { Service, service } from './lib/service'
import { humanize } from './lib/format'
import type { Status } from './types/controls'
import { renderControl, type ControlContext } from './controls'

// The control shape belongs to the registry. Deriving it from renderControl's
// own signature keeps this file from naming a type the registry may move.
type Control = Parameters<typeof renderControl>[0]

// Page discovery is App's, so the discovered page is declared here: the fields
// App reads, and nothing else. The rest of the payload is the registry's
// business, and Schema stays unknown because json.RawMessage crosses the bridge
// either already decoded or as text.
//
// Group, Order and FirstRun are the host's nav answer, not decoration. The Host
// sorts one flat list by Order and, while the profile is still fresh, lets the
// first-run page lead; Group is what the shell groups that sorted list by, and
// FirstRun is the host's projection of the schema flag below. All three are
// PascalCase because the Go struct carries no json tags.
interface Page {
  ID: string
  Title: string
  Group: string
  Order: number
  FirstRun: boolean
  Schema: unknown
}

// A page schema as the Go side declares it (§3.6c): a description, a first-run
// marker and a list of controls. Nothing else is read, so a page may carry
// fields the shell has never heard of without breaking the shell — but a field
// the daemon DOES send is read here rather than dropped, which is what makes
// firstRun below work.
interface PageSchema {
  description?: string
  firstRun?: boolean
  controls?: Control[]
}

// A nav group is a run of pages that share a Group value, in served order. The
// Host serves each group's pages together, so grouping by first appearance is
// enough and nothing is reordered here: a nav that disagreed with the host's
// order would be a second answer to the same question.
interface NavGroup {
  group: string
  pages: Page[]
}

function navGroups(pages: Page[]): NavGroup[] {
  const groups: NavGroup[] = []
  for (const page of pages) {
    const open = groups.find((g) => g.group === page.Group)
    if (open) open.pages.push(page)
    else groups.push({ group: page.Group, pages: [page] })
  }
  return groups
}

// The opening page is the first page the host served. It is not "the first
// page alphabetically" and it is not a page named in a list here: the host is
// what knows a fresh profile lands on the wizard and a finished one lands on
// Home, and re-deciding that in the shell would let the two disagree.
function landingId(pages: Page[]): string {
  return pages[0]?.ID ?? ''
}

// Failures are tracked per source and cleared when that source recovers, so
// one dead call cannot hide another and a fixed call does not leave a stale
// error lying on screen. The keys are the names the user reads.
type Source = 'settings pages' | 'daemon status' | 'activity log' | 'shell log'

// A bridge error is text, not a crash: the shell prints the message.
function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

// Schema crosses the Wails bridge as json.RawMessage. The generated type says
// string, the bridge hands the frontend the raw JSON already decoded, so both
// shapes are accepted. Anything that is not an object renders as no controls
// rather than throwing the page away.
function schemaOf(page: Page): PageSchema {
  const raw: unknown = page.Schema
  let parsed: unknown = raw
  if (typeof raw === 'string') {
    try {
      parsed = JSON.parse(raw)
    } catch {
      return {}
    }
  }
  return parsed && typeof parsed === 'object' ? (parsed as PageSchema) : {}
}

// A settings window is not a log archive. The visible run is capped and the
// overflow is stated, which beats an unbounded scrollback that hides the top
// of the list under a thousand identical lines.
const MAX_TRACES = 200

// Event-log lines carry no clock of their own (the recorder appends winner,
// action, params), so a time is shown only when the daemon already wrote one. A
// per-event clock invented here would be a fiction; the snapshot stamp is the
// one time this shell can state truthfully.
const LEADING_STAMP =
  /^(?:\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?|\d{2}:\d{2}:\d{2})(?=\s|$)/

interface TraceLine {
  time: string
  iso: string
  text: string
}

function splitStamp(line: string): TraceLine {
  const m = LEADING_STAMP.exec(line)
  if (!m) return { time: '', iso: '', text: line }
  const stamp = m[0]
  // Only a full date parses as machine-readable datetime; a bare HH:MM:SS is
  // still a valid <time> body but carries no date, so it gets no attribute.
  const iso = /^\d{4}-\d{2}-\d{2}/.test(stamp) ? stamp.replace(' ', 'T') : ''
  return { time: stamp, iso, text: line.slice(stamp.length).trim() }
}

function Timeline(props: { logs: string[]; readAt: number }) {
  const { logs, readAt } = props
  // Traces arrive oldest-first (the recorder appends), so newest-first is a
  // reverse, and the cap keeps the newest end.
  const rows = useMemo(() => logs.slice(-MAX_TRACES).reverse().map(splitStamp), [logs])
  const overflow = logs.length - rows.length

  return (
    <section className="timeline" aria-label="Activity">
      <h3 className="group-title">
        Activity
        <span className="tl-meta">read at {new Date(readAt).toLocaleTimeString()}</span>
      </h3>
      {logs.length === 0 ? (
        <p className="ctl-empty">No activity recorded yet.</p>
      ) : (
        <>
          {overflow > 0 ? (
            <p className="tl-cap">
              Showing {rows.length} of {logs.length} events, newest first.
            </p>
          ) : null}
          <ul className="tl">
            {rows.map((row, i) => (
              <li className={row.time ? 'tl-row' : 'tl-row is-plain'} key={i}>
                {row.time ? <time className="tl-time" dateTime={row.iso || undefined}>{row.time}</time> : null}
                <span className="tl-msg">{row.text}</span>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

export default function App() {
  const [pages, setPages] = useState<Page[]>([])
  const [active, setActive] = useState<string>('')
  const [status, setStatus] = useState<Status | null>(null)
  const [logs, setLogs] = useState<string[]>([])
  const [readAt, setReadAt] = useState(() => Date.now())
  const [shellLog, setShellLog] = useState<string>('')
  const [note, setNote] = useState<string>('')
  const [faults, setFaults] = useState<Partial<Record<Source, string>>>({})
  const [refreshToken, setRefreshToken] = useState(0)

  const reportFault = useCallback((source: Source, e: unknown) => {
    setFaults((f) => ({ ...f, [source]: `${source}: unavailable (${message(e)})` }))
  }, [])

  const clearFault = useCallback((source: Source) => {
    setFaults((f) => {
      if (!(source in f)) return f
      const next = { ...f }
      delete next[source]
      return next
    })
  }, [])

  const refresh = useCallback(() => {
    // One bump per poll is the entire contract with the controls: a control
    // that owns a data source re-reads it when the token moves. The cadence
    // therefore belongs here, and the five seconds below, not in a control.
    setRefreshToken((n) => n + 1)

    // Each source lands on its own, so one failing call cannot blank the
    // others and nothing is left showing a stale value as if it were current.
    Service.Pages()
      .then((list) => {
        const found = list ?? []
        setPages(found)
        // A page can vanish between polls (its plugin was uninstalled). Keep
        // the current one while it still exists rather than pulling the user
        // off it mid-edit; only when it is gone does the host's own first page
        // take over, which is how the first poll opens on the wizard.
        setActive((cur) => (found.some((p) => p.ID === cur) ? cur : landingId(found)))
        clearFault('settings pages')
      })
      .catch((e) => reportFault('settings pages', e))

    Service.GetStatus()
      .then((s) => {
        setStatus(s ?? null)
        clearFault('daemon status')
      })
      .catch((e) => reportFault('daemon status', e))

    Service.GetEventLogs()
      .then((lines) => {
        setLogs(lines ?? [])
        setReadAt(Date.now())
        clearFault('activity log')
      })
      .catch((e) => reportFault('activity log', e))

    // The UI log is the backend's own record of bridge and IPC failures. The
    // newest line stays in the About block so a failure is never only in the
    // daemon's memory.
    Service.UILogs()
      .then((lines) => {
        const list = lines ?? []
        setShellLog(list.length ? list[list.length - 1] : '')
        clearFault('shell log')
      })
      .catch((e) => reportFault('shell log', e))
  }, [clearFault, reportFault])

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 5000)
    return () => clearInterval(t)
  }, [refresh])

  const page = pages.find((p) => p.ID === active) ?? null
  const schema = useMemo(() => (page ? schemaOf(page) : {}), [page])
  const controls = Array.isArray(schema.controls) ? schema.controls : []
  // One page, one log. A page that declares a traceList control already
  // renders ctx.logs through it, so the shell-level Timeline below would print
  // the same lines a second time under a different cap — and two copies that
  // disagree about how much history exists is the one thing a log view must
  // never do. The test is on the control KIND: naming a page id here would be
  // the hardcoding the file header rules out.
  const ownsTrace = controls.some((ctl) => ctl.kind === 'traceList')
  const problems = Object.values(faults)

  // note is the reporter, not the text: App owns the note line and renders it
  // from its own state, and a control's only job is to say what happened.
  const ctx: ControlContext = {
    service,
    status,
    logs,
    refreshToken,
    note: setNote,
    refresh,
    pageId: page?.ID ?? '',
  }

  return (
    <div className="window">
      <header className="masthead">
        <h1>CrossOS</h1>
        <p className="status">
          {status ? (status.Running ? 'Running' : 'Daemon not running') : 'Connecting…'}
          {status?.SafeMode ? ' · Safe mode' : ''}
          {status && !status.Interception ? ' · Remapping off' : ''}
        </p>
        {/* The tap error is the line that teaches the user which permission
            to grant, so it stays visible instead of collapsing into a log. */}
        {status?.TapError ? <p className="status is-error">{status.TapError}</p> : null}
      </header>

      {problems.length ? (
        <div className="faults" role="status">
          {problems.map((p) => (
            <p className="ctl-error" key={p}>
              {p}
            </p>
          ))}
        </div>
      ) : null}

      <div className="body">
        <nav className="sections" aria-label="Settings pages">
          {navGroups(pages).map((group) => (
            <div className="nav-group" key={group.group}>
              {/* A group the daemon left unnamed gets no cap rather than a
                  placeholder word: an empty header is noise, and the pages
                  below it still read as a list. */}
              {group.group ? <h2 className="nav-group-title">{humanize(group.group)}</h2> : null}
              {group.pages.map((p) => (
                <button
                  key={p.ID}
                  type="button"
                  className={p.ID === active ? 'section is-active' : 'section'}
                  aria-current={p.ID === active ? 'page' : undefined}
                  onClick={() => {
                    setActive(p.ID)
                    // A note about the page just left would otherwise sit under
                    // the next page's controls and read as if it belonged to them.
                    setNote('')
                  }}
                >
                  <span className="section-label">{p.Title || p.ID}</span>
                  {/* The first-run marker is read out of the page's own schema,
                      which is the declaration the host sorted on. Naming the
                      page here instead would make the wizard a hardcoded row. */}
                  {schemaOf(p).firstRun ? <span className="section-flag">Start here</span> : null}
                </button>
              ))}
            </div>
          ))}
        </nav>

        <main className="form">
          {page ? (
            <>
              <h2 className="group-title">{page.Title || page.ID}</h2>
              {schema.description ? <p className="page-desc">{schema.description}</p> : null}
              {controls.length === 0 ? (
                <p className="ctl-empty">This page declares no controls yet.</p>
              ) : (
                controls.map((ctl, i) => (
                  // Positional key: the list is schema-ordered and stable, and
                  // naming a control field here would couple the shell to a
                  // shape the registry owns.
                  <Fragment key={i}>{renderControl(ctl, ctx)}</Fragment>
                ))
              )}
            </>
          ) : (
            <p className="ctl-empty">
              {pages.length ? 'Pick a page to see its settings.' : 'No settings pages discovered.'}
            </p>
          )}

          {note ? (
            <p className="note">
              <span className="note-text">{note}</span>
              <button
                type="button"
                className="note-dismiss"
                aria-label="Dismiss this message"
                onClick={() => setNote('')}
              >
                Dismiss
              </button>
            </p>
          ) : null}

          {ownsTrace ? null : <Timeline logs={logs} readAt={readAt} />}
        </main>
      </div>

      {/* About block: version plus the newest shell log line, mirroring
          rectangle's version label and check-for-updates row. */}
      <footer className="about">
        {/* Version is served by the daemon; until the first status lands the
            block shows the name alone rather than a placeholder version that
            could be mistaken for the real one. */}
        <span className="version">CrossOS{status?.Version ? ` ${status.Version}` : ''}</span>
        {shellLog ? <span className="about-msg">{shellLog}</span> : null}
      </footer>
    </div>
  )
}
