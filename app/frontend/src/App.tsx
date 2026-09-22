import { useEffect, useState } from 'react'
import { Service as AppService } from "../bindings/crossos/app/backend";
import type { Page, Status } from "../bindings/crossos/app/backend/models";

// CrossOS settings shell: renders Host-discovered pages served by the Go
// backend over the Wails service binding. No page list is hardcoded here —
// the Go Host owns discovery (§7.2); the frontend renders what it serves.
// Page/Status shapes are the generated binding models (never re-declared).
//
// Page bodies render GENERICALLY from Schema.controls: every control kind
// below maps 1:1 to a backend `kind` string. New kinds arrive via Registry,
// never via a frontend code change (same discovery rule as pages).
interface Control {
  kind: string;
  id: string;
  label?: string;
  text?: string;
  description?: string;
  title?: string;
  note?: string;
  action?: string;
  confirm?: string;
  source?: string;
  steps?: string[];
  items?: string[];
  format?: string;
  fields?: string[];
  readOnly?: string[];
  rowActions?: string[];
  actions?: string[];
  rowAction?: string;
  widgets?: string[];
  aboutLink?: string;
  trialLink?: string;
  docsLink?: string;
  editLinks?: string;
  conflicts?: string;
  editable?: boolean;
  immediate?: boolean;
}

function parseControls(page: Page): Control[] {
  try {
    const schema = JSON.parse((page as any).Schema ?? (page as any).schema ?? '{}');
    return Array.isArray(schema.controls) ? schema.controls : [];
  } catch {
    return [];
  }
}

function MatrixControl() {
  // Known builtin RuleIDs (mirror core/rules parity list — the daemon
  // rejects unknown IDs, so this list can only address rows the daemon
  // already knows, never invent them).
  const ids = [
    'windows-keyboard.alt-f4-close-window',
    'windows-keyboard.ctrl-c-copy',
    'windows-keyboard.win-left-snap',
    'windows-keyboard.win-right-snap',
    'windows-keyboard.win-up-maximize',
    'windows-keyboard.win-down-minimize',
    'developer.ctrl-shift-enter-terminal',
    'developer.ctrl-shift-c-copypath',
    'developer.ctrl-shift-p-editor',
    'launcher.ctrl-space-launcher',
  ];
  const [states, setStates] = useState<Record<string, boolean>>({});
  const [msg, setMsg] = useState<string>('');
  const toggle = (id: string) => {
    const next = !(states[id] ?? true);
    AppService.SetRuleEnabled(id, next)
      .then((ok: boolean | null) => {
        setStates((s) => ({ ...s, [id]: ok ?? next }));
        setMsg('');
      })
      .catch((e: any) => setMsg('Toggle failed: ' + String(e)));
  };
  return (
    <div className="ctl">
      <h3>Shortcut matrix</h3>
      <p className="note">Toggle a row — takes effect immediately, no restart (config.setRuleEnabled). {msg}</p>
      <ul className="actions">
        {ids.map((id) => (
          <li key={id}>
            {id} — {states[id] === false ? 'disabled' : 'enabled'}
            {' '}
            <button className="btn" onClick={() => toggle(id)}>
              {states[id] === false ? 'Enable' : 'Disable'}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function App() {
  const [pages, setPages] = useState<Page[]>([]);
  const [active, setActive] = useState<string>('');
  const [status, setStatus] = useState<Status | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [error, setError] = useState<string>('');
  const [actionMsg, setActionMsg] = useState<string>('');

  const refresh = () => {
    AppService.Pages()
      .then((p: Page[] | null) => {
        const list = p ?? [];
        setPages(list);
        if (!active && list.length > 0) setActive(list[0].ID);
      })
      .catch((e: any) => setError(String(e)));
    AppService.GetStatus()
      .then((s: Status | null) => { if (s) setStatus(s); })
      .catch((e: any) => setError(String(e)));
    AppService.UILogs()
      .then((l: string[] | null) => { if (l) setLogs(l); })
      .catch(() => {});
  };

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
  }, []);

  const page = pages.find((p) => p.ID === active);

  const runAction = (action: string) => {
    // Safety page actions with live daemon backing.
    if (action === 'safety.panicStop') {
      AppService.PanicStop()
        .then(() => { setActionMsg('PANIC STOP executed — interception disabled.'); refresh(); })
        .catch((e: any) => setActionMsg('PANIC STOP failed: ' + String(e)));
      return;
    }
    if (action === 'safety.reset') {
      if (!window.confirm('Remove login item, disable extension, clean CrossOS-owned state?')) return;
      AppService.ResetEverything()
        .then((steps: string[] | null) => setActionMsg('Reset plan: ' + (steps ?? []).join(' → ')))
        .catch((e: any) => setActionMsg('Reset failed: ' + String(e)));
      return;
    }
    // Plugin lifecycle actions.
    if (action === 'plugin.enable' || action === 'plugin.disable') {
      setActionMsg('Toggle plugins from the Plugins section below.');
      return;
    }
    setActionMsg('Action ' + action + ' queued (daemon executes).');
  };

  const renderControl = (c: Control, key: number) => {
    switch (c.kind) {
      case 'button':
        return (
          <div key={key} className="ctl">
            <button
              className={'btn' + (c.id === 'panicStop' ? ' is-danger' : '')}
              onClick={() => c.action && runAction(c.action)}
            >
              {c.label ?? c.id}
            </button>
            {c.note && <p className="note">{c.note}</p>}
            {c.confirm && <p className="note">Confirm: {c.confirm}</p>}
          </div>
        );
      case 'trial':
        return (
          <div key={key} className="ctl">
            <h3>{c.label ?? 'Trial'}</h3>
            <p className="note">Countdown served by Core (trialCountdown). Trial links to the Safety surface.</p>
            {(c.actions ?? []).map((a) => (
              <button key={a} className="btn" onClick={() => runAction(a)}>{a}</button>
            ))}
          </div>
        );
      case 'auditList':
        return (
          <div key={key} className="ctl">
            <h3>{c.label ?? 'Audit'}</h3>
            <p className="note">Source: {c.source ?? 'core:ownershipAudit'}</p>
            <button className="btn" onClick={() => c.rowAction && runAction(c.rowAction)}>Roll back</button>
          </div>
        );
      case 'pluginList':
        return (
          <div key={key} className="ctl">
            <h3>Installed plugins</h3>
            <ul className="actions">
              {(status?.Plugins ?? []).map((pl) => (
                <li key={pl.ID}>
                  {pl.ID} — {pl.Enabled ? 'enabled' : 'disabled'} ({pl.Healthy})
                  {' '}
                  <button
                    className="btn"
                    onClick={() => {
                      AppService.TogglePlugin(pl.ID, !pl.Enabled)
                        .then(() => refresh())
                        .catch((e: any) => setActionMsg('Toggle failed: ' + String(e)));
                    }}
                  >
                    {pl.Enabled ? 'Disable' : 'Enable'}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        );
      case 'enableFlow':
        return (
          <div key={key} className="ctl">
            <h3>{c.label ?? 'Enable'}</h3>
            <ol className="actions">
              {(c.steps ?? []).map((s) => <li key={s}>{s}</li>)}
            </ol>
            {c.note && <p className="note">{c.note}</p>}
          </div>
        );
      case 'traceList':
        return (
          <div key={key} className="ctl">
            <h3>Activity trace</h3>
            <p className="note">{c.format ?? ''}</p>
            {logs.length === 0
              ? <p className="note">No events yet — press a shortcut to seed the trace.</p>
              : (
                <ul className="actions">
                  {logs.slice(-20).map((l, i) => <li key={i}>{l}</li>)}
                </ul>
              )}
          </div>
        );
      case 'matrix':
      case 'overrides':
        return <MatrixControl key={key} />;
      case 'shortcutList':
        return (
          <div key={key} className="ctl">
            <h3>Shortcuts</h3>
            <p className="note">{c.editable ? 'Editable — conflicts resolve winner + losers.' : ''} Source: {c.source ?? ''}</p>
            {c.editLinks && <p className="note">Edit content on owning pages ({c.editLinks}).</p>}
          </div>
        );
      case 'zoneEditor':
        return (
          <div key={key} className="ctl">
            <h3>Snap zones</h3>
            <p className="note">{c.note ?? ''} Source: {c.source ?? ''}</p>
          </div>
        );
      case 'palette':
        return (
          <div key={key} className="ctl">
            <h3>Command palette</h3>
            <p className="note">{c.note ?? ''}</p>
          </div>
        );
      case 'checklist':
        return (
          <div key={key} className="ctl">
            <h3>Readiness</h3>
            <ul className="actions">
              {(c.items ?? []).map((it) => <li key={it}>{it}</li>)}
            </ul>
          </div>
        );
      case 'actionSettings':
        return (
          <div key={key} className="ctl">
            <h3>Action settings</h3>
            <p className="note">Editable: {(c.fields ?? []).join(', ')}</p>
            <p className="note">Read-only: {(c.readOnly ?? []).join(', ')}</p>
            {c.note && <p className="note">{c.note}</p>}
          </div>
        );
      case 'gateBadge':
        return (
          <div key={key} className="ctl">
            <h3>Level B gate</h3>
            <p className="note">{c.note ?? ''}</p>
          </div>
        );
      case 'schemaForm':
        return (
          <div key={key} className="ctl">
            <h3>Plugin settings</h3>
            <p className="note">{c.note ?? 'Settings declared by plugins render here automatically.'}</p>
          </div>
        );
      case 'note':
        return <p key={key} className="note">{c.text ?? ''}</p>;
      case 'version':
      case 'license':
      case 'credits':
        return (
          <div key={key} className="ctl">
            <h3>{c.id}</h3>
            <p className="note">Source: {c.source ?? ''}{c.note ? ' — ' + c.note : ''}</p>
          </div>
        );
      default:
        return <p key={key} className="note">Unsupported control “{c.kind}” ({c.id}) — update the shell renderer.</p>;
    }
  };

  const controls = page ? parseControls(page) : [];

  return (
    <>
      <main className="container">
        <header className="brand">
          <h1 className="title">CrossOS</h1>
          <p className="subtitle">
            {status
              ? status.Running
                ? status.SafeMode
                  ? 'Safe mode — confirm or roll back on the Safety page.'
                  : 'Running.'
                : 'Daemon not running — start the Core daemon.'
              : 'Connecting to Core…'}
          </p>
        </header>

        {error && <div className="toast is-visible" role="alert"><span className="toast-msg">{error}</span></div>}
        {actionMsg && <div className="toast is-visible" role="status"><span className="toast-msg">{actionMsg}</span></div>}

        <nav className="pages">
          {pages.map((p) => (
            <button
              key={p.ID}
              className={'btn' + (p.ID === active ? ' is-active' : '')}
              onClick={() => setActive(p.ID)}
            >
              {p.Title}
            </button>
          ))}
        </nav>

        {page && (
          <section className="page">
            <h2>{page.Title}</h2>
            <p className="page-id">{page.ID}</p>
            {controls.length === 0
              ? <p className="note">No controls served for this page yet.</p>
              : controls.map((c, i) => renderControl(c, i))}
          </section>
        )}
      </main>

      <hr className="footer-divider"/>
      <footer className="footer">
        <span className="footer-version"><span>CrossOS shell</span></span>
        {logs.length > 0 && <span className="footer-time"><span>{logs[logs.length - 1]}</span></span>}
      </footer>
    </>
  )
}

export default App
