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

## These ARE positionally accurate. That was not true until this round.

**An earlier version of this file said the renderer got positions wrong, and
that `--describe` was right in all three cases where they disagreed.** It was
the other way round, and the reason is worth recording because six rounds of
design work rested on it.

`ViewTree` printed `view.frame`, which is in the **parent's** coordinates —
and an AppKit tree mixes both conventions, because `isFlipped` is a per-view
property. `TopDownDocument`, `EmptyStateView`, `HairlineView` and the rail are
flipped; `NSStackView`, `NSClipView` and every control are not. So one printed
`y` meant "from the top" and the next meant "from the bottom", in the same
tree.

The consequence was not subtle. The tree put `core.home`'s title at `y=294`,
which reads as 294pt down a 700pt pane; the picture puts its first ink at
78pt. Both were believed, and the disagreement was charged to the renderer.
**A tree whose y axis changes meaning halfway down is not evidence about
anything**, and it was being used as evidence — including to certify this
renderer wrong.

The three disagreements in the table above are all explicable by it:

| what | on the live tree | in the picture |
| --- | --- | --- |
| matrix checkbox | `(537, 0, 44, 26)` in a `560x26` row | ten points low |
| safety trial text | leading-aligned, 106pt wide at x=-2 | centred |
| safety action buttons | `339x24`, `163x24`, `137x24` all at `x=0` | small chips drifting right |

A bottom-origin `y` read as a top-origin one moves things by the height of
their container, which is exactly the size of the disagreements recorded —
26pt in a row, 600pt in a stack, and a stack's own height for the buttons.

`ViewTree` now converts as it descends, so **every frame it prints is in the
window's coordinates: top-left origin, y growing downward**, and a non-flipped
ancestor is marked `TOP` so the reader can see which convention applied. With
one convention the tree and the picture agree.

There is still one offset to hold in mind, and it is not a defect: `--shots`
without `--window` renders `contentView`, which sits **52pt below the top of
the window** because that is where the toolbar is. The tree measures from the
window's top. Add the 52 and they line up.

## What the pictures are still not for

**Colour.** This host resolves `windowBackgroundColor` and
`controlBackgroundColor` to the same value — `(255, 255, 255)` — so every
page renders 97% pure white and the surface hierarchy that gives a macOS app
its depth is absent from the picture. The cause is the machine, not the app,
and `ShotRenderer.swift` records that three separate attempts to force an
appearance changed nothing measurable. **No conclusion about this app's
colours can be drawn from these images.**

**Anything a person has to like.** These are for shape, rhythm and content:
line lengths, whether a group reads as a group, whether a page says the wrong
thing. Whether it is *beautiful* is not measurable here, and this repo cannot
do that for itself.

## Measuring the pictures instead of looking at them

The images are the only artefact here that is not a claim about the app, so
the script that makes them also checks them:

```sh
python3 scripts/analyze-shot.py docs/screenshots/*.png
```

It reports where the ink actually is, the margins, and the gaps between
blocks — measured off the pixels rather than off the frames, because "the gap
between section frames is 40pt" is not what a reader sees.

`scripts/check-rhythm.py` asks the harder question: **does the spacing carry
the grouping?** This app's groups draw no box on purpose, so if the spacing
does not separate the roles, the page is a list rather than a pane. It takes
the roles from the view tree (via a `.layout.json` sidecar the shell writes
beside each PNG) and the distances from the pixels, because classifying gaps
by their own size is circular — that is the assumption under test.

It currently measures 3 of the 15 pages and reports the other 12 as not
measurable, which is itself the finding: **nine pages have a single group**, so
there is no section-to-section spacing to check on them.

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