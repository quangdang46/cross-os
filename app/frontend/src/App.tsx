// Shell bootstrap — intentionally minimal.
//
// The settings UI is NOT designed here. Per the project's UI policy the
// interface is ported from a reference app, not hand-written; the hand-
// written version that used to live here was removed (see git history) and
// the replacement must cite its source (settings UI in
// tmp/research/rectangle is the closest analog: a macOS window manager's
// preferences surface).
//
// Until that port lands this renders the bare shell frame so the app
// still boots and the Go binding is exercised. No styling, no invented
// layout — that is the port's job, not this file's.

function App() {
  return <main id="crossos-shell-root" />
}

export default App
