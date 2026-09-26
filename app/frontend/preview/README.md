# The shell, on its own

`npm run shell` serves the settings window in a browser at
<http://127.0.0.1:5299/preview/index.html> with no Go backend, no Wails bridge
and no daemon. It is the only way to look at this UI without building the app,
and the layout in `public/style.css` has defects that no test in the suite can
see — jsdom does not load a stylesheet, so a broken grid is green on CI.

## How it works

`preview/vite.config.ts` swaps one module: a `resolveId` hook redirects
`src/lib/service` to `preview/service-stub.ts`, which re-exports the test
fixture's stateful machine (`src/test/fixtures.ts`). Everything else is the real
shell — the real `App.tsx`, the real 27 control renderers, the real stylesheet.
The pages served are the fixture's, so they exercise every control kind.

The hook is `enforce: 'pre'` and matches the specifier `lib/service`, so it
intercepts exactly that seam. A fixture cannot quietly become the app's backend.

## The second document

`src/switcher.html` is the app's OTHER window — the overlay that appears when
the switcher chord is pressed. It is a separate entry, not a route, and nothing
in the repo had ever rendered it: the unit tests drive `SwitcherPanelControl`
directly, so they cover the panel but not the document around it.

```bash
open http://127.0.0.1:5299/preview/switcher.html?summon=1
```

`?summon=1` answers `SwitcherWait` with a trigger. Without it the overlay sits
on *"Waiting for the switcher chord."* forever, which is the honest answer —
nothing can synthesise the chord, and that absence is the point.

Two seams are swapped to get there, both in `preview/vite.config.ts`:
`src/lib/service` for the fixture, and `@wailsio/runtime` for `preview/wails-stub.ts`.
The second is not optional — the overlay awaits `Window.Show()` on every trigger,
a browser rejects it, the rejection becomes a fault, and a fault replaces the
panel. The one screen the overlay exists to show was the one screen the preview
could not show.

`preview/switcher.tsx` has no `createRoot`: `src/switcher.tsx` renders itself at
module scope, which is how the real document works, so importing it is the job.

## Query parameters

| Param | What it does |
|---|---|
| `?page=Home` | Land on a page other than the first. The click is the same one a person makes on the rail. |
| `?state=ready` | A set-up machine: daemon on, tap installed, profile applied. |
| `?state=fresh` | A new machine: the tap is refused, so the permission callout and the wizard's not-done steps are on screen. **The first thing a new user sees.** |
| `?state=partial` | Profile applied, one extension off — the wizard mid-flight. |
| `?measure=1` | Writes every `.ctl` row's height, resolved grid columns and each child's offset into a `<pre id="measure">`. Read it back with `--dump-dom`. |
| `?audit=1` | Writes the palette actually in force — every distinct background, ink, radius, font size and weight on the page, with the selector that produced each — into `<pre id="audit">`. |
| `?flat=1` | Drops the `@supports (color: AccentColor)` block, so the designed hex palette is visible. Chrome reports `AccentColor` as supported while resolving none of its keywords, which collapses the whole palette to the initial value. **Pair this with `--dark`** — see below. |
| `?comfortable=1` | Sets `[data-density='comfortable']` on `:root`, which is the attribute the daemon writes and the stylesheet reads. |
| `?measure=1&kind=<sel>` | As `?measure=1`, plus one line per element matching `<sel>` with its box, computed line-height and margins. This is how the trial timer was found carrying the UA's `1em 0`. |
| `?focus=<selector>` | Focuses one element. Note this does **not** engage `:focus-visible` on a `<button>` — a programmatic `.focus()` is not a keyboard interaction, so the shot shows no ring and proves nothing. Use `tabshot.sh`. |

Dark and high contrast are not query parameters. They are media queries, and
`shots.sh` emulates them properly over CDP:

```bash
./preview/shots.sh home "state=ready&page=Home" --dark
./preview/shots.sh keys "state=ready&page=Keyboard&flat=1" --dark --contrast
```

**`--dark` alone will not look dark in a browser, and that is not a bug in the
app.** The `@supports (color: AccentColor)` block is last on purpose — the OS
must get the last word on appearance and accent — and Chrome claims to support
`AccentColor` while resolving none of its keywords, so the block wins and hands
the page a light palette in dark mode. That is Chrome's broken, not the
cascade's. Add `flat=1` to drop the block and see the dark theme the stylesheet
actually specifies, under a real emulated `prefers-color-scheme: dark`.

macOS is WebKit, and WebKit resolves the keywords; that is the only place the
bridge is meant to apply.

## Focus, honestly

```bash
./preview/tabshot.sh "http://127.0.0.1:5299/preview/index.html?state=ready&page=Keyboard&flat=1" ring 14
# ctl-toggle | outline=2px solid rgb(10, 102, 255) offset=2px
# /tmp/crossos-shots/ring.png
```

`tabshot.sh` dispatches a real `Tab` key over CDP and reports the active element's
computed outline plus a screenshot. This is the only way to see a focus ring:
`:focus-visible` deliberately does not match a programmatic focus, so the
`?focus=` param is a trap for exactly the check it looks like it can do.

## The real engine

Everything above is Chrome. The app ships in a WKWebView, and Chrome is not it:
Chrome REPORTS supporting `AccentColor` and resolves none of its keywords, so
the whole palette there collapses to the initial value. That is a property of
Chrome, not of the app, and it is why `?flat=1` exists.

`preview/webkit-probe.swift` reads the resolved values out of a real WKWebView —
the same engine the shipped window uses — with no screenshot and no pixels:

```bash
swiftc -O preview/webkit-probe.swift -o /tmp/webkit-probe
/tmp/webkit-probe "http://127.0.0.1:5299/preview/index.html?state=ready&page=Home"
/tmp/webkit-probe "…page=Shortcuts" --contrast      # WCAG 1.4.11, live elements
/tmp/webkit-probe "…page=Home" --dark
```

**What it can and cannot settle.** It found the one defect no other harness
could: `--line-strong` was bridged to `ButtonBorder`, which reads
`rgb(255, 255, 255)` in WebKit, so every input, select and button in the
shipped app had a white border on a white card — 1.00:1 where 1.4.11 asks 3:1.
Chrome never showed it, because Chrome never resolved the keyword at all.

It CANNOT settle dark mode. Setting `NSApplication.appearance` flips
`prefers-color-scheme` but the CSS system colour keywords do not follow it in a
headless process: a bare `background: Canvas` reads white in both. A control
page (`_probe.html`, which the probe prefers over its own token list) is the
check — when it disagrees with the app, the probe is wrong, not the app.

## Screenshots

```bash
./preview/shots.sh home "state=ready&page=Home"      # -> /tmp/crossos-shots/home.png
./preview/shots.sh fresh "state=fresh&page=Welcome"
```

1100x720 at 2x — the window size from `app/main.go:85`, so what comes out is
the window as a person meets it. For the narrow branch, pass the width to Chrome
directly; `shots.sh` is fixed at the real window size on purpose.

## What the four media queries cost to check

`comfortable` density, `prefers-contrast`, `prefers-reduced-motion` and the
`max-width: 40em` collapse are all declared and none of them had ever run. The
collapse was broken: its reset was `.ctl > :not(.ctl-head)` at (0,2,0) and the
catch-all it had to undo is eight `:not()` deep at (0,9,0), so a one-column
grid kept conjuring a second column. The other three behave.

## Reading a measurement

```bash
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless --disable-gpu --window-size=1100,720 \
  --virtual-time-budget=6000 --dump-dom \
  "http://127.0.0.1:5299/preview/index.html?state=ready&page=About&measure=1" \
  | python3 -c "import sys,re,html;d=sys.stdin.read();m=re.search(r'<pre id=\"measure\">(.*?)</pre>',d,re.S);print(html.unescape(m.group(1)))"
```

`cols=220px 460px` on a row whose label is 50px of text is the shape that put a
hole down the middle of every form. `cols=680px 0px` with a 696px child is the
fixed one: content spanning the row, controls on a single right edge.
