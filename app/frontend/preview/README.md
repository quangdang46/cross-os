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

## Query parameters

| Param | What it does |
|---|---|
| `?page=Home` | Land on a page other than the first. The click is the same one a person makes on the rail. |
| `?state=ready` | A set-up machine: daemon on, tap installed, profile applied. |
| `?state=fresh` | A new machine: the tap is refused, so the permission callout and the wizard's not-done steps are on screen. **The first thing a new user sees.** |
| `?state=partial` | Profile applied, one extension off — the wizard mid-flight. |
| `?measure=1` | Writes every `.ctl` row's height, resolved grid columns and each child's offset into a `<pre id="measure">`. Read it back with `--dump-dom`. |
| `?audit=1` | Writes the palette actually in force — every distinct background, ink, radius, font size and weight on the page, with the selector that produced each — into `<pre id="audit">`. |
| `?flat=1` | Drops the `@supports (color: AccentColor)` block, so the designed hex palette is visible. Chrome reports `AccentColor` as supported while resolving none of its keywords, which collapses the whole palette to the initial value. |
| `?dark=1` | Forces the dark media query to match, so both appearances can be compared from one machine set to light. |

## Screenshots

```bash
./preview/shots.sh home "state=ready&page=Home"      # -> /tmp/crossos-shots/home.png
./preview/shots.sh fresh "state=fresh&page=Welcome"
```

1100x720 at 2x — the window size from `app/main.go:85`, so what comes out is
the window as a person meets it.

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
