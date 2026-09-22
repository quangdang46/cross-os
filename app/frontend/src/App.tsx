import { useEffect, useState } from 'react'
import { Service as AppService } from "../bindings/crossos/app/backend";

// CrossOS settings shell: renders Host-discovered pages (Dashboard, Keyboard,
// Windows, Finder, Activity, Safety, Plugins, Shortcuts, About, Welcome) served
// by the Go backend over the Wails service binding. No page list is hardcoded
// here — the Go Host owns discovery (§7.2); the frontend renders what it serves.

interface Page {
  ID: string;
  Title: string;
  Location: string;
  Actions: string[];
}

interface Status {
  Running: boolean;
  SafeMode: boolean;
  Killed: boolean;
  Plugins: { ID: string; Enabled: boolean; Healthy: string }[];
}

function App() {
  const [pages, setPages] = useState<Page[]>([]);
  const [active, setActive] = useState<string>('');
  const [status, setStatus] = useState<Status | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [error, setError] = useState<string>('');

  const refresh = () => {
    AppService.Pages()
      .then((p: Page[]) => {
        setPages(p);
        if (!active && p.length > 0) setActive(p[0].ID);
      })
      .catch((e: any) => setError(String(e)));
    AppService.GetStatus()
      .then(setStatus)
      .catch((e: any) => setError(String(e)));
    AppService.UILogs()
      .then(setLogs)
      .catch(() => {});
  };

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
  }, []);

  const page = pages.find((p) => p.ID === active);

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
            {page.Actions.length > 0 && (
              <ul className="actions">
                {page.Actions.map((a) => <li key={a}>{a}</li>)}
              </ul>
            )}
          </section>
        )}

        {status && status.Plugins.length > 0 && (
          <section className="page">
            <h2>Plugins</h2>
            <ul className="actions">
              {status.Plugins.map((pl) => (
                <li key={pl.ID}>{pl.ID} — {pl.Enabled ? 'enabled' : 'disabled'} ({pl.Healthy})</li>
              ))}
            </ul>
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
