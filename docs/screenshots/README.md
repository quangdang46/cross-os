# What the app looks like right now

Fifteen PNGs, one per page the daemon serves, rendered by
`./scripts/screenshots.sh` and committed **with the change that produced them**.

That reverses what `docs/baseline/README.md` says about the React screenshots,
and the reason is the point: those were left out because nothing regenerated
them, so they went stale the moment anything changed and quietly stopped being
evidence. A picture you cannot get a fresh copy of is not a check.

Each is a **whole window** — navigation rail and content — rather than the
content pane alone. `--shots` without `--window` renders the pane, and that
single flag is why a broken navigation rail survived eight commits: every
layout defect in this repo was measured on half the app.

## Regenerating

```sh
./scripts/run.sh --no-open      # a daemon has to be running
./scripts/screenshots.sh        # writes these files
git add docs/screenshots && git commit
```

## The app itself has been run

Every check in this repo until now ran the shell **in-process** — `--shots`,
`--audit` and `--click-test` all build the window inside the same binary that
draws the pictures. That is not the same as the app running.

It has now been run as a released, double-clicked app:

```sh
swift build --package-path macos -c release
open macos/.build/release/CrossOS
```

Measured: the process is named `CrossOS`, it stays up, and the daemon stays up
beside it for as long as it is left. It was alive after two minutes, and it quit
cleanly on request.

What that does NOT prove is anything about the picture — a window existing is
not evidence of how it looks, which is the reason these PNGs exist at all.

## What is NOT in these pictures

**The title bar and the toolbar are missing**, so the search field is not in
any of them. `window.contentView` starts below the title bar, and
`--shots --window` renders that view.

Three ways of getting the chrome in were tried and all three failed:

- capturing `contentView.superview` (the theme frame) — renders the same
  picture, so the frame is not taller than the content
- pulling `contentView` out of its window into a synthetic holder —
  `Trace/BPT trap`, because a window tears down its own content view when it
  is removed
- `screencapture` — "could not create image from display", which is Screen
  Recording permission

So the toolbar has never been checked by a render in this repo, and any
statement about it in a commit message was made about a picture it was not
in. **A person looking at the real window is the only way to check it** —
`./scripts/run.sh`.

## These show the daemon's state at capture time

The pictures are of a running app, so they are of whatever that daemon
happened to be doing. `--click-test` presses PANIC STOP and then Reset
Everything and does not put the machine back — it verifies destructive
controls by causing them — so screenshots taken straight after a run show
`core.home` saying interception is off and `core.safety` reporting what
it removed.

To capture the normal state, resume first:

```sh
python3 - <<'EOF'
import json, socket
s = socket.socket(socket.AF_UNIX); s.settimeout(5)
s.connect("~/Library/Application Support/CrossOS/crossos.sock".replace("~", __import__("os").path.expanduser("~")))
s.send(b'{"jsonrpc":"2.0","method":"safety.resume","id":1}\n'); s.recv(2048)
EOF
./scripts/screenshots.sh
```

That is why three of the files change after a `--click-test` run and do
not change after anything else.

## Width varies with the page, and is pinned

Two consecutive renders of the same build are byte-identical, so the pictures
are reproducible. They were not always: navigating to `core.safety` used to
make the window **18pt wider** — a long button label widening the frame — and
back again on the way out. The width now comes from the window's own frame
rather than from its content, so all fifteen pages render 908pt wide.

## Reading them

The window is sized to its content down to a floor set by the navigation rail,
so a short page has space below it and a tall page has none. Fifteen pages, so
compare heights before concluding a page is empty.

| Page | What it shows |
| --- | --- |
| `core-home` | daemon/keyboard/profile status, and the readiness list |
| `core-profiles` | a whole configuration, applied at once |
| `core-matrix` | every chord and what it does, with per-rule switches |
| `core-rules` | rules of your own |
| `core-windows` | window snapping zones |
| `core-switcher` | the alt-tab switcher |
| `core-commands` | Finder context menu, and file-type associations |
| `core-finder` | what the Finder extension adds |
| `core-activity` | what CrossOS has seen this session |
| `core-observe` | what watching costs, stated plainly |
| `core-extensions` | the plugins, and what they ask for |
| `core-schemaForm` | the path a plugin's own config form takes |
| `core-safety` | panic stop, re-enable, reset everything |
| `core-about` | version and licence |
| `core-onboard` | the first-run wizard |