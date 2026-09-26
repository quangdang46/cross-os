# CrossOS — Design System

> The single source of truth for how CrossOS looks, how it moves, and how it
> behaves under stress. Written against the real shell (`app/frontend`) and the
> real page inventory (15 pages, 4 groups, 27 control kinds).
>
> Researched 2026-09-26 against Mobbin: 46 screens across six patterns
> (settings shell, onboarding wizard, shortcut sheet, activity log, launcher
> overlay, conflict resolver). Every reference is linked in §14.
>
> **Architecture rule this document must not break** (`App.tsx:34-40`): the
> shell renders whatever the Go Host serves. No rule, class or component below
> may be keyed on a page id, a control id or a plugin id. Hierarchy comes from
> an *optional* schema field (§4.3), never from a lookup table in TypeScript.

---

## 1. Diagnosis — what is wrong with the current UI

The shell is not badly *styled*; it is badly *structured*. `public/style.css`
carries genuinely good decisions (a 4-step radius scale, `@supports (color:
AccentColor)` bridging to the OS palette, a `prefers-reduced-motion` block,
tabular numerals on the countdown). Those survive this rewrite. What fails is
hierarchy, rhythm, and specificity of action.

| # | Symptom | Evidence in the current code | Cost to the user |
|---|---|---|---|
| 1 | **No page title.** The page title renders as `.group-title` — 12px, 600, uppercase, `--text-dim`, `.04em` tracking | `style.css:450-457` | A 12px uppercase cap is identical to a section cap. Nothing tells you which page you are on except a 12px blue nav row. |
| 2 | **The form is a wall.** Every `.ctl` draws a full-bleed `border-bottom` across the entire content width, edge to edge, forever | `style.css:506-513` | 12 controls = 12 identical horizontal lines. Zero grouping. Langdock, Framer, beehiiv and Klaviyo all break the page into titled cards. |
| 3 | **No card container for the form itself.** `.ctl-list` and `.ctl-steps` have cards; plain controls do not | `style.css:625-633`, `998-1009` | The card concept exists in the codebase and is applied to two control kinds only. |
| 4 | **Label column is a fixed 220px, baseline-aligned** | `style.css:506-512` | Controls do not line up in a right-hand column. Two `.ctl-slider`s render at different x. |
| 5 | **Nav is 250px of 12px rows, 9.5px uppercase caps, saturated accent fill** | `style.css:299`, `328-353`, `431-435` | The 2014-web-sidebar tell. Every reference uses 13–14px rows, sentence-case groups, and a *tint* or *left bar* — never a saturated pill per page. |
| 6 | **The status line is a 12px dim corner note** | `style.css:220-231` | "Running · Remapping off" is the most important fact in the app and it is grey 12px under a 15px wordmark. |
| 7 | **`TapError` is the same 12px red line** | `style.css:228-231` | For a first-run user — the exact person this app exists for — the one line saying *"you must grant Input Monitoring"* is indistinguishable from a status blurb. |
| 8 | **Every button is identical**: 26px, 1px border, white fill | `style.css:905-923` | "Grant permission" and "Reset everything" have identical weight. The primary action is invisible. |
| 9 | **Empty states are `font-style: italic; color: text-dim`** | `style.css:743-747` | Italic grey is the universal "unfinished app" tell. Peec AI, Loop, Replit all have structured empty states with an affordance. |
| 10 | **The activity log is a monospace `<ul>` with a border per row** | `style.css:1405-1464` | Same wall. 1Password, Circle and PlanetScale all use filter chips, group headers, tabular timestamps and per-row actions. |
| 11 | **No `.kbd` component.** CrossOS is *about* shortcuts; they render as plain text or a bordered `<input>` | `style.css:1348-1359` | Perplexity, Notion Mail, Sana AI and Frame.io all render shortcuts as discrete bordered monospace chips. This is the app's signature content and it has no visual identity. |
| 12 | **The wizard is a nav list with one blue row** | `style.css:998-1130` | Reads as a list, not a flow. Semrush, Motion, Maze and Dropbox Dash all put progress at the top and one step body in the middle. |
| 13 | **No space scale.** Sixteen distinct `px` padding values: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 16, 20, 24 | whole file | Sixteen values on a 4px grid means the eye reads noise, not rhythm. |
| 14 | **`.ctl-fact` sets `opacity: 0.7` on the whole row** | `style.css:813-824` | The *value* is the content of a fact pane. Dimming it 30% fails contrast on the pane's primary text. |
| 15 | **Tiles are absolutely positioned with JS-computed geometry, `overflow: visible`** | `style.css:1160-1185` | No grid semantics, no reflow, fragile at any window size. StackAI, Frame, Gamma and Discord all use a real grid. |
| 16 | **One global focus ring**: `2px solid accent; outline-offset: 2px` on every element | `style.css:192-195` | Tabbing produces a heavy detached double-ring on every nav row, input and button. |
| 17 | **No density, contrast or reduced-transparency accommodations** | — | A window-management utility is used for hours. |
| 18 | **`--line` is 1.51:1 on white** | `style.css:22`, `376`, `548-552` | That is the resting border of `.ctl-toggle`, `.ctl-input` and `.ctl-select`. WCAG 1.4.11 wants 3:1 for the boundary of an interactive control, so today a control's own extent is below the floor. |
| 19 | **`--done #34c759` is 2.22:1 on white** | `style.css:68`, `1054-1057` | It is the fill of the wizard's "step done" disc. The check glyph carries the meaning, so this is a visibility problem rather than a semantic one — but the disc is hard to see. |
| 20 | **Everything is `px`** | whole file | A macOS app must scale with the OS text size; `px` does not. |

**What is already right and stays.** The `@supports (color: AccentColor)` block
(`style.css:104-148`) that hands palette resolution back to macOS; the radius
scale (`:45-55`); the decision that colour is never the only signal (`:591`,
`:1284`, `:1350`); the timestamp honesty in `App.tsx:160-181` — no invented
clock; the `ownsTrace` de-duplication at `App.tsx:311-313`; the per-source fault
tracking at `App.tsx:230-241`; the `Toggle`-is-a-real-checkbox argument at
`common.tsx:147-159`; the `MAX_TRACES` cap with its honest sentence
(`App.tsx:158`, `198-204`); the `--danger`-is-not-Apple-red contrast decision
(`style.css:99-102`). These are cited, not replaced.

---

## 2. Design principles

1. **One accent, spent once.** The accent marks *the current thing*: one active
   nav row, one primary button per view, one focus ring. Everything else is ink.
   A saturated fill repeated 15 times down a sidebar is how this app looks cheap.
2. **Every action is worth choosing.** Three button tiers at most (primary /
   secondary / danger). If two actions on a page look equally weighted, the page
   has no opinion — pick one.
3. **A control is a sentence about one thing.** A card has a title, a row has a
   label, a control has a state word. Never colour alone, never an icon alone.
4. **Degradation is the product.** CrossOS intercepts keystrokes. Its own UI must
   degrade loudly, locally and reversibly. Permission state, daemon state and
   per-plugin failure are first-class visual objects, not log lines.
5. **Density is a user setting.** Compact by default, Comfortable on request.
   Retrofitting it later means re-tuning sixty paddings.
6. **Nothing shells out to a page id.** A plugin that ships a new page gets
   working, well-laid-out UI with no TypeScript change (§4.3).

---

## 3. Foundations

### 3.1 Colour

Semantic tokens only. A control never reads a hex. Light values below; dark in
§11. **The `@supports (color: AccentColor)` bridge is retained verbatim** — it is
the most correct thing in the current stylesheet.

```css
:root {
  /* ---- surfaces, back to front ---- */
  --bg-window:    #ffffff;   /* the content plane                     */
  --bg-sidebar:   #f5f5f7;   /* nav rail, footer, wells               */
  --bg-raised:    #ffffff;   /* cards sitting on the content plane    */
  --bg-sunken:    #f5f5f7;   /* read-only fields, list cards           */
  --bg-hover:     rgba(0, 0, 0, .05);
  --bg-pressed:   rgba(0, 0, 0, .09);
  --bg-selected:  color-mix(in srgb, var(--accent) 12%, transparent);

  /* ---- ink: four tones, no more ---- */
  --fg-primary:   #1d1d1f;   /* row labels, values, page title     */
  --fg-secondary: #6e6e73;   /* descriptions, hints, placeholders */
  --fg-tertiary:  #86868b;   /* timestamps, caps, disabled ONLY   */
  --fg-onaccent:  #ffffff;
  --fg-link:      #0a66ff;

  /* ---- lines ----
     Alphas are ink over white, and the measured ratios are recorded because
     they are not what you would guess: 11% -> #e6e6e6 (1.25:1), 30% -> #bbb
     (1.92:1). Only --line-strong reaches the 3:1 that WCAG 1.4.11 asks of a
     control boundary, and it needs ~47% to get there. */
  --line-hairline: color-mix(in srgb, var(--fg-primary) 14%, transparent);  /* 1.33:1 — decorative only  */
  --line-default:  color-mix(in srgb, var(--fg-primary) 38%, transparent);  /* 2.36:1 — internal dividers    */
  --line-strong:   color-mix(in srgb, var(--fg-primary) 50%, transparent);  /* 3.27:1 — control boundaries    */

  /* ---- accent (bridged to the OS accent colour where supported) ---- */
  --accent:       #0a66ff;
  --accent-hover: color-mix(in srgb, var(--accent) 85%, #000);
  --accent-tint:  color-mix(in srgb, var(--accent) 12%, transparent);
  --accent-ring:  color-mix(in srgb, var(--accent) 45%, transparent);

  /* ---- state: an INK for text, a FILL for marks, a WASH for surfaces ---- */
  --ok:     #15803d;  --ok-fill:     #15803d;  --ok-wash:     rgba(21,128,61,.10);
  --warn:   #92400e;  --warn-fill:   #b45309;  --warn-wash:   rgba(180,83,9,.10);
  --danger: #b91c1c;  --danger-fill: #c8102e;  --danger-wash: rgba(200,16,46,.09);
  --live:   #15803d;  --live-fill:   #16a34a;  --live-wash:   rgba(22,163,74,.12);

  --focus-ring: var(--accent);
  --focus-width: 2px;
  --focus-offset: 2px;
}
```

Rules the current stylesheet already gets right and must keep:

- **`--live` is not `--accent` and not `--ok`.** "This machine is recording your
  keystrokes" gets its own token (`style.css:27-31`) and is not reused for
  selection. Its light value `#166534` is **7.13:1** on white and 6.55:1 on
  `--bg-sunken` — comfortably AA. Leave it alone.
- **`--danger` is not Apple's system red.** `#ff3b30` fails 4.5:1 on white
  (`style.css:99-102`). Keep the darker step. The current dark `--danger
  #ff6f81` is 6.23:1 on `#1e1e1e` and is also fine.
- **`--ok` moves off a saturated fill.** Not for the reason it first looks: the
  two greens are not the problem. The problem is that `--done #34c759` is
  **2.22:1** on white — below the 3:1 UI floor, and it is the fill of the
  wizard's "step done" disc. Resolution: `--live` keeps green (it is a live
  signal and should look like one, and it passes); `--ok` is expressed as a
  **checkmark glyph on a neutral surface**, so "derived as done" is carried by
  shape over a visible fill. `--ok #15803d` is 5.02:1 on white as ink.
- **`--line` is the real defect, not the accents.** `#d2d2d7` on white is
  **1.51:1**, and it is the border of every resting input, select and switch.
  WCAG 1.4.11 wants 3:1 for the boundary of an interactive control. The
  measurement is the awkward part: it takes **50% ink (3.27:1)**, not the 18–30%
  that looks right on screen. So the three line tokens each carry a *different*
  obligation, and only one of them is a control boundary:
  **`--line-strong` 3.27:1 for control boundaries; `--line-default` 2.36:1 for
  internal dividers, where nothing depends on seeing it; `--line-hairline`
  1.33:1 for card outlines and decoration.**
- Contrast floors, all measured on the values above: body ≥ 4.5:1; secondary
  `#6e6e73` = **5.07:1**; tertiary `#86868b` = **3.62:1** and is **timestamps,
  caps and disabled ink only** — never a value, never a row label.

### 3.2 Type

Base 13px — the macOS control size. `-webkit-font-smoothing: antialiased` stays
(`style.css:186`).

| Token | Size / weight / tracking | Used for |
|---|---|---|
| `--fs-cap` | 11 / 600 / .05em / uppercase | Card eyebrow, sidebar group label |
| `--fs-meta` | 11.5 / 400 | Hints, timestamps, footnotes, log meta |
| `--fs-small` | 12 / 400 | Secondary labels, chips, badges, small buttons |
| `--fs-body` | 13 / 400 | Row labels, values, body copy, inputs |
| `--fs-body-strong` | 13 / 590 | Row label of a *changed* or *dangerous* row |
| `--fs-card` | 15 / 600 / −.01em | Card title |
| `--fs-page` | 20 / 600 / −.02em | Page title |
| `--fs-display` | 28 / 600 / −.02em | Wizard step title, About only |
| `--fs-mono` | 12 / 400 | `.kbd`, log lines, `.is-mono` fact values |
| `--lh-body` | 1.45 | |
| `--lh-tight` | 1.2 | headings |

**The 9.5px uppercase cap is retired.** It is unreadable at 1× and is the reason
all four of the fifteen nav groups read as "web app". 11px semibold, `.05em`, sentence
case is the macOS source-list convention and what every reference shell uses.

Everything is declared in `rem` against a `html { font-size: 13px }` root, so OS
text scaling works (§9).

### 3.3 Space — a real 4px scale

Sixteen distinct padding values is the noise, and the list is a 4px grid with
almost every rung used at least once. These are the only ones allowed:

```css
--sp-0: 0;    --sp-1: 2px;   --sp-2: 4px;   --sp-3: 8px;   --sp-4: 12px;
--sp-5: 16px; --sp-6: 20px;  --sp-7: 24px;  --sp-8: 32px;  --sp-9: 40px;
--sp-10: 56px;
```

Two composite rhythms carry the layout:

- **Row rhythm** — `--row-pad-block` (8px compact / 12px comfortable),
  `--row-pad-inline: 14px`, rows separated by a 0.5px hairline.
- **Level rhythm** — 6px between a card title and its rows, 20px between cards,
  28px between the page title and the first card.

> **The one rule:** a gap *inside* one kind of thing is smaller than the gap
> *between* two kinds of thing. That single ratio is what stops a form reading as
> a wall.

### 3.4 Radius & elevation

```css
--r-xs: 4px;     /* .kbd, .badge      */
--r-sm: 6px;     /* .btn, .input, .switch track */
--r-md: 8px;     /* .card, .list, .sheet — also the existing --radius-list */
--r-lg: 10px;    /* .popover          */
--r-full: 999px; /* .badge--pill, .dot */
```

The four-step scale from `style.css:45-55` is kept and extended by one. The
existing `--radius-list: 8px` — a card *around* a list of rows, a shape none of
the four covers, which the file already uses twice (`style.css:48-55`) —
becomes `--r-md`.

Elevation, hairline-first. A macOS utility on a desktop has no elevation
problem, it has a *separation* problem:

```css
--elev-0: none;                              /* flush                        */
--elev-1: 0 0 0 .5px var(--line-hairline);   /* the hairline                 */
--elev-2: 0 1px 2px rgba(0,0,0,.06),
          0 4px 12px rgba(0,0,0,.06);        /* popover, menu                */
--elev-3: 0 8px 32px rgba(0,0,0,.18),
          0 2px 8px rgba(0,0,0,.10);         /* sheet / modal                */
```

The 0.5pt hairline is kept from the current file (`style.css:312-313`,
`630`, `1005`) — WebKit keeps the half pixel on Retina rather than rounding up,
which is the point.

### 3.5 Motion

Only two kinds, both feedback. The current position (`style.css:1492-1510`) is
correct and stays: a 120ms state change, and the 2s live-dot breathing pulse,
which is a *signal* and not decoration.

```css
--dur-instant: 0ms;
--dur-fast: 120ms;   /* hover, focus, switch, chip   */
--dur-base: 180ms;   /* popover in, accordion, sheet  */
--dur-slow: 240ms;   /* sheet content cross-fade      */
--ease-out: cubic-bezier(.2, .8, .3, 1);
--ease-in-out: cubic-bezier(.4, 0, .2, 1);
```

- Transform and opacity only. Never `width`, `height` or `top`.
- The live pulse runs as a `@keyframes` opacity cycle, never a JS re-render —
  30 React renders a second to animate one property is exactly what
  `style.css:680-688` argues against, and that argument is kept.
- `prefers-reduced-motion: reduce` collapses both, via the existing global block
  (`style.css:1502-1510`) — **with one correction**: `animation-iteration-count: 1`
  must be dropped for `.live-dot`, whose entire meaning is the cycle. Under
  reduced motion it becomes a static ring at 0.6 opacity — still visibly on,
  never animated.
- **Nothing animates on first paint.** No entrance animation on page change, no
  staggered list reveal. A settings window that fades in feels slow.

---

## 4. Information architecture

### 4.1 The real inventory

15 pages, 4 groups, from `app/backend/*.go`:

| Group (cap) | Symbol | Pages (order) |
|---|---|---|
| **Home** | `⌂` | Welcome `core.onboarding` (firstRun, 0) · Home `core.home` (0) · Profiles `core.profiles` (10) |
| **Shortcuts** | `⌘` | Keyboard (20) · My Rules (25) · Windows (30) · Switcher (35) · Shortcuts (40) · Explorer `core.finder` (50) |
| **Activity** | `◷` | Activity (60) · Observe (70) |
| **Advanced** | `⚙` | Extensions (80) · Plugin Settings (90) · Safety (100) · About (110) |

Symbols are daemon-supplied and currently render as **text glyphs in a 1.1em box**
(`style.css:360-366`) — `⌂ ⌘ ◷ ⚙` at 9.5px is a smudge. They become real 16px
glyphs in a fixed 20px column, decorative, `aria-hidden`, with the section name
carrying every bit of the meaning.

### 4.2 Page anatomy

Every page is: **title → description → callout (conditional) → sections → inline
note → activity (only if the page declares no trace control).**

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Keyboard                                  ┌──────────────────────────┐   │
│  Remap Windows shortcuts so they work                                 ●  │   │
│  the same on macOS.                                                Live  │   │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ ⚠ CrossOS needs Input Monitoring to remap keys                    │   │   │
│  │   Open System Settings → Privacy & Security → Input Monitoring.  │   │   │
│  │                        [ Open System Settings ]  [ I've done it ]│   │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  WINDOWS CLIPBOARD                                                        │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ Copy                                            ⌃C        ( ●─── ) │   │
│  │ Maps Ctrl+C to ⌘C everywhere except the terminal.                 │   │
│  ├────────────────────────────────────────────────────────────────────┤   │
│  │ Cut                                             ⌃X        ( ●─── ) │   │
│  ├────────────────────────────────────────────────────────────────────┤   │
│  │ Paste in terminal                               ⌃⇧V        ( ●─── ) │   │
│  │ Terminal is exempt from clipboard remapping.                     │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  FINDER                                                                   │
│  ┌────────────────────────────────────────────────────────────────────┐   │
│  │ Rename file                          F2         [Rec] [ Record ] │   │
│  └────────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ✓ Copy saved · ⌃C → ⌘C                                          Dismiss   │
└──────────────────────────────────────────────────────────────────────────┘
```

- **Page title** — 20/600 (`--fs-page`), one line, `text-overflow: ellipsis`.
- **Page description** — 13px `--fg-secondary`, `max-width: 68ch`, 4px below.
- **Max content width 760px, left-aligned.** Not centred: this is a document
  pane, and a centred measure in an 1100px window leaves two dead gutters.
- Above 760px the right gutter is deliberately empty. A settings pane that fills
  its window edge to edge is a spreadsheet.

### 4.3 How a page declares structure — the only legal mechanism

The shell stays schema-driven. Hierarchy is a **new optional field**, not a
lookup. A page that declares nothing still gets the implicit single section.

```jsonc
// PageSchema — additive. Every field optional; absence must render, not fail.
{
  "description": "Remap Windows shortcuts so they work the same on macOS.",
  "firstRun": false,

  "sections": [                      // NEW — optional
    { "id": "clipboard", "title": "Windows clipboard",
      "description": "…optional one-liner…",
      "controls": [ /* Control[] */ ] }
  ],

  "controls": [ /* legacy: wrapped into one implicit, untitled section */ ]
}
```

And three optional fields on a `Control`, all of which the renderer already has
the information for and currently discards:

```jsonc
{ "kind": "toggle", "id": "clip.copy", "label": "Copy",
  "help": "Maps Ctrl+C to ⌘C everywhere except the terminal.",  // NEW
  "tone": "normal",              // NEW: "normal" | "danger"
  "group": "clipboard" }         // NEW: flat alternative to sections[]
```

Contract:

- `sections[]` wins when present; `controls[]` is the fallback for a daemon that
  has not shipped it. Both paths are always live — that is what makes a
  daemon/shell version skew survivable.
- A page with **no** section titles renders every row in one implicit card with
  no card header. Visually close to today, structurally correct. A Go page that
  ships without sections looks plain; it never looks broken.
- **The shell may not branch on a section id.** `id` exists for anchor links and
  test selection only.

---

## 5. Shell components

### 5.1 Status — the single most important fix

The current `.masthead` / `.status` / `.status.is-error` (`style.css:205-231`)
renders the app's most consequential state as a 12px grey note. Replaced by a
**status strip** under the title bar, always present, whose content and colour
track the daemon's actual state.

```html
<div class="statusbar" data-state="ok|degraded|down|connecting">
  <span class="statusbar-dot" aria-hidden="true"></span>
  <span class="statusbar-text">Remapping on · 3 plugins active</span>
  <span class="statusbar-meta">CrossOS 0.4.1</span>
</div>
```

| `data-state` | Trigger | Dot | Copy |
|---|---|---|---|
| `connecting` | `GetStatus` unresolved | pulsing `--fg-tertiary` | "Starting CrossOS…" |
| `ok` | `Running && Interception` | solid `--ok-fill` | "Remapping on · N plugins active" |
| `degraded` | `Running && !Interception` **or** `SafeMode` | solid `--warn-fill` | "Remapping off — keyboard shortcuts are not being applied" |
| `down` | `!Running` | solid `--danger-fill` | "CrossOS is not running" + **Start** button |

**`degraded` and `down` get an action.** A status that names a condition and
offers no fix is a complaint, not a design.

```html
<div class="callout" data-tone="warn">
  <svg class="callout-icon" aria-hidden="true">…</svg>
  <div class="callout-body">
    <p class="callout-title">CrossOS needs Input Monitoring to remap keys</p>
    <p class="callout-text">Open System Settings → Privacy &amp; Security →
      Input Monitoring, add CrossOS, then restart the app.</p>
  </div>
  <div class="callout-actions">
    <button class="btn btn--primary">Open System Settings</button>
    <button class="btn btn--ghost">I've done this</button>
  </div>
</div>
```

`status.TapError` (the daemon's own string, `App.tsx:339`) becomes `callout-text`
when present and `callout-title` when absent. The daemon's message is verbatim,
never paraphrased — and it is the fallback that guarantees this callout is never
empty.

**References:** Maze and Motion permission screens (a titled card naming what is
needed and why, one primary CTA), Dropbox Dash (per-item connect rows), Pin (a
step with a provider list and a single Continue).

```css
.statusbar {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  min-height: 32px;
  padding: 0 var(--sp-7);
  background: var(--bg-sidebar);
  border-bottom: var(--elev-1);
  font: var(--fs-small)/1 var(--font);
}
.statusbar-dot {
  width: 8px; height: 8px;
  border-radius: var(--r-full);
  background: var(--fg-tertiary);
  flex: none;
}
.statusbar[data-state='ok']        .statusbar-dot { background: var(--ok-fill); }
.statusbar[data-state='degraded']  .statusbar-dot { background: var(--warn-fill); }
.statusbar[data-state='down']      .statusbar-dot { background: var(--danger-fill); }
.statusbar[data-state='connecting'] .statusbar-dot { animation: breathe 2s var(--ease-in-out) infinite; }
.statusbar-text  { color: var(--fg-primary); }
.statusbar-meta  { margin-left: auto; color: var(--fg-tertiary); font-variant-numeric: tabular-nums; }

.callout {
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr) auto;
  gap: var(--sp-3);
  align-items: start;
  margin: 0 0 var(--sp-5);
  padding: var(--sp-4);
  border: 1px solid color-mix(in srgb, var(--warn) 28%, transparent);
  border-radius: var(--r-md);
  background: var(--warn-wash);
}
.callout[data-tone='danger'] {
  border-color: color-mix(in srgb, var(--danger) 30%, transparent);
  background: var(--danger-wash);
}
.callout-title { margin: 0 0 var(--sp-1); font: 590 var(--fs-body)/1.4 var(--font); }
.callout-text  { margin: 0; font: var(--fs-small)/1.5 var(--font); color: var(--fg-secondary); }
.callout-actions { display: flex; gap: var(--sp-2); align-self: center; }
```

**The `faults` block** (`App.tsx:342-350`) stops being a bare list of red
paragraphs. Four bridge sources failing is an *engineering* state, not a user
state: it collapses to a single dismissible `.banner` (`--danger-wash`, 13px)
with the messages in a `<details>`, and a **Copy diagnostics** button that puts
status + all four logs on the clipboard. If the daemon is up but the bridge is
down, the settings still work — the UI must say that rather than imply the app
is broken.

### 5.2 Title bar

The window is a native Wails/NSWindow: the OS already draws a title bar with the
app name. **`.masthead` is deleted.** What replaces it:

- A 44px row carrying the **page title** and the **status pill**, inside the
  content pane rather than spanning it.
- The sidebar gets its own 44px row with the app mark and the `⌘K` hint.
- The window `title` stays "CrossOS"; the sidebar top row shows
  `CrossOS <version>` only on About and is blank elsewhere.

This removes the "web page header" tell entirely: no horizontal band of logo +
grey text sitting above the app.

### 5.3 Sidebar

The dominant reference pattern: Featurebase, Graphite, Framer, Google Drive,
Langdock, Klaviyo, Fireflies, Peec AI, Dropbox Dash, Clerk, Plain, Teachable,
Workable, beehiiv.

```css
.sidebar {
  flex: none;
  width: 232px;
  display: flex;
  flex-direction: column;
  background: var(--bg-sidebar);
  border-right: var(--elev-1);
  overflow-y: auto;
  overscroll-behavior: contain;
}
```

**Why 232px and not the current 250px.** 250 is defensible in the abstract, but
the rows are now 13px with a 16px icon and 12px inline padding, and the trailing
`nav-item-badge` ("Start here" on Welcome) needs room. 232 fits icon + label +
badge with 12px of slack; 250 left the badge looking cramped in a wide empty
rail. It stays **fixed**, not resizable: this rail holds 15 declared pages a
person can read in one glance, and a resizer implies a length it does not have.

```css
.sidebar-group-label {
  padding: var(--sp-4) var(--sp-3) var(--sp-1);
  font: 600 11px/1.3 var(--font);
  letter-spacing: .05em;
  text-transform: uppercase;
  color: var(--fg-tertiary);
}
.nav-group + .nav-group .sidebar-group-label { padding-top: var(--sp-5); }

.sidebar-group-icon {
  display: inline-grid;
  place-items: center;
  width: 20px;
  margin-right: var(--sp-1);
  color: var(--fg-secondary);
  font-size: 13px;
  line-height: 1;
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  width: 100%;
  min-height: var(--nav-min-height, 30px);
  padding: var(--sp-1) var(--sp-3);
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--fg-primary);
  font: var(--fs-body)/1.4 var(--font);
  text-align: left;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-out);
}
.nav-item:hover { background: var(--bg-hover); }

.nav-item[aria-current='page'] {
  background: var(--bg-selected);          /* tint, NOT a saturated fill */
  font-weight: 590;
}
.nav-item[aria-current='page']::before {   /* 3px leading bar, macOS list idiom */
  content: '';
  position: absolute;
  left: 0; top: 50%;
  width: 3px; height: 16px;
  border-radius: 0 var(--r-full) var(--r-full) 0;
  background: var(--accent);
  transform: translateY(-50%);
}
```

Three changes, each with a reason:

1. **Tint + leading bar replaces the full-saturation blue fill.** Fifteen
   saturated pills down a rail is the loudest thing in the current UI. The tint
   still reads as selected; the bar survives greyscale and
   `prefers-contrast: more`.
2. **13px rows at 30px.** The current 12px/4px-padding row is ~20px tall, below
   the comfortable minimum and the reason the rail reads as dense clutter.
3. **11px uppercase caps, not 9.5px** — and a 20px icon column so titles align
   down the rail. The existing `nav-group-mark` logic (`App.tsx:370-374`,
   `style.css:360-366`) is right; only its size changes.

The `section-flag` "Start here" badge (`App.tsx:395`, `style.css:407-416`) keeps
its pill shape, re-tuned to `--fs-small` / `--r-full` / `--bg-sunken`. On the
selected row it loses its wash and keeps the on-accent ink (`style.css:424-427`).

**No inline search field.** The existing argument (`style.css:271-289`) is
correct: 15 pages in 4 groups is a list a person can read, and a filter over it
promises navigation power the surface does not have. **But** the argument is
about *filtering the rail*, and the missing thing is different — ⌘K quick jump
(§5.9), which every top reference has.

### 5.4 Content pane

```css
.content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;      /* no 15px jump when a page overflows */
  padding-bottom: var(--sp-9);
}
.page { max-width: 760px; padding: var(--sp-5) var(--sp-7) 0; }
.page-header { margin-bottom: var(--sp-6); }
.page-title {
  margin: 0;
  font: 600 var(--fs-page)/var(--lh-tight) var(--font);
  letter-spacing: -.02em;
}
.page-description {
  margin: var(--sp-1) 0 0;
  max-width: 68ch;
  font: var(--fs-body)/var(--lh-body) var(--font);
  color: var(--fg-secondary);
}
```

**There is no top padding on `.content`.** The first thing in the scroll area is
the page title at `--sp-5` below the strip. A band of nothing above a heading is
what makes a settings pane read as a web page.

### 5.5 Card & row — the structural fix

This replaces `.ctl`'s full-bleed `border-bottom` wall.

```css
.card {
  margin: 0 0 var(--sp-5);
  background: var(--bg-raised);
  border: var(--elev-1);
  border-radius: var(--r-md);
  overflow: clip;                 /* keeps row hairlines inside the radius */
}
.card-header { padding: var(--sp-4) var(--sp-5) var(--sp-3); }
.card-title {
  margin: 0;
  font: 600 var(--fs-card)/var(--lh-tight) var(--font);
  letter-spacing: -.01em;
}
.card-description {
  margin: var(--sp-1) 0 0;
  font: var(--fs-small)/var(--lh-body) var(--font);
  color: var(--fg-secondary);
}
.card-body { border-top: var(--elev-1); }   /* only when there IS a header */
.card-body--flush { border-top: 0; }

.row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) var(--control-col, 200px);
  align-items: center;                        /* centre, not baseline */
  gap: var(--sp-4) var(--sp-5);
  min-height: var(--row-min-height, 44px);
  padding: var(--row-pad-block, var(--sp-4)) var(--sp-5);
  border-bottom: var(--elev-1);               /* hairline INSIDE the card */
}
.row:last-child { border-bottom: 0; }

.row-label {
  font: var(--fs-body)/var(--lh-body) var(--font);
  color: var(--fg-primary);
}
.row-help {                                   /* NEW — from Control.help */
  margin: var(--sp-1) 0 0;
  font: var(--fs-meta)/var(--lh-body) var(--font);
  color: var(--fg-secondary);
}
.row-value {
  justify-self: end;                          /* the RIGHT EDGE aligns */
  min-width: 0;
  font: var(--fs-body)/var(--lh-body) var(--font);
  color: var(--fg-primary);
  text-align: right;
  overflow-wrap: anywhere;
}
.row--wide  { --control-col: 1fr; }          /* full-width value */
.row--stack { grid-template-columns: minmax(0, 1fr); }
```

Four changes, each load-bearing:

1. **Rows live inside a card; hairlines live inside the card.** *This is the fix
   for the wall.* Between cards there is 20px of whitespace and no line at all.
2. **`--control-col: 200px` with `justify-self: end`.** Every switch, select and
   button in a card now shares a right edge. The current 220px label column does
   the opposite — the *labels* align and the controls scatter.
3. **`align-items: center`, not `baseline`** (`style.css:511`). A 22px switch
   baseline-aligned against 13px text sits visibly high.
4. **`.row--wide` and `.row--stack`.** `Ctrl+Alt+Delete delay` is a number field;
   it wants 320px, not 200px. A rule clause is a sentence; it stacks. The current
   code does this with `.ctl > :not(.ctl-head) { grid-column: 2 }`
   (`style.css:525-527`) — the new grid expresses it as a modifier class instead
   of a child selector, which is why `.ctl-head` can go away.

`Control.help` lands under the label at `--fs-meta`. This is the largest content
win available: CrossOS's controls (`Ctrl+C`, `Alt+Tab`, Input Monitoring) are
*only* explicable in a sentence, and today that sentence does not exist in the
schema. Optional — a control without it renders exactly as today.

### 5.6 Controls

#### Switch

```css
.switch {
  appearance: none;
  position: relative;
  width: 38px; height: 22px;
  margin: 0;
  border: 1px solid var(--line-strong);
  border-radius: var(--r-full);
  background: var(--bg-sunken);
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out);
}
.switch::after {                    /* the knob */
  content: '';
  position: absolute;
  top: 2px; left: 2px;
  width: 16px; height: 16px;
  border-radius: var(--r-full);
  background: #fff;
  box-shadow: 0 1px 2px rgba(0,0,0,.20);
  transition: transform var(--dur-fast) var(--ease-out);
}
.switch:checked, .switch[aria-checked='true'] {
  background: var(--accent);
  border-color: var(--accent);
}
.switch:checked::after { transform: translateX(16px); }
```

Retains the current decision wholesale: a real `<input type="checkbox">`, so it
is focusable, space-activatable and announced as a switch with zero hand-written
ARIA (`common.tsx:147-159`), and the knob's *position* is the signal so it reads
with no colour perception at all (`style.css:591`). Three deltas:

1. **38px, not 40px.** 38×22 is the macOS switch; 40px is not a size anything
   on the platform uses, and it is why the control reads as slightly custom. The
   knob geometry is already correct — the current 16px knob at `left: 2px`
   travelling 18px in a 38px content box (`style.css:365-372`, `383-390`,
   `border-box` at `:53-55`) leaves a symmetric 2px gap at each end — so only
   the track width changes, and the knob keeps travelling 16px.
2. **A resting knob shadow** (`0 1px 2px rgba(0,0,0,.20)`). macOS switches have
   one; without it the knob reads as a hole punched in the track.
3. **A `--line-strong` resting border**, not `--line`. The current border is
   `#d2d2d7` at **1.51:1** — below the 3:1 WCAG 1.4.11 floor for a control
   boundary, so today the switch's own extent is hard to see.

#### Buttons — three tiers

The current single `.ctl-button` (`style.css:905-923`) is the reason nothing
stands out.

```css
.btn {
  appearance: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-2);
  min-height: var(--control-height, 28px);
  padding: 0 var(--sp-4);
  border: 1px solid var(--line-default);
  border-radius: var(--r-sm);
  background: var(--bg-raised);
  color: var(--fg-primary);
  font: 400 var(--fs-body)/1 var(--font);
  white-space: nowrap;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out);
}
.btn:hover  { background: var(--bg-sunken); }
.btn:active { background: var(--bg-pressed); }

/* primary — at most ONE per view. */
.btn--primary {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--fg-onaccent);
  font-weight: 510;
}
.btn--primary:hover { background: var(--accent-hover); border-color: var(--accent-hover); }

/* danger — destructive only. Reset, delete, uninstall. */
.btn--danger {
  color: var(--danger);
  border-color: color-mix(in srgb, var(--danger) 40%, transparent);
}
.btn--danger:hover { background: var(--danger-wash); }

/* ghost — a tertiary action inside a card. */
.btn--ghost {
  background: transparent;
  border-color: transparent;
  color: var(--fg-secondary);
}
.btn--ghost:hover { background: var(--bg-hover); color: var(--fg-primary); }

/* stop — stopping a recorder is not the mirror of starting one. */
.btn--stop {
  color: var(--danger);
  border-color: color-mix(in srgb, var(--danger) 45%, transparent);
}
.btn--stop:hover { background: var(--danger-wash); }

.btn--sm { min-height: 24px; padding: 0 var(--sp-3); font-size: var(--fs-small); }
.btn--icon { padding: 0; width: var(--control-height, 28px); }
```

`.btn--stop` preserves `style.css:738-741` (Karabiner's `.destructive` +
`stop.fill`): stopping a recorder is not the inverse of starting one.

**Disabled:** `opacity: .45; cursor: default`, hover suppressed
(`style.css:935-947` is right and stays). A disabled control that still takes a
hover fill looks live and refuses the click.

#### `.kbd` — the new signature component

CrossOS's entire product is a mapping between two keyboards. The current UI
renders `Ctrl+C` as plain text or a bordered `<input>`; every reference with
shortcuts in it (Perplexity, Notion Mail, Sana AI, Frame.io, Magnific, Height)
renders them as discrete bordered monospace chips.

```css
.kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 var(--sp-2);
  border: 1px solid var(--line-default);
  border-bottom-width: 2px;         /* the physical key edge */
  border-radius: var(--r-xs);
  background: var(--bg-raised);
  color: var(--fg-primary);
  font: 500 var(--fs-small)/1 var(--font-mono);
  white-space: nowrap;
}
.kbd--accent { border-color: var(--accent); color: var(--accent); }
.kbd--lg { height: 28px; min-width: 28px; font-size: var(--fs-body); }
.kbd-chord { display: inline-flex; align-items: center; gap: 3px; }
.kbd-chord-sep { color: var(--fg-tertiary); font-size: var(--fs-small); }
```

A chord is a **row** of chips, never one string: `⌃` `C`. A 22px chip is
scannable and `Ctrl+C` is not — and the whole reason the app exists is that
these are two distinguishable physical keys.

Applied to: `shortcutList`, `keymapEditor`, `zoneEditor`, `ruleBuilder`,
`palette`, `matrix`, `conflictResolver`, and every `.row` whose control is a
chord. **This is the single highest quality-per-line change in the document.**

#### Inputs, selects, sliders

- `.input` — `--control-height`, `--r-sm`, `--line-default` border,
  `--bg-raised` fill, 0 10px padding. Focus: `border-color: var(--accent)` plus
  `box-shadow: 0 0 0 3px var(--accent-ring)`. Placeholder `--fg-tertiary`.
- `.select` — as today, but the chevron becomes an inline SVG in `--fg-tertiary`.
  The current chevron is two `linear-gradient`s (`style.css:558-566`) which
  cannot be coloured independently of `currentColor` — which is the only reason
  the `background-image: none` disabled-select workaround at `style.css:951-953`
  has to exist.
- `.slider` — `accent-color: var(--accent)` **plus a numeric `.input` beside
  it**, so the value is readable and typeable rather than drag-only.
- `.textarea` — `--bg-raised`, `--r-sm`, `resize: vertical`, min 72px. Needed by
  `note` and `overrides`, which have nowhere to put a sentence today.

#### Badges & chips

```css
.badge {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  padding: 1px var(--sp-2);
  border-radius: var(--r-full);
  background: var(--bg-sunken);
  font: 500 var(--fs-small)/1.6 var(--font);
  color: var(--fg-secondary);
  white-space: nowrap;
}
.badge--ok     { background: var(--ok-wash);     color: var(--ok); }
.badge--warn   { background: var(--warn-wash);   color: var(--warn); }
.badge--danger { background: var(--danger-wash); color: var(--danger); }
.badge--live   { background: var(--live-wash);   color: var(--live); }
```

Every badge **must** carry a word. A coloured dot with no label is banned
app-wide — the current code already argues this three times (`style.css:591`,
`1284-1289`, `1350-1353`) and that argument becomes a lint rule, not a comment.

### 5.7 Live indicator

`style.css:695-736` is a good port of Karabiner's `CaptureActiveLabel` and is
kept: a 9px filled dot breathing on a 2s cycle, green, beside the control that
stops the capture, dimmed-and-still for the waiting state. The colour is fine —
`--live #166534` is **7.13:1** on white. Three changes, none of them about
contrast:

- Bump to `--fs-body` (13px). At 11px next to a 13px row label it reads as a
  printing artefact rather than a state.
- Wrap it in `.badge--live`. Not for legibility — the dot already passes — but
  because a bare 9px dot on white has no *weight*, and this is the one indicator
  on which the app's privacy story rests. A pill reads as a status; a dot reads
  as a bullet.
- Add the count: `● Recording · 12 keys`. A recorder that has captured nothing in
  four seconds is either broken or the user has not typed — and today nothing on
  screen distinguishes those two.

### 5.8 Wizard

Replaces `.ctl-steps` (`style.css:998-1130`) — a nav list with one blue row. The
reference pattern (Semrush, Motion, Maze, Dropbox Dash, Evernote, Langdock) is:
**progress at the top, one step body in the middle, one primary CTA at the
bottom.**

```html
<section class="wizard">
  <ol class="wizard-progress" aria-label="Setup progress">
    <li class="wizard-step" data-state="done">
      <span class="wizard-step-mark" aria-hidden="true">✓</span>
      <span class="wizard-step-label">Install the extension</span>
    </li>
    <li class="wizard-step" data-state="current" aria-current="step">…</li>
    <li class="wizard-step" data-state="todo">…</li>
  </ol>

  <div class="wizard-body">
    <h3 class="wizard-title">Grant Input Monitoring</h3>
    <p class="wizard-text">CrossOS reads the keys you press so it can rewrite
      them. It never sends them anywhere.</p>
    <div class="wizard-actions">
      <button class="btn btn--primary">Open System Settings</button>
      <button class="btn btn--ghost">Skip for now</button>
    </div>
  </div>
  <p class="wizard-rollup" aria-live="polite">Step 2 of 4</p>
</section>
```

```css
.wizard-progress {
  display: flex;
  gap: var(--sp-2);
  margin: 0 0 var(--sp-6);
  padding: 0;
  list-style: none;
}
.wizard-step {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  min-width: 0;
  font: var(--fs-small)/1.3 var(--font);
  color: var(--fg-tertiary);
}
.wizard-step[data-state='current'] { color: var(--fg-primary); font-weight: 590; }
.wizard-step-mark {
  flex: none;
  display: grid;
  place-items: center;
  width: 18px; height: 18px;
  border: 1.5px solid currentColor;
  border-radius: var(--r-full);
  font-size: 10px;
}
.wizard-step[data-state='done'] .wizard-step-mark {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--fg-onaccent);
}
.wizard-body {
  padding: var(--sp-6);
  background: var(--bg-raised);
  border: var(--elev-1);
  border-radius: var(--r-md);
}
.wizard-title {
  margin: 0 0 var(--sp-2);
  font: 600 var(--fs-display)/1.2 var(--font);
  letter-spacing: -.02em;
}
.wizard-text { margin: 0 0 var(--sp-5); max-width: 56ch; color: var(--fg-secondary); }
.wizard-actions { display: flex; gap: var(--sp-3); }
.wizard-rollup { margin: var(--sp-3) 0 0; font: var(--fs-meta)/1.4 var(--font); color: var(--fg-tertiary); }
```

The horizontal progress rail (Semrush, Evernote) beats a vertical list for 4–6
steps. **The mark is a check when done, a ring when not** — shape, not colour
(`style.css:1042-1052` already argues this; the rule holds). `aria-current="step"`
announces position; the roll-up is a polite live region.

### 5.9 ⌘K quick jump

Not a rail filter (§5.3) — a modal index over **pages and controls**, which
Featurebase, Langdock, Klaviyo, Framer, Height, Perplexity, Graphite and
Magnific all have. For a 27-kind, 15-page app it is how a power user reaches
`keymapEditor` without the mouse.

```html
<div class="sheet" role="dialog" aria-modal="true" aria-label="Jump to">
  <div class="sheet-input">
    <svg aria-hidden="true">…</svg>
    <input class="sheet-field" placeholder="Jump to a setting…" aria-controls="sheet-list" />
    <kbd class="kbd">esc</kbd>
  </div>
  <ul class="sheet-list" id="sheet-list" role="listbox">
    <li class="sheet-group-label">Shortcuts</li>
    <li class="sheet-row" role="option" aria-selected="true">
      <span class="sheet-row-title">Copy</span>
      <span class="sheet-row-hint">Keyboard</span>
      <span class="sheet-row-keys"><kbd class="kbd">⌃</kbd><kbd class="kbd">C</kbd></span>
    </li>
  </ul>
  <div class="sheet-footer">
    <kbd class="kbd">↑</kbd><kbd class="kbd">↓</kbd> navigate
    <kbd class="kbd">↵</kbd> open · <kbd class="kbd">esc</kbd> close
  </div>
</div>
```

Frame.io, StackAI, Adobe, Magnific and Discord are the references: search field
pinned top, rows of `title + hint + trailing keys`, a footer of key hints, a
scrim behind. Results are a **flat union of pages and controls**, grouped by the
same cap as the rail — so ⌘K is "the rail, but with the values in it too".

Selection: prefix match on the label, then on `help`, then on the value. `↑`/`↓`
roving `aria-activedescendant`, `↵` activates, `esc` closes, `⌘K` toggles. No
fuzzy-match highlighting in v1 — the ordering is enough.

### 5.10 Activity log

Replaces `.timeline` (`style.css:1405-1464`) and `.ctl-stages`.

```html
<section class="log" role="log" aria-label="Activity">
  <header class="log-header">
    <h3 class="log-title">Activity</h3>
    <div class="log-filters">
      <button class="filter-chip" aria-pressed="true">All</button>
      <button class="filter-chip" aria-pressed="false">Remap</button>
      <button class="filter-chip" aria-pressed="false">Window</button>
      <button class="filter-chip" aria-pressed="false">Errors</button>
    </div>
    <div class="log-actions">
      <label class="follow"><input class="switch" type="checkbox" checked /> Follow</label>
      <button class="btn btn--ghost btn--sm">Copy</button>
      <button class="btn btn--ghost btn--sm">Clear</button>
    </div>
  </header>
  <p class="log-cap" aria-live="polite">Showing the 200 most recent of 1,284 events.</p>
  <ol class="log-list">
    <li class="log-day">Today</li>
    <li class="log-row" data-tone="ok">
      <time class="log-time" datetime="2026-09-26T14:02:11.481Z">14:02:11</time>
      <span class="log-dot" aria-hidden="true"></span>
      <span class="log-msg">remapped <kbd class="kbd">⌃C</kbd> → <kbd class="kbd">⌘C</kbd></span>
      <span class="log-target">Terminal</span>
    </li>
  </ol>
</section>
```

References: 1Password (filter dropdowns, sortable columns, per-row overflow),
Circle (filter chip row above the list), PlanetScale (add-filter popover), 7shifts
(per-row `Show Details` disclosure + coloured verb chip), Square (date group
headers), Customer.io (type / name / timestamp columns).

Kept from the current code and **not** up for negotiation:

- **The cap and the honest sentence** — `MAX_TRACES = 200` with "Showing 200 of
  1,284 events" (`App.tsx:158`, `198-204`). Every reference caps too; a log that
  silently truncates is the one thing a log must not do.
- **No invented clock** — `splitStamp` (`App.tsx:173-181`) parses the daemon's own
  timestamp and leaves the row unstamped when there is none. This is why the
  time column is `max-content`, not a fixed width.
- **One page, one log** — `ownsTrace` (`App.tsx:311-313`) suppresses the shell
  timeline when a page declares a trace control. Both spellings
  (`traceList`, `pipelineTrace`) stay checked.

New: `--tone` gives a 6px dot (`--ok` / `--warn` / `--danger` / neutral) **and**
a word in the message. The `PipelineTraceControl` stage list (`.ctl-stages`,
`style.css:1305-1325`) becomes a per-event expandable detail with the stages as
a definition list, so a decision is one click away instead of a second screen.

`aria-live="off"` on the list — a 200-row live region would read 200 lines on
every 5s poll. The cap sentence is the live region.

### 5.11 Switcher overlay

`src/switcher.html` + `.ctl-tile` (`style.css:1160-1216`) is the worst offender
structurally: absolute positioning with geometry computed in JavaScript
(`SwitcherPanelControl`'s header documents the arithmetic), `overflow: visible`
on the grid, no grid semantics.

Replace with a real grid and a search field. References: Frame, StackAI, Gamma,
Adobe, Evernote, Discord — search pinned on top, then a grid of tiles each with
icon, name and secondary line.

```css
.switcher {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  min-height: 100%;
  padding: var(--sp-4);
}
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--sp-3);
}
.tile {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  min-height: 76px;
  padding: var(--sp-3);
  border: 1px solid var(--line-default);
  border-radius: var(--r-md);
  background: var(--bg-raised);
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-out),
              border-color var(--dur-fast) var(--ease-out);
}
.tile:hover:not(:disabled) { background: var(--bg-sunken); }
.tile[aria-selected='true'] {
  border-color: var(--accent);
  box-shadow: inset 0 0 0 1px var(--accent);
  background: var(--bg-sunken);
}
```

The tile **keeps** the daemon's word "Selected" beside its name
(`style.css:1156-1158`) — the highlight is a border, a fill *and* a word, never
the border alone. `role="grid"` / `row` / `gridcell` with `aria-selected`, arrow
navigation, and the geometry arithmetic in `SwitcherPanelControl` deleted
outright. Column count is a pure function of width: 1 at <520px, 2 at <820px,
`auto-fill` above.

### 5.12 Inline note

The current `.note` + `.note-dismiss` (`style.css:466-492`, `App.tsx:424-436`)
is a grey paragraph with a bordered Dismiss button. Becomes a toast-style inline
confirmation under the row that changed, tinted `--ok-wash`, auto-dismissing
after 4s — with the text also landing in the activity log, so nothing is lost.

```html
<p class="note" data-tone="ok" role="status">
  <svg class="note-mark" aria-hidden="true">✓</svg>
  <span class="note-text">Copy saved · ⌃C → ⌘C</span>
  <button class="note-dismiss btn btn--ghost btn--sm">Dismiss</button>
</p>
```

`role="status"` is a polite live region: a save is announced without stealing
focus. An **error** note uses `data-tone="danger"`, `role="alert"`, and does not
auto-dismiss.

### 5.13 Footer

`.about` (`style.css:1468-1489`) currently spans the full window width under both
the rail and the content. It becomes a sticky content-pane footer, inside the
760px measure:

- Left: `CrossOS 0.4.1`, plus an **Update available** pill when the daemon says
  so.
- Right: the shell log line, `--fs-meta`, `--fg-tertiary`, single-line,
  ellipsised, `title`-tooltipped for the full text.

A full-width band under a 760px column is a second horizontal rule competing with
the sidebar's. The version is a fact, not chrome — About (110) is where it
belongs; the footer keeps it only as a convenience.

---

## 6. Per-page designs

| Page | Group | Composition |
|---|---|---|
| **Welcome** `core.onboarding` | Home | `wizard` as the whole page (§5.8). Four steps: install extension → grant Input Monitoring → grant Accessibility → pick a profile. Carries `firstRun` so the rail badges it (`App.tsx:395`). |
| **Home** `core.home` | Home | Status bar + `homeSummary` stat grid (Remapping / Plugins / Conflicts) + a `checklist` of next actions. The one page allowed a 2-up grid at ≥900px (§7.2). |
| **Profiles** `core.profiles` | Home | `profileList` — a `.card` per profile; the active one gets a `✓` badge and an `--accent` border. Apply is the single primary. |
| **Keyboard** `core.keyboard` | Shortcuts | 3–4 cards (Clipboard, Editing, Navigation, Text). Every row: label + `help` + `.kbd` chord + switch. **The canonical page — the reference implementation for §4.3.** |
| **My Rules** `core.myRules` | Shortcuts | `ruleBuilder` — `.ctl-clause` becomes `.clause`: `--bg-sunken`, 3px `--accent` leading border, `min-height: 44px` so it reads as a sentence. `keymapEditor` beneath in a `row--stack`. |
| **Windows** `core.windows` | Shortcuts | `matrix` — a switch grid (Win+←/→/↑/↓, Win+D/M) as `.card` rows. |
| **Switcher** `core.switcher` | Shortcuts | The inline switcher preview (§5.11) + behaviour rows. |
| **Shortcuts** `core.shortcuts` | Shortcuts | `shortcutList` — the `.kbd` showcase. Grouped by action family, 11px caps, chords right-aligned in a fixed column so the eye reads a column. Perplexity / Notion Mail. |
| **Explorer** `core.finder` | Shortcuts | `menuList` (Finder right-click: New File, Copy Path, Open Terminal) + `fileTypeList`. |
| **Activity** `core.activity` | Activity | `pipelineTrace` as the page body — the §5.10 log with stage detail expanded per event. No shell timeline (already handled by `ownsTrace`). |
| **Observe** `core.observe` | Activity | `observeToggle` with the live indicator (§5.7) + `auditList`. **This is the page where the recording affordance must be loudest** — it is the privacy surface. |
| **Extensions** `core.extensions` | Advanced | `pluginList` — a `.card` per plugin with a switch, a state badge and a one-line description. |
| **Plugin Settings** `core.schemaHelp` | Advanced | `schemaForm` — the generic form. Proves §4.3: a page that declares nothing still lays out correctly. |
| **Safety** `core.safety` | Advanced | `license` + Safe Mode + **Reset Everything**. The one page with a danger zone: a `.card--danger` with a 1px `--danger` border and a single `.btn--danger`. |
| **About** `core.about` | Advanced | `version`, `credits`, `palette` — a 640px column, `--fs-display` name, version as a large fact, links as `.btn--ghost`. The only page allowed to break the 760px left alignment. |

---

## 7. Layout

### 7.1 Breakpoints

The window is 1100×720 by default (`app/main.go:85`) and resizable.

| Width | Behaviour |
|---|---|
| **≥ 900px** | Rail 232px fixed, content 760px max, left-aligned. Optional 2-up stat grid on Home. |
| **640–899px** | Identical. The content never reflows before the rail does — 232 + 760 + 2×24 = 1040 and the window is 1100 by default. |
| **< 640px** | Rail collapses to a 48px icon-only column (the daemon's 16px glyphs are already there). `.row` drops to one column, control `justify-self: start`. |
| **< 420px** | Rail collapses to 0, reached by a hamburger in the page header. Only reachable by dragging the window to ~420pt, which on macOS means display scaling. It must not break; it is not designed. |

Height is never breakpoint-driven. Every pane scrolls independently
(`overscroll-behavior: contain`) and the window itself does not scroll.

### 7.2 The 2-up grid

Klaviyo, Langdock and Dropbox Dash put every setting in a **full-width card** —
no two-up. Framer and beehiiv use two-up only for genuinely paired short
settings. CrossOS follows the first rule. The only 2-up in the app is the Home
stat grid, where each tile is a self-contained fact and nothing has to be read
across a gutter.

### 7.3 Density

`data-density="comfortable" | "compact"` on `:root`, persisted by the daemon
(the shell has no storage of its own — `Service` is its only persistence seam):

| Token | Comfortable | Compact |
|---|---|---|
| `--row-pad-block` | `--sp-4` (12px) | `--sp-3` (8px) |
| `--row-min-height` | 44px | 34px |
| `--control-height` | 28px | 24px |
| `--nav-min-height` | 30px | 26px |
| `--page-pad-x` | `--sp-7` (24px) | `--sp-5` (16px) |

**Compact is the default.** A window-management utility is used for long
stretches, and 44px rows × 40 rows is 1,760px of scroll.

---

## 8. States

| State | Trigger | Treatment |
|---|---|---|
| **Default** | — | Per §5.6. |
| **Hover** | pointer | `--bg-hover` on rows/nav; `--bg-sunken` on buttons/tiles. |
| **Focus-visible** | keyboard | `outline: var(--focus-width) solid var(--focus-ring)`, offset **per component**: 0 on `.input`/`.select` (ring hugs the border, `box-shadow 0 0 0 3px var(--accent-ring)`), 2px on `.btn`/`.nav-item`/`.tile`, and **not on `.row` at all** — a row is a container, only its controls take focus. The global `offset: 2px` (`style.css:192-195`) is what is being fixed. |
| **Pressed** | pointer down | `--bg-pressed`. |
| **Disabled** | `disabled` | `opacity: .45`, `cursor: default`, hover suppressed. Always paired with a word: the switch's `reason` title (`common.tsx:166-176`) becomes a **visible `.row-help` line**, not a tooltip — a tooltip is unreachable by keyboard. |
| **Pending write** | a `service` call in flight | Control dims to 0.45, a `--fg-tertiary` "Saving…" replaces the trailing helper, `aria-busy`. The word is currently scattered across controls; it becomes a single shared `.row-pending`. |
| **Error (control)** | a `service` call rejected | `.row--error`: a 3px `--danger` leading bar, `.row-help` replaced by the message in `--danger` at `--fs-meta`, `role="alert"`. **The row does not collapse or vanish** — losing a control because it failed is the worst outcome. |
| **Error (page)** | every source for a page failed | `.empty` + `.callout[data-tone="danger"]` + **Copy diagnostics**. |
| **Empty** | `EmptyState` | §8.1. |
| **Loading** | first poll unresolved | **Skeletons, not spinners** — four `.skeleton` bars in the shape of the cards. First load only: subsequent polls (`App.tsx:291-295`, 5s) mutate in place and **never** show a loading state, because a settings window that flashes a spinner every 5 seconds is worse than one that shows the last good value. |
| **Success** | a write resolved | `.note[data-tone="ok"]` (§5.12), auto-dismiss 4s. |
| **Danger** | `tone: "danger"` | `.row--danger` label ink, `.btn--danger` actions, `--danger-wash` card. |
| **Offline** | `!status.Running` | Status strip goes `down`; the content pane dims to 0.6 under a non-blocking overlay ("CrossOS is not running — settings are read-only"). A settings window that lets you flip switches which do nothing is lying. |

### 8.1 Empty states

`ctl-empty` is `font-style: italic; color: var(--text-dim)` (`style.css:743-747`).
Replaced by a structured empty state — the pattern from Peec AI, Loop, Replit and
Langdock.

```html
<div class="empty">
  <div class="empty-mark" aria-hidden="true"><svg width="32" height="32">…</svg></div>
  <p class="empty-title">No remapping rules yet</p>
  <p class="empty-text">Rules turn Windows shortcuts into macOS ones. Add one and
    it applies immediately — nothing restarts.</p>
  <button class="btn btn--primary">Add a rule</button>
</div>
```

Three rules:

1. **Never italic.** Italic grey text is the "unfinished app" tell.
2. **Every empty state names the thing and offers the action that fills it.** The
   exception is genuinely terminal emptiness ("No activity recorded yet"), which
   gets title + text and no button.
3. **The mark is a 32px glyph in `--fg-tertiary`, decorative.** It carries no
   meaning a colour-blind or screen-reader user would lose, and it is the only
   place in the app a large illustration is allowed.

### 8.2 Error copy

- Say what happened, then what to do. Never both, never neither.
- Name the actual setting path, verbatim from macOS:
  `System Settings → Privacy & Security → Input Monitoring`.
- Never blame the user. "Input Monitoring is off", not "You haven't enabled…".
- No exclamation marks, ever.
- The daemon's own error string is shown **verbatim** in a `--font-mono`
  `--bg-sunken` block under a human sentence. The daemon knows things the shell
  does not, and paraphrasing is where the meaning is lost.

---

## 9. Accessibility

The current stance — colour is never the only signal, the focus ring is never
removed, the switch is a real checkbox, the tile says "Selected" in words — is
correct and is extended into a checklist.

- [x] **Never `outline: none`.** The current rule stays.
- [ ] **Per-component focus geometry** (§8). The global `2px / offset 2px` is
      replaced, not removed.
- [ ] **`.row` is not focusable.** Only the control inside it is.
- [ ] **Every `.switch` has an accessible name matching its visible label.** The
      current `aria-label` (`common.tsx:185`) is right, but a control labelled
      "Copy" to a screen reader and "Copy to clipboard" on screen is a defect —
      associate with `id`/`for` or make them identical.
- [ ] **Every icon-only button has `aria-label`** (`note-dismiss` already does,
      `App.tsx:430`).
- [ ] **`.kbd` is `aria-hidden` when it duplicates a visible label**, exposed when
      it is the only representation (`shortcutList` rows, the ⌘K sheet).
- [ ] **Wizard** progress is an `<ol>` with `aria-current="step"`; the roll-up is
      `aria-live="polite"`.
- [ ] **The log** is a `role="log"` region with `aria-live="off"`; filter chips
      are `aria-pressed`; the count sentence is the live region.
- [ ] **Colour is never the only signal** — enforced by lint, not by comment.
- [ ] **`prefers-contrast: more`** — `--line-*` gain ~6 points of alpha,
      `--fg-tertiary` → `--fg-secondary`, the selected nav tint gains a 2px solid
      border.
- [ ] **`prefers-reduced-transparency: reduce`** — `--bg-sunken` and every
      `rgba(...)` wash become their opaque equivalents.
- [ ] **Text scaling** — `html { font-size: 13px }` with everything in `em`/`rem`,
      so `⌘+` works. The current stylesheet mixes `px` throughout, which is a
      genuine defect: `WKWebView` only honours `NSFont` scaling through relative
      units.
- [ ] **`.ctl-fact` loses `opacity: .7`** (`style.css:822`). The *label* goes
      `--fg-secondary`; the *value* stays `--fg-primary`. Dimming a fact's value
      fails contrast on the pane's most important text.

---

## 10. Content & voice

- **Sentence case everywhere.** `Remap Windows shortcuts`, not `REMAP WINDOWS
  SHORTCUTS`. Uppercase is reserved for the 11px caps and nothing else.
- **No period on a label.** `Copy`, not `Copy.`
- **Verb-first buttons.** `Open System Settings`, `Add a rule`, `Reset
  everything`. Never `OK`, never `Submit`, never `Yes`.
- **Second person, present tense.** "CrossOS needs Input Monitoring", not
  "Input Monitoring is required by CrossOS".
- **Numbers are tabular** (`style.css:956-961` is already correct). Any figure
  that changes in place — the countdown, the event count, the key-capture count —
  gets `font-variant-numeric: tabular-nums`.
- **No emoji in the UI.** The `LOCK` glyph forces text presentation with U+FE0E
  for exactly this reason (`common.tsx:108`); that discipline extends to every
  glyph. The daemon's `Symbol` field is an SF-Symbol-style character set, not
  emoji.

---

## 11. Dark mode

Dark is not a hex swap. The current dark block (`style.css:153-167`) is right in
principle — the light accent and danger fall below 4.5:1 on near-black and each
gets its own step. The changes:

```css
@media (prefers-color-scheme: dark) {
  :root {
    --bg-window:   #1e1e1e;
    --bg-sidebar:  #1c1c1e;
    --bg-raised:   #252528;      /* raised ABOVE the window plane */
    --bg-sunken:   #2a2a2c;
    --bg-hover:    rgba(255, 255, 255, .06);
    --bg-pressed:  rgba(255, 255, 255, .10);

    --fg-primary:   #f5f5f7;
    --fg-secondary: #a1a1a6;     /* the light #6e6e73 would be 3.29:1 here */
    --fg-tertiary:  #8a8a8f;     /* 4.6:1 — timestamps and caps only     */

    /* 33% is the 3:1 point on #1e1e1e, so --line-strong clears it with room */
    --line-hairline: rgba(255, 255, 255, .12);   /* 1.44:1 — decorative  */
    --line-default:  rgba(255, 255, 255, .22);   /* 2.18:1 — dividers    */
    --line-strong:   rgba(255, 255, 255, .45);   /* 4.40:1 — control edge */

    --accent:       #5b9bff;
    --accent-hover: #7aafff;
    --ok:     #4ade80;  --ok-fill:     #30d158;
    --warn:   #fbbf24;  --warn-fill:   #d97706;
    --danger: #ff6f81;  --danger-fill: #ff453a;
    --live:   #4ade80;  --live-fill:   #30d158;

    --elev-2: 0 1px 2px rgba(0,0,0,.5), 0 4px 12px rgba(0,0,0,.45);
    --elev-3: 0 8px 32px rgba(0,0,0,.6), 0 2px 8px rgba(0,0,0,.5);
  }
}
```

**The current dark mode is compliant, and this section does not pretend
otherwise.** Measured: `--text #f5f5f7` = 15.31:1, `--text-dim #98989d` =
**5.81:1**, `--accent #5b9bff` = 6.02:1, `--live #4ade80` = 9.57:1, `--danger
#ff6f81` = 6.23:1, `--sel-bg #1f5fbf` against white = 6.09:1. Every tier passes
AA on `#1e1e1e`. (The *light* `#6e6e73` is never used in dark mode — the dark
block at `style.css:153-167` correctly overrides it — so the 3.29:1 that value
would score is not in play.)

What changes is the **third tier**, which dark mode currently lacks: it has
`--text` and `--text-dim` and nothing dimmer, so every timestamp, section cap and
disabled ink is drawn at full secondary weight. `--fg-tertiary: #8a8a8f` is
**4.85:1** and adds that tier without a contrast regression.

The `@supports (color: AccentColor)` bridge (`style.css:104-148`) stays and gains
`--bg-window: Canvas`, `--bg-raised: ButtonFace`, `--accent: AccentColor` — the
whole point being that a macOS app follows the user's accent colour and
appearance, and the current file already knows this and bridged only half the
tokens.

---

## 12. Migration

### 12.1 Class map

Nothing is deleted before its replacement ships. The stylesheet is edited in
phases, not rewritten.

| Current | New | Note |
|---|---|---|
| `.window` | `.shell` | unchanged anatomy |
| `.masthead`, `.masthead h1` | — | **deleted** (§5.2) |
| `.status`, `.status.is-error` | `.statusbar[data-state]` + `.callout` | §5.1 |
| `.faults` | `.banner[data-tone="danger"]` | gains `<details>` + Copy diagnostics |
| `.sections` | `.sidebar` | 250px → 232px |
| `.nav-group-title` | `.sidebar-group-label` | 9.5px → 11px |
| `.nav-group-mark` | `.sidebar-group-icon` | 1.1em → 16px in a 20px column |
| `.section`, `.section.is-active` | `.nav-item[aria-current="page"]` | fill → tint + 3px bar |
| `.section-flag` | `.nav-item-badge` | |
| `.form` | `.content > .page` | adds the 760px measure |
| `.group-title` | `.page-title` **and** `.card-title` | one class becomes two roles |
| `.page-desc` | `.page-description` | |
| `.note`, `.note-dismiss` | `.note[data-tone]`, `.note-mark` | + `role="status"` |
| `.ctl` | `.card > .row` | **the structural change** |
| `.ctl-head` | — | deleted; the label is a grid child now |
| `.ctl-label` | `.row-label` | |
| `.ctl-value` | `.row-value` | gains `justify-self: end` |
| — | `.row-help` | **new**, from `Control.help` |
| `.ctl-item` | `.row` (inner) or `.list-row` | context-dependent |
| `.ctl-input` / `.ctl-select` | `.input` / `.select` | |
| `.ctl-slider` | `.slider` | |
| `.ctl-toggle` | `.switch` | 40px → 38px, resting shadow |
| `.ctl-list` | `.list` | |
| `.ctl-chip` | `.badge` | |
| — | `.kbd` | **new** (§5.6) |
| — | `.btn` + `--primary` / `--danger` / `--ghost` / `--stop` / `--sm` | **new**; `.ctl-button` keeps its name and gains tiers |
| `.ctl-empty` | `.empty` | loses italic |
| `.ctl-error` | `.row--error` + `.row-help` | |
| `.ctl-facts*` | `.facts*` | loses `opacity: .7` |
| `.ctl-cards`, `.ctl-card` | `.card` | **unified** — profile cards and form cards are one component |
| `.ctl-caps`, `.ctl-cap` | `.list-row`, `.list-row-mark` | |
| `.ctl-steps`, `.ctl-step*` | `.wizard*` | |
| `.ctl-rollup` | `.wizard-rollup` | |
| `.ctl-tile-*` | `.tile*` | grid, not absolute |
| `.ctl-stage*` | `.log-detail dt/dd` | |
| `.ctl-clause` | `.clause` | |
| `.ctl-recorder` | `.recorder` | |
| `.ctl-countdown` | unchanged | |
| `.ctl-link` | `.link` | |
| `.tl*`, `.timeline` | `.log*` | |
| `.about` | `.content-footer` | |

`.ctl-` is the current styling contract (`common.tsx:1-12`) and it is retired
deliberately: `ctl` said *"this is a control"*, and this design's central claim is
that **a control is not the layout unit — a row inside a card is**. The prefix
becomes `row-` / `card-` / `list-`, named for what they draw rather than for the
registry that draws them. A clean break in one pass beats a six-month
dual-vocabulary migration.

### 12.2 Phases

Each is shippable and leaves the app coherent.

**P0 — Foundations, no markup change.** Token block, `@supports` bridge, the
dark-mode contrast fix, the density attribute, `prefers-contrast` and
`prefers-reduced-transparency`. *Ships alone; the app looks better immediately
and nothing else has to land at the same time.*

**P1 — Status.** `.statusbar` + `.callout` + `.banner`. Delete `.masthead`.
*This is the phase that fixes the first-run experience, and it touches only
`App.tsx:328-350`.* The `Open System Settings` action needs one new service method
on the Go side — a small, isolated addition.

**P2 — Shell.** Sidebar rewrite, content measure, page title, footer. Delete
`.group-title`'s 12px-uppercase role; introduce `.card` / `.row` / `.card-title`.

**P3 — Controls.** `.btn` tiers, `.switch`, `.badge`, `.input`/`.select`, the new
`.row-help`, and the `sections[]` / `help` / `tone` schema fields with the Go
pages updated to use them. *Largest phase; the type scale and card/row structure
are already in place so the rest is mechanical.*

**P4 — The `.kbd` component**, applied to all seven control kinds that emit key
specs. *Highest perceived-quality-per-line in the document.*

**P5 — Feature surfaces.** Wizard, log + pipeline trace, switcher grid, ⌘K sheet,
empty states, toasts.

**P6 — Delete.** The old vocabulary, in one commit, once P5 has been open a week.

### 12.3 Effort

| Phase | Go | TSX | CSS | Notes |
|---|---|---|---|---|
| P0 | — | — | 2 d | Tokens + media queries |
| P1 | 0.5 d (one service method) | 0.5 d | 1 d | |
| P2 | 1 d (`sections[]` on 15 pages) | 1 d | 2 d | |
| P3 | 1 d (`help`/`tone` on ~60 controls) | 2 d | 2 d | |
| P4 | — | 1 d | 0.5 d | |
| P5 | — | 3 d | 2 d | ⌘K is ~1 d of the 3 |
| P6 | — | 0.5 d | 0.5 d | |
| **Total** | **3 d** | **8 d** | **8 d** | ~3 calendar weeks |

### 12.4 Tests that must change

- `renderers.test.tsx`, `wizardSettle.test.tsx`, `firstRun.test.tsx`,
  `checklist.test.tsx` and the other control tests assert on `className` strings.
  All break at P2 and are updated in the same commit.
- `TestEveryRegisteredKindIsReachable` (Go, `app/backend`) is unaffected — no kind
  is added or removed.
- **New:** a test that every control kind renders inside a `.card`, and a test
  that **no rule in `style.css` contains a page id, a control id or a plugin id**
  — the mechanical guard on the rule `App.tsx:34-40` states and this document
  repeats.

---

## 13. Acceptance criteria

Each is checkable in a browser.

1. The page title is 20px semibold and is the first thing in the content pane. No
   12px uppercase cap is doing that job.
2. **No control in the app has a full-bleed `border-bottom` outside a card.**
3. Every switch, select and button in a card shares a right edge.
4. Exactly one element per view is filled with the accent as a primary action.
5. The rail's selected row is a 12%-accent tint plus a 3px bar, not a saturated
   pill.
6. With `Running=false, Interception=false, TapError="…"`, a titled callout with
   a primary action is visible **above the fold without scrolling**, and the body
   text names `System Settings → Privacy & Security → Input Monitoring` verbatim.
7. Every keyboard shortcut renders as one or more `.kbd` chips. There is no bare
   `Ctrl+C` string anywhere in the UI.
8. With all sources failing, the app shows a dismissible banner with a **Copy
   diagnostics** button — not four red paragraphs.
9. The activity log has ≥2 filter chips, date group headers, tabular timestamps,
   and a sentence stating exactly how many of how many events are shown.
10. Every text tier passes 4.5:1 in **both** schemes, and every control boundary
    reaches 3:1 — `--line-strong` in particular, which is 1.51:1 today
    (`#d2d2d7` on white) and is the border of every resting input, select and
    switch.
11. `⌘K` opens a modal listing all 15 pages and every control label, grouped as
    the rail is, and `↵` navigates.
12. The switcher overlay is a CSS grid; window resize reflows it; the window
    picker is reachable by keyboard alone.
13. **Zero italic text in the app.**
14. The wizard shows a progress rail, one step body and one primary CTA.
15. `grep -nE 'core\.(home|keyboard|safety|onboarding)' app/frontend/public/style.css`
    is empty.

---

## 14. Reference index

All Mobbin, retrieved 2026-09-26.

**Settings shell — sidebar + card + hairline row**
[Workable](https://mobbin.com/screens/2e0de63f-d64f-4080-9ea7-fdb3fbb02fc8) (trailing row actions) ·
[Featurebase](https://mobbin.com/screens/32b42868-60b6-460b-80f8-c9fb51fca937) (tinted rail, card row) ·
[beehiiv](https://mobbin.com/screens/704dcfea-b99c-4278-b7b2-4cb0e4c74e3f) (grouped cards) ·
[Plain](https://mobbin.com/screens/dd2f590c-33ec-4d0d-bdcf-72d9a82760d6) (trailing Save) ·
[Langdock](https://mobbin.com/screens/e2a6a79a-d254-43c1-8156-f89a20fa7a4f) (row help, dependent rows) ·
[Teachable](https://mobbin.com/screens/9c100494-356e-470f-bc43-9a7d7caa63e8) (label / description / control columns) ·
[Graphite](https://mobbin.com/screens/102736c8-9a2a-4185-a155-025f1c0b3217) (20px card titles) ·
[Google Drive](https://mobbin.com/screens/c026c57c-8eba-4d55-b68f-d99ebc4416f3) (radio groups, dark, minimal chrome) ·
[Clerk](https://mobbin.com/screens/64495603-2e48-4214-922d-2022463a27e2) (floating unsaved-changes bar) ·
[Framer](https://mobbin.com/screens/cd5de83c-ee9e-4467-aad4-121427f9b9cf) (page-title scale, hairline rows) ·
[Dropbox Dash](https://mobbin.com/screens/2b07de77-af07-4067-b7b2-c92886e4fbe8) (grouped setting blocks) ·
[Klaviyo](https://mobbin.com/screens/3323972c-bc73-4c96-94d1-aeb089303716) (card + save button) ·
[Fireflies](https://mobbin.com/screens/6e50a822-69c1-447c-95cf-29bdb47fae80) (all-caps sub-sections in a card) ·
[Peec AI](https://mobbin.com/screens/9d19e227-3e12-40ec-9f28-6d8946fa2092) (trailing Save, empty export state)

**Onboarding / permission**
[Evernote](https://mobbin.com/screens/87bb81a8-73e4-4c6e-9d79-2208e6307b01) (dot progress, option cards, one CTA) ·
[Pin](https://mobbin.com/screens/78b19bb6-a48d-4d53-ae00-30bceb069204) (Step 1 of 7, provider list) ·
[Langdock](https://mobbin.com/screens/99e5dc52-e679-4df9-a29e-71d4554736be) (Back / Continue footer) ·
[Semrush](https://mobbin.com/screens/18940aca-263d-4882-8c9d-25f67be9ece2) (numbered segmented progress) ·
[Maze](https://mobbin.com/screens/8cf9682a-28ba-4659-9c3c-3424ed51af2e) (permission card with items + Continue) ·
[Motion](https://mobbin.com/screens/0c8f5852-5996-4614-ad84-59b20aed9fcf) (permission list: icon, name, why) ·
[Chatbase](https://mobbin.com/screens/c9f4e75a-8f24-4fad-9cb0-f473e581b65c) (checklist progress with spinner states) ·
[Dropbox Dash](https://mobbin.com/screens/dd58254c-c915-4015-8b7b-4908e86dd35f) (connect rows, illustration)

**Keyboard shortcuts**
[Perplexity](https://mobbin.com/screens/b0a63cb0-ff2e-49a9-90ab-b4dff7f8084d) (kbd chips, grouped by cap, right-aligned) ·
[Notion Mail](https://mobbin.com/screens/c5a80598-edc0-4d59-9e16-492c36ab89d5) (dense two-column sheet) ·
[Sana AI](https://mobbin.com/screens/cdd5147b-b244-4f90-a2ff-b08bfbc720a9) (three-column grid, icon + kbd) ·
[Frame.io](https://mobbin.com/screens/7bf0c9af-dd42-4511-a35e-15e097df461b) (grouped searchable sheet) ·
[Magnific](https://mobbin.com/screens/14ceb943-f04a-460f-b4f5-2ebd78d74aff) (⌘K palette, footer key hints) ·
[Retool](https://mobbin.com/screens/9f838167-8bdd-4d7a-8c71-f2588318a77b) (syntax docs inline in settings) ·
[Height](https://mobbin.com/screens/227ea05e-2f27-4ad6-8bbf-fa8915d896f3) (searchable full-page shortcut list)

**Activity log**
[1Password](https://mobbin.com/screens/a5632ec1-f072-43e2-8f2b-ad047df4ca04) (filter row, sortable columns, row overflow) ·
[Circle](https://mobbin.com/screens/5de52a3a-4801-4f05-86d1-bf3df8d4c769) (filter chips above a plain list) ·
[PlanetScale](https://mobbin.com/screens/350460dd-4eba-4197-9f8e-f18d0e96cfef) (add-filter popover, three columns) ·
[7shifts](https://mobbin.com/screens/8b845427-b403-4eb7-87c9-aa771a00d378) (verb chip, date+time stack, Show Details) ·
[Square](https://mobbin.com/screens/4c14b94a-4abc-4e9b-aa97-de2006ca8498) (date group headers) ·
[Customer.io](https://mobbin.com/screens/d56c279e-cb13-4599-b12f-6e57ecb1ba64) (type / name / timestamp columns) ·
[HubSpot](https://mobbin.com/screens/96ceeb00-11c7-4544-a962-f38905fb52ed) (all-event-types filter chip) ·
[Fibery](https://mobbin.com/screens/94ad5a19-976a-4301-a5b9-c62e1c3f7b00) (dense event stream, entity chips)

**Launcher / switcher overlay**
[Frame](https://mobbin.com/screens/e3379b9e-68b0-48ee-82fe-fa2ae058c002) (search on top, rows with trailing keys) ·
[StackAI](https://mobbin.com/screens/bbcc94bb-f535-4dca-8532-56b135fce5c3) (grouped ⌘K, footer key hints) ·
[Adobe](https://mobbin.com/screens/875f19c6-0743-4a17-a34b-dd7dd9b9f68f) (recents + results + footer) ·
[Microsoft Loop](https://mobbin.com/screens/24f59200-51a6-4d71-a02d-6428f2ead199) (search + recent workspaces) ·
[Gamma](https://mobbin.com/screens/c6ae8918-d163-4b0e-9d35-c4c25674e9cb) (grid/list toggle, thumbnails) ·
[Discord](https://mobbin.com/screens/42c4d138-1cf5-4a0a-acc1-d67c03e0d956) (focused search, two result kinds) ·
[Evernote](https://mobbin.com/screens/46926538-2111-4bed-8396-4db69c067660) (modal search over the whole app) ·
[Replit](https://mobbin.com/screens/dd0c79a8-feb5-4819-a470-466d2a592f4a) (compact icon + name list)

**Conflict resolution / comparison**
[Salesforce](https://mobbin.com/screens/5dc154a3-7f3d-4869-8a5d-1398d85fb0ca) (two-column compare, radio per field, Back/Next) ·
[ManyChat](https://mobbin.com/screens/7c5ba065-40ce-48ca-80a2-bcfac03d550d) (side-by-side records, diff fields) ·
[Mistral AI](https://mobbin.com/screens/782f5257-54b5-44df-aac7-4ffd42aa1d3c) (old vs new pane, added lines tinted) ·
[Suno](https://mobbin.com/screens/06f86d45-a245-450f-ac75-a905f1862112) (two full options side by side, choose) ·
[Air](https://mobbin.com/screens/ac2d76ce-872c-4bfc-959f-38c97719f1c1) (radio option list + Confirm) ·
[Origin](https://mobbin.com/screens/2fd89d01-e3b2-4054-bee6-dbf2780f40cd) (selected option card, explanatory chips) ·
[Relevance AI](https://mobbin.com/screens/cf5634be-2e4c-4499-97c3-e761293bdd80) (two radios, one filled confirm) ·
[Maze](https://mobbin.com/screens/11300a56-b150-4583-b60e-13770fdc603c) (title + count, single primary action)

### 14.1 The conflict resolver, specifically

`ConflictResolver` is the one control whose UX the references answer completely
and which the current shell has no spec for.

Two conflicting rules must be shown **simultaneously and comparably** — that is
the entire task, and neither a sequential wizard (Air, Relevance AI) nor a diff
(Salesforce's field table) does it. Salesforce's Compare-leads and ManyChat's
merge are the pattern:

- A modal `.sheet` at `--elev-3`, max 720px.
- A one-line premise: *"`Ctrl+Alt+Delete` is already mapped by Windows Keyboard."*
- **Two columns, both fully rendered.** Never a collapsed summary.
- The conflicting chord is the visual anchor in both: two rows of `.kbd`.
- One choice marked per conflict, with a `.radio` — and the *losing* rule is not
  hidden, because "what did I give up" is the question the user actually has.
- The consequence of each choice printed under it at `--fs-meta` `--fg-secondary`:
  *"Windows Keyboard's `Ctrl+Alt+Delete` will no longer fire."*
- Footer: `Cancel` (ghost, left) · `Apply` (**primary**, right), with `⌘↵` shown
  as a `.kbd` — it is the keyboard way out, and the app's whole thesis is that
  the keyboard is faster.
- `Escape` closes and changes nothing. The choice commits only on `Apply`.
