# The React baseline

Numbers captured from the Wails/React shell **before** the port to AppKit, on
2026-09-26 at commit `f15eab8`. Phase 1 of the port is judged against these: if
`CoreClient` cannot reproduce them from the daemon, the client is wrong, not the
number.

The screenshots that go with this (14 pages, 1100x720 at 2x) are deliberately not
committed — they are a few megabytes of binaries that go stale the moment anything
changes. The three files here are the part that can be diffed and argued with.

## Why this is a baseline at all

jsdom does not load a stylesheet, so nothing in the React test suite could see a
broken grid, a collapsing box, or a border at 1.00:1 contrast. The shell shipped all
three. Every defect this port exists to prevent was found by *measuring* rather
than by looking, and these are the measurements.

## The three files

### `webkit-palette.json` — the palette the shipped app actually resolves

Read out of a real `WKWebView`, the engine the app ships in, with no screenshot and
no pixels: a probe element is given each custom property as a background colour and
the computed value read back (`preview/webkit-probe.swift`).

Two entries are worth reading twice:

- `"--fg-primary": "rgb(0, 0, 0)"` — the stylesheet says `#1d1d1f` and annotates it
  `16.83:1`. WebKit resolves `CanvasText` to **absolute black**, so every contrast
  figure written beside that token describes a colour that never shipped.
- `"--accent": "rgb(0, 122, 255)"` — the stylesheet says `#0a66ff` (`4.82:1`).
  WebKit resolves `AccentColor` to the system blue. The fallback and the shipped
  value are different colours, and only the second one is what a user sees.

Chrome resolves none of these keywords while reporting that it supports them, which
is why the whole palette collapses there and why `?flat=1` exists in the preview
harness. **This file is the reason the port is worth doing**: the gap between "what
the stylesheet declares" and "what the engine paints" is not a CSS bug that can be
fixed in CSS. In AppKit there is no gap, because there is no stylesheet to
misdeclare anything.

### `contrast.txt` — WCAG 1.4.11 against live elements, not tokens

```json
"input.border":  { "composited": "rgb(128,128,128)", "surfaceBehind": "rgb(255,255,255)", "ratio": 3.97 }
```

Every operable control's boundary, read off the live element and the live surface
behind it. 3.97:1 clears the 3:1 that 1.4.11 asks of a control's own extent.

This is the number that was **1.00:1** before `c096d8a`. `--line-strong` had been
bridged to `ButtonBorder`, which WebKit resolves to `rgb(255,255,255)` — a white
border on a white card, invisible, introduced by the very block that added the
token to fix invisible borders. Only a real WebView showed it.

In AppKit this class of defect is not merely fixed, it is unrepresentable: an
`NSTextField` border is drawn by AppKit, and the app has no say in it.

### `geometry.txt` — every `.ctl` row's height and resolved grid

```
row h=251.2 cols=429px 0px | What is on | ctl-head@16w429 ctl-facts@16w445 …
row h=474   cols=429px 0px | Profiles   | ctl-head@16w429 ctl-value@16w429 ctl-cards@16w445
```

Produced by `?measure=1` in the preview harness. The `cols=` field is the resolved
`grid-template-columns` of each row.

Read `cols=429px 0px` carefully, because it is the whole argument in one line: the
second track is **zero pixels wide**. The layout is `minmax(0, 1fr) auto` — the label
takes the row and the value takes what it needs — and on these two pages no row has a
value, so `auto` resolves to nothing. That is correct. It is also what replaced a
constant `minmax(0, 220px)` label gutter that had left ~170px of void between a
50px label like "version" and its value, and what let a page hold a 220px-label row
beside a 1fr-label row so the label edge jumped between two rows a reader was meant
to see as one list.

`h=251.2` on Home and `h=474` on Profiles are the row heights to compare against.
Home's "What is on" row was **883.5px** before the wizard collapse was fixed — in a
720px window, so it did not fit and had to scroll to show a four-row summary.

## Recreating this

```bash
cd app/frontend
npm run shell &                                     # vite, port 5299
./preview/shots.sh <name> "state=ready&page=Home"   # screenshots, /tmp only
./preview/shots.sh <name> "state=fresh&page=Welcome"

CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless --disable-gpu --virtual-time-budget=5000 --dump-dom \
  "http://127.0.0.1:5299/preview/index.html?state=ready&page=Home&measure=1" \
  | python3 -c "import sys,re; print(re.search(r'<pre id=\"measure\">(.*?)</pre>', sys.stdin.read(), re.S).group(1))"

swiftc -O preview/webkit-probe.swift -o /tmp/webkit-probe
/tmp/webkit-probe "http://127.0.0.1:5299/preview/index.html?state=ready&page=Home"
/tmp/webkit-probe "http://127.0.0.1:5299/preview/index.html?state=ready&page=Shortcuts&flat=1" --contrast
```

`--flat=1` drops the `@supports (color: AccentColor)` block, which is how the
*designed hex* palette is made visible. In Chrome the bridge wins and hands the page
a light palette in dark mode, because Chrome claims to support `AccentColor` while
resolving none of its keywords. macOS is WebKit, and WebKit resolves them — that is
the only place the bridge is meant to apply.

Cleanup afterwards: `rm -rf /tmp/crossos-shots /tmp/crossos-shots-cdp /tmp/webkit-probe`.
