// CrossOS settings shell.
//
// Port source: rectangle (rxhanson/Rectangle), whose preferences window
// is the closest analog to a CrossOS settings surface — one fixed form of
// grouped rows, each row a control with its label beside it, with the version
// and the update affordance at the head of the form.
//
//   tmp/research/rectangle/Rectangle/Base.lproj/Main.storyboard
//     :2685-3457  the settings scene
//     :2693       the content stack: vertical, leading-aligned, spacing 10
//     :2696, :2751  two of its seven horizontal rows (:2696 :2751 :2790 :2831
//     :2848 :2942 :3262 — :2831 is hidden="YES"), control beside label
//   tmp/research/rectangle/Rectangle/PrefsWindow/SettingsViewController.swift
//     :9-39   one outlet per control on those rows (all 27 the class declares)
//     :57-61  the per-setting action: read the control, write Defaults
//   tmp/research/rectangle/Rectangle/PrefsWindow/PrefsViewController.swift
//     :11-48  one shortcut control per action (the row shape is the storyboard's)
//
// No code was copied: the reference is AppKit and this is React, so the form's
// STRUCTURE transfers, not its implementation. Tracked in
// third_party/rectangle/ATTRIBUTION.md.
//
// The nav rail below is a second port, of menumate's sidebar rather than its
// manifest: a fixed-width left column carrying a small section cap over each run
// of items (tmp/research/menumate/App/UI/MenuHubScreen.swift:94-130 the column,
// :692-712 SectionCap — a 9.5pt semibold, tracked label above a group). Its
// WIDTH is Karabiner-Elements' instead, because that is the settings sidebar
// measured against this window: 250pt ideal, sized so titles fit
// (src/apps/share/swift/Views/SidebarStyle.swift:35 and :39, at Karabiner's
// 1100pt default content size and app/main.go:85's 1100). Both ports transfer
// layout, not code; each reference's entry is in its own
// third_party/<repo>/ATTRIBUTION.md.
//
// What is CrossOS's own: the rows are not hardcoded. The Go Host discovers the
// pages and their controls (§3.6c) and this file renders whatever the Service
// serves, so a new settings page is a Go change and never a UI change. Control
// kinds live in ./controls; a page id or a control id in a conditional below
// would be a defect, not a convenience. The same rule covers the nav: its
// order, its groups and its opening page all come off the wire, because the
// Host is the only place that knows which page a fresh profile lands on.

import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { Clipboard } from '@wailsio/runtime'
// The generated bindings are named in exactly one module (lib/service), so
// this file never reaches into ../bindings by path: a page, a control and the
// shell all cross the bridge through the same seam. The controls get `service`
// — the copy checked against ServiceApi — while this file keeps the raw module
// for the three calls ServiceApi deliberately does not model (page discovery
// and the two log sources); handing it the checked copy instead would just move
// that gap rather than close it.
import { Service, service } from './lib/service'
import { humanize, plural } from './lib/format'
import { commandFor } from './controls/actions'
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
  // Symbol is the mark the daemon chose for the SECTION this page belongs to.
  // It is a property of the section and not of the page, which is why seven
  // pages in one group all carry the same one — and why the shell keeps no map
  // from a group name to a picture. The vocabulary is the daemon's, and a shell
  // that hardcoded it would be a second place a section could be renamed with
  // the mark left behind.
  Symbol: string
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
  // The mark off the first page of the group. Taken from one page because the
  // daemon declares it per section and every page in a section carries the same
  // one; a group whose pages disagreed would be a daemon problem, and this
  // reads the first rather than inventing a rule for which one wins.
  symbol: string
  pages: Page[]
}

function navGroups(pages: Page[]): NavGroup[] {
  const groups: NavGroup[] = []
  for (const page of pages) {
    const open = groups.find((g) => g.group === page.Group)
    if (open) open.pages.push(page)
    else groups.push({ group: page.Group, symbol: page.Symbol, pages: [page] })
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

// The four states the daemon can be in, and the sentence each one is named by
// (5.1). The dot beside the sentence is the only colour in the window; the
// sentence is what carries the meaning, which is the never-colour-alone rule
// applied where a colour would otherwise have been the whole message.
//
// Order matters and follows the table: `down` is tested before `degraded`
// because a daemon that is not running is not "remapping off", it is gone, and
// a stopped daemon with SafeMode set must not be reported as a warning about
// shortcuts. A plugin cannot add a fifth state here: the four are properties
// of the daemon's own status, which is what keeps this file off the page ids
// the file header rules out.
type DaemonState = 'connecting' | 'ok' | 'degraded' | 'down'

function daemonState(status: Status | null): DaemonState {
  if (!status) return 'connecting'
  if (!status.Running) return 'down'
  if (status.SafeMode || !status.Interception) return 'degraded'
  return 'ok'
}

function statusSentence(status: Status | null): string {
  switch (daemonState(status)) {
    case 'connecting':
      return 'Starting CrossOS…'
    case 'ok': {
      // The count is the daemon's own, counted from the plugin rows it serves,
      // and never from the pages: a page is not a plugin, and a page list that
      // happened to be the right length would make this number a coincidence.
      const enabled = (status?.Plugins ?? []).filter((p) => p.Enabled).length
      return `Remapping on · ${plural(enabled, 'plugin')} active`
    }
    case 'degraded':
      return 'Remapping off — keyboard shortcuts are not being applied'
    case 'down':
      return 'CrossOS is not running'
  }
}

// The path macOS itself uses, verbatim (8.2). It is a sentence rather than a
// link because the shell cannot deep-link into a row of the Privacy pane, and
// a person who is told the row by name does not need the app to press the
// button for them — the button gets them to the screen that holds the row.
const INPUT_MONITORING_PATH =
  'Open System Settings → Privacy & Security → Input Monitoring, add CrossOS, then restart the app.'

// The action id is the permission token the daemon issued and checks, which is
// a different kind of key from the ones the file header rules out: it names no
// page, control or plugin, and it already has a command in the registry. Going
// through commandFor rather than ServiceApi directly is what makes a build
// whose generated bindings predate OpenSystemSettings refuse in words instead
// of throwing over a deep link (controls/actions.ts, settingsCalls).
const OPEN_SETTINGS = 'permissions.openSettings'
const openSettings = commandFor(OPEN_SETTINGS)

// What Copy diagnostics puts on the clipboard: the status block and all four
// sources the shell reads, in one plain-text report. Plain text rather than
// JSON because the person pasting it is pasting into a bug report, and the
// version, the timestamp and the section headings are the three things a
// reader needs before any of the values.
function diagnostics(
  status: Status | null,
  pages: Page[],
  logs: string[],
  shellLog: string,
): string {
  return [
    'CrossOS diagnostics',
    `captured ${new Date().toISOString()}`,
    '',
    '== status ==',
    status ? JSON.stringify(status, null, 2) : '(daemon status unavailable)',
    '',
    `== settings pages (${pages.length}) ==`,
    pages.length ? pages.map((p) => `${p.ID} — ${p.Title || p.ID}`).join('\n') : '(none served)',
    '',
    `== activity log (${logs.length} lines) ==`,
    logs.length ? logs.join('\n') : '(empty)',
    '',
    '== shell log ==',
    shellLog || '(empty)',
  ].join('\n')
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
  // Dismissals are remembered by the text they dismissed, not by a boolean.
  // Both callouts re-derive from a 5s poll, so a boolean would be re-armed by
  // the next poll and the button would be a control that undoes itself; keying
  // on the message means what you dismissed stays dismissed, and a DIFFERENT
  // daemon message — a new fault, a different permission — still gets through.
  const [dismissedTap, setDismissedTap] = useState('')
  const [dismissedFaults, setDismissedFaults] = useState('')
  const [openingSettings, setOpeningSettings] = useState(false)
  const [copyingDiagnostics, setCopyingDiagnostics] = useState(false)

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
  // BOTH spellings, not the legacy one. `traceList` is the grace-period alias
  // (controls/index.tsx) and `pipelineTrace` is the current kind; a page
  // migrated to the current spelling would otherwise pass this line and draw
  // the same decisions a second time under the shell's own cap — the exact
  // failure the comment above exists to prevent.
  const ownsTrace = controls.some(
    (ctl) => ctl.kind === 'traceList' || ctl.kind === 'pipelineTrace',
  )
  const problems = Object.values(faults)
  // The banner is dismissed against the messages it is showing, so dismissing
  // survives the next poll and a NEW fault still arrives. Its title says the
  // daemon is up and the pages below still work, because four bridge sources
  // failing is an engineering state and the old four-paragraph block implied
  // the whole app was broken (5.1, acceptance criterion 8).
  const faultText = problems.join('\n')
  const showBanner = problems.length > 0 && faultText !== dismissedFaults
  // Same rule for the permission callout: the daemon's own string is what is
  // shown, so the string is what is remembered.
  const tapError = status?.TapError ?? ''
  const showCallout = tapError !== '' && tapError !== dismissedTap

  // The pane the grant is made in, opened through the action registry. A
  // refused call says which binding is missing rather than leaving the button
  // looking like it worked: the grant is made by hand, in a window this app
  // does not own, so nothing here can be read back as a granted permission.
  async function openSystemSettings(): Promise<void> {
    if (!openSettings) {
      setNote('This build has no way to open System Settings.')
      return
    }
    setOpeningSettings(true)
    setNote('')
    try {
      await openSettings(service)
      setNote('System Settings opened. Grant the permission it names, then restart CrossOS.')
    } catch (e) {
      setNote(`Could not open System Settings: ${message(e)}`)
    } finally {
      setOpeningSettings(false)
    }
  }

  async function copyDiagnostics(): Promise<void> {
    setCopyingDiagnostics(true)
    setNote('')
    try {
      await Clipboard.SetText(diagnostics(status, pages, logs, shellLog))
      setNote('Diagnostics copied — paste them into a bug report.')
    } catch (e) {
      // The four sources are still on screen, so a copy that failed has lost
      // nothing; it must not put the banner in the same state as a fault.
      setNote(`Could not copy diagnostics: ${message(e)}`)
    } finally {
      setCopyingDiagnostics(false)
    }
  }

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
      {/* The status strip (5.1). It replaces the masthead, which is deleted
          (5.2): the window is a native NSWindow and the OS already draws a
          title bar with the app name, so a band of logo + grey text above the
          app was the web-page-header tell. This strip is always present, so
          the daemon's state is never something you have to notice is missing.

          .statusbar-meta is defined and deliberately not filled here. 5.1's
          example puts the version in it and the footer (5.13) also keeps it,
          and the window is 1100px wide with both in frame: two strings saying
          the same number is one fact with two answers, and 5.2 puts the
          version on About and nowhere else in the chrome. P2 rewrites the
          footer and settles where it lives once. */}
      <div className="statusbar" data-state={daemonState(status)}>
        <span className="statusbar-dot" aria-hidden="true" />
        <span className="statusbar-text">{statusSentence(status)}</span>
      </div>

      {showBanner ? (
        <div className="banner" data-tone="danger" role="status">
          <p className="banner-title">
            {plural(problems.length, 'source')} unavailable — the daemon is up, so the pages below
            still work.
          </p>
          {/* The messages go in a <details>: four paragraphs of bridge errors
              is what this replaced, and a person who needs them opens them
              rather than reading past them to the button. */}
          <details className="banner-detail">
            <summary>{plural(problems.length, 'message')}</summary>
            {problems.map((p) => (
              <p key={p}>{p}</p>
            ))}
          </details>
          <div className="banner-actions">
            <button
              type="button"
              className="btn btn--sm"
              onClick={copyDiagnostics}
              disabled={copyingDiagnostics}
            >
              Copy diagnostics
            </button>
            <button
              type="button"
              className="btn btn--ghost btn--sm"
              onClick={() => setDismissedFaults(faultText)}
            >
              Dismiss
            </button>
          </div>
        </div>
      ) : null}

      <div className="body">
        <nav className="sections" aria-label="Settings pages">
          {navGroups(pages).map((group) => (
            <div className="nav-group" key={group.group}>
              {/* A group the daemon left unnamed gets no cap rather than a
                  placeholder word: an empty header is noise, and the pages
                  below it still read as a list. */}
              {group.group ? (
                <h2 className="nav-group-title">
                  {/* Every section is identified by something other than the
                      word alone. alt-tab-macos draws a symbol beside each of
                      its four settings sections and leaves none blank
                      (SettingsWindow.swift:549-554) — a sidebar where one
                      section has a mark and the next does not reads as
                      unfinished rather than as a choice. Decoration beside the
                      word, never the signal: a screen reader gets the word
                      alone, and the mark is a fixed width so the words line
                      up down the rail. */}
                  {group.symbol !== '' ? (
                    <span className="nav-group-mark" aria-hidden="true">
                      {group.symbol}
                    </span>
                  ) : null}
                  {humanize(group.group)}
                </h2>
              ) : null}
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
          {/* The measure and the scroll container are two elements (5.4): `.form`
              scrolls, `.page` is the 760px document column inside it. It is
              left-aligned, not centred — see the rule. */}
          <div className="page">
          {page ? (
            <>
              {/* An h1, because it is the top-level heading of this pane: the
                  sidebar caps are h2 over their groups and the 12px uppercase
                  cap this replaced was indistinguishable from them, so nothing
                  on screen said which of the fifteen pages you were on. */}
              <div className="page-header">
                <h1 className="page-title">{page.Title || page.ID}</h1>
                {schema.description ? <p className="page-desc">{schema.description}</p> : null}
              </div>
              {/* The permission callout (5.1), in the page's own anatomy
                  position — title, description, callout, controls (4.2) — and
                  not in the window's chrome, because above the fold on an
                  1100x720 window means above the fold of the content pane.

                  The title is 5.1's own sentence; the body names the setting
                  path macOS uses, verbatim (8.2), and the daemon's string goes
                  under it in a mono block, unparaphrased. The daemon knows
                  which permission and which call failed and the shell does
                  not, so the daemon's words are the ones that are shown. */}
              {showCallout ? (
                <div className="callout" data-tone="warn" role="status">
                  <span className="callout-icon" aria-hidden="true">
                    ⚠
                  </span>
                  <div className="callout-body">
                    <p className="callout-title">
                      CrossOS needs Input Monitoring to remap keys
                    </p>
                    <p className="callout-text">{INPUT_MONITORING_PATH}</p>
                    <p className="callout-raw">{tapError}</p>
                  </div>
                  <div className="callout-actions">
                    <button
                      type="button"
                      className="btn btn--primary"
                      onClick={openSystemSettings}
                      disabled={openingSettings}
                    >
                      Open System Settings
                    </button>
                    <button
                      type="button"
                      className="btn btn--ghost"
                      onClick={() => setDismissedTap(tapError)}
                    >
                      I&apos;ve done this
                    </button>
                  </div>
                </div>
              ) : null}
              {controls.length === 0 ? (
                <p className="ctl-empty">This page declares no controls yet.</p>
              ) : (
                /* The implicit section (4.3). A page that declares no
                   `sections[]` gets ONE card holding every row, with no card
                   header — structurally correct, and visually close to what
                   shipped. It is the fallback half of the contract: when the
                   daemon does ship sections[], they win and this is replaced by
                   one card per section. Both paths stay live, which is what
                   makes a daemon/shell version skew survivable. */
                <div className="card">
                  {controls.map((ctl, i) => (
                    // Positional key: the list is schema-ordered and stable, and
                    // naming a control field here would couple the shell to a
                    // shape the registry owns.
                    <Fragment key={i}>{renderControl(ctl, ctx)}</Fragment>
                  ))}
                </div>
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

          {/* The footer (5.13), moved INSIDE the content pane. It spanned the
              full window width under both the rail and the content, which made
              it a second horizontal rule competing with the sidebar's own for
              the same edge — and a full-width band under a 760px column is
              chrome pretending to be part of the document. The version is a
              fact, not chrome: About (110) is where it belongs, and this keeps
              it only as a convenience. */}
          <footer className="about">
            {/* Version is served by the daemon; until the first status lands the
                block shows the name alone rather than a placeholder version that
                could be mistaken for the real one. */}
            <span className="version">CrossOS{status?.Version ? ` ${status.Version}` : ''}</span>
            {shellLog ? <span className="about-msg">{shellLog}</span> : null}
          </footer>
          </div>
        </main>
      </div>
    </div>
  )
}
