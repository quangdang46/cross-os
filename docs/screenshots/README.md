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