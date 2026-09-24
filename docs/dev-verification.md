# Verifying the tree off macOS (cross-os-j7p)

The daemon's shipping target is macOS. Most of the tree is not, and this note
records what a Windows or Linux dev box can actually typecheck today, why, and
what is still gated.

Run it with:

```sh
./scripts/dev-verify.sh          # everything
./scripts/dev-verify.sh core     # core module only
./scripts/dev-verify.sh app      # app (shell) module only
```

It needs only `go`, `node` and `npm` — no `wails3`, no `task`. It is not
`scripts/run.sh` (cross-os-jn1), which builds the real product and does need
`wails3` on a macOS host.

## What was actually wrong

`core/internal/adapter` was already correctly partitioned. The darwin-only
files each carried `//go:build darwin`:

| file | constraint | supplies |
| --- | --- | --- |
| `ax_cgo.go` | `darwin` | AX bridge cgo surface |
| `tap_cgo.go` | `darwin` | `setDecideExport`, `crossosGoDecide`, `cgFlag*` |
| `keycode_darwin.go` | `darwin` | `ToWinKeycode` |
| `tap_darwin.go` | `darwin` | `darwinTap` / `darwinQuery` / `darwinProbe` |
| `ax_bridge_test.go` | `darwin` | AX tests |
| `dll_windows.go`, `dll_windows_test.go`, `tap_windows.go` | `windows` | Windows hook seam |
| `stub.go` | `!windows && !darwin` | unsupported-platform stub |

The break was in three files that reached across that boundary with no
constraint of their own:

- **`driver.go`** — the only unconstrained *non-test* offender, and the single
  root cause. It held the portable `Driver` / `FromDecision` / `KeyAction`
  surface *and* four helpers that call `setDecideExport`, `crossosGoDecide`,
  `cgFlag*` and `ToWinKeycode`.
- **`tap_bridge_test.go`**, **`tap_dispatch_test.go`** — tests calling
  `crossosGoDecide` / `setDecideExport`.

Because `driver.go` is unconstrained it compiled on every GOOS and referenced
symbols that exist on exactly one, so `go build`, `go vet` and `go test` all
failed for the whole module off macOS. A codebase whose safety argument is
"the compiler agrees with us" had one platform where the compiler was silent.

**`core/pkg/winlayout` needed no change.** Its non-test build was always
portable; it only failed because `dispatch_test.go` imports `adapter`, so the
adapter failure dragged winlayout's test build down with it. Zero files in that
package were touched.

## The fix, and why macOS is unaffected

`driver.go` could not simply get `//go:build darwin`. That would drop `Driver`
itself on Windows, where `tap_windows.go`, `stub.go`, `cmd/crossos/main.go` and
winlayout's dispatch test all use it — trading a compile error for a worse one.
Constraints are per-file, so the darwin-only half had to move.

The four helpers moved **verbatim** into `tap_darwin.go`, which already carried
`//go:build darwin`. That is deliberately the choice that adds no file: no file
was created or removed, and every touched file's constraint is darwin-equivalent
on darwin, so `GOOS=darwin` selects an identical file set and the darwin package
declares an identical symbol set. The two test files gained
`//go:build darwin` (they were already darwin-only in practice — there is no
off-darwin `crossosGoDecide` to assert against).

Re-derive the pinned set at any time:

```sh
cd core
GOOS=darwin go list -f 'GoFiles={{.GoFiles}}
TestGoFiles={{.TestGoFiles}}' ./internal/adapter ./pkg/winlayout
```

As of this note, `./internal/adapter` selects:

```
GoFiles:      adapter.go doc.go driver.go errors.go keycode_darwin.go seam_errors.go tap_darwin.go
TestGoFiles:  adapter_test.go ax_bridge_test.go tap_bridge_test.go tap_dispatch_test.go
```

`GOOS=darwin go vet ./internal/adapter/` cannot fully typecheck off macOS:
cgo is unavailable, so `tap_cgo.go` drops out and the pre-existing
`TapInstall` reference in `tap_darwin.go` is undefined. That is an environment
limit, not a regression — the real darwin check is `go test ./...` on a mac.

## The app module and `//go:embed all:frontend/dist`

`app/main.go` embeds `all:frontend/dist`, so from a clean checkout
`go build ./...` in `app/` dies before it typechecks anything:

```
main.go:29:12: pattern all:frontend/dist: no matching files found
```

`dist/` is a build artifact and is gitignored, as the policy requires. The
catch is that the real bundle is *not* reachable without `wails3`:
`vite.config.ts` registers `wails("./bindings")` unconditionally, and the
`@wailsio/runtime` plugin hard-requires
`bindings/github.com/wailsapp/wails/v3/internal/eventcreate`, which only
`wails3 generate bindings ./...` writes. Measured on a machine with the CLI
absent:

```
$ npx vite build --mode production
✗ Build failed
[plugin wails-typed-events]
Event bindings module not found at import specifier
  './bindings/github.com/wailsapp/wails/v3/internal/eventcreate'
```

So `scripts/dev-verify.sh` does the honest thing: if `wails3` is on `PATH` it
generates the bindings and builds the real bundle; otherwise it writes a
**placeholder** `dist/index.html` whose body states that no bundle exists, so
the Go toolchain can typecheck `main.go` and `backend/`. The placeholder
honours the "never a blank page" rule — it renders a diagnostic, not white
space — and `dist/` being gitignored keeps it out of every commit.

What that buys, on a box with no `wails3`:

```sh
cd app/frontend && npm ci     # 27 packages
cd ../..                     # repo root
go -C app test ./backend/...  # PASS — needs no dist at all
go -C app build ./...         # PASS — needs the placeholder
```

## `app/frontend/bindings/` is generated output, and it is not a `.d.ts`

`app/frontend/bindings/` is gitignored, and `wails3 generate bindings ./...`
**wipes the directory** before writing it. What it writes is JavaScript:

```
bindings/crossos/app/backend/{index,models,service}.js
bindings/github.com/wailsapp/wails/v3/internal/eventcreate.js
```

A hand-written `index.d.ts`/`models.d.ts` placed there does not survive the
next generation, so it is a local convenience at best. Measured with the real
generator, it was also *wrong*: Wails builds each model property from the Go
**json tag** when the field has one and from the Go field name when it does not
(`internal/generator/collect/struct.go` reads `reflect.StructTag.Get("json")`
— the same rule `encoding/json` applies, because that is what marshals the
result). So the eight tagged rows come out snake_case:

| Go struct | generated property |
| --- | --- |
| `MatrixRow` | `rule_id`, `plugin`, `action`, `keys`, `contexts`, `enabled` |
| `ZoneRow` | `id`, `name`, `x`, `y`, `w`, `h` |
| `TrialState` | `plugin`, `state`, `remaining_ms`, `timeout_ms` |
| `Status` (no tags) | `Running`, `SafeMode`, … |

`Status`, `PluginState` and `Page` carry no json tags, so those three stay
PascalCase. The two spellings coexisting is not a quirk to remember — it is
`encoding/json`'s rule, and `app/backend`'s `TestGeneratedModelsMatchWireTags`
asserts it against the generated `models.js` so a tag added or renamed on either
side of the module boundary is a test failure rather than an `undefined` in a
settings row.

`tsconfig.json` sets `allowJs` for the same reason. Without it the generated
`.js` modules resolve to `any`, the `ServiceApi` annotation in
`src/lib/service.ts` silently stops checking anything, and a green
`tsc --noEmit` says nothing about the bindings at all.

## The daemon lock, and the darwin-only tap tests

`core/cmd/crossos` carried the same defect one level up from `adapter` — and
it was all this note's gap list had left, two entries in all.
`listenSocket` called `syscall.Flock` from an unconstrained file, and
`main_test.go` called the darwin-only tap seam (`BindDecideForTest`,
`DecideForMacTest`, `ToWinKeycode`) from an unconstrained file. Both are now
split per GOOS:

| file | constraint | supplies |
| --- | --- | --- |
| `lock_unix.go` | `unix && !solaris && !aix` | `syscall.Flock(LOCK_EX\|LOCK_NB)` |
| `lock_windows.go` | `windows` | `LockFileEx`, exclusive + fail-immediately |
| `lock_other.go` | `!windows && (!unix \|\| solaris \|\| aix)` | a typed refusal |
| `main_darwin_test.go` | `darwin` | the three CGEvent-tap tests |
| `main_test.go` | (none) | the 26 portable tests, on every GOOS |

Those lock constraints are measured, not guessed. `syscall.Flock` exists on
darwin, linux, the BSDs, dragonfly, illumos and ios, and on no other GOOS —
solaris and aix satisfy `unix` but do not carry it, so they are excluded by
name and fall through to `lock_other.go`:

```sh
mkdir -p /tmp/flockprobe && cd /tmp/flockprobe
printf 'package p\nimport "syscall"\nvar _ = syscall.Flock\n' > p.go
go mod init p >/dev/null
for spec in darwin/arm64 linux/amd64 windows/amd64 freebsd/amd64 \
            netbsd/amd64 openbsd/amd64 dragonfly/amd64 illumos/amd64 \
            solaris/amd64 aix/ppc64; do
  printf '%-18s ' "$spec"
  GOOS="${spec%%/*}" GOARCH="${spec##*/}" go build ./... >/dev/null 2>&1 \
    && echo 'HAS Flock' || echo 'NO Flock'
done
```

**The Windows half is a real lock.** Windows has no `flock(2)`; the equivalent
is `LockFileEx`, an exclusive byte-range lock, resolved through
`syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")` because Go's stdlib
`syscall` package does not export it — only `x/sys/windows` does — which keeps
the daemon on the stdlib-only dependency set its other Windows seam
(`adapter/dll_windows.go`) already uses. `ERROR_LOCK_VIOLATION` is not exported
either, so `lock_windows.go` carries the value under its Win32 name.

A `return nil` there would be the worst available outcome and the one this
repo's rules name: the lock would fail **open**, a second daemon would read the
lock file as free, unlink the live daemon's socket and serve beside it — the
exact socket theft `cross-os-jn1` exists to prevent. So the property is pinned
by a test that runs on whichever GOOS runs it, and the platforms with no
advisory locking refuse to start rather than serve unlocked:

```
$ go test ./cmd/crossos/ -run TestLockExclusiveRefusesSecondHolder -v
--- PASS: TestLockExclusiveRefusesSecondHolder (0.00s)
```

Replacing `lockExclusive` with a no-op turns it red, which is the whole point:

```
--- FAIL: TestLockExclusiveRefusesSecondHolder (0.00s)
    main_test.go:823: second holder: err=<nil>, want errLockHeld — a lock that
    fails open lets a second daemon steal the socket
```

The three moved tests keep running on macOS. They are the proof that a chord
as CGEvent reports it reaches the same rule the router reaches with the
internal Windows virtual keycode — a property no off-darwin stub could assert,
which is why the file is constrained rather than emptied. The package goes
from 54 to 55 passing tests on darwin (the new lock test is the difference)
and `main_test.go` holds 26 of them, portable.

Re-derive the per-GOOS sets at any time:

```sh
cd core
GOOS=darwin  go list -f 'GoFiles={{.GoFiles}}
TestGoFiles={{.TestGoFiles}}' ./cmd/crossos
GOOS=windows go list -f 'GoFiles={{.GoFiles}}
TestGoFiles={{.TestGoFiles}}' ./cmd/crossos
```

| GOOS | `GoFiles` | `TestGoFiles` |
| --- | --- | --- |
| darwin | `autostart_darwin.go lock_unix.go main.go pagedata.go tap_darwin.go userules.go` | `autostart_darwin_test.go main_darwin_test.go main_test.go pagedata_test.go userules_test.go` |
| windows | `autostart_other.go lock_windows.go main.go pagedata.go tap_other.go userules.go` | `main_test.go pagedata_test.go userules_test.go` |

`GOOS=windows go build ./...` and `GOOS=windows go vet ./...` both pass. So
does CI's `build` job, which compiles the daemon for `windows/amd64`: that row
was red while `main.go` called `syscall.Flock` unconditionally, and is green
now.

## Known gaps

The two `core/cmd/crossos` gaps this note used to carry are closed — see
*The daemon lock, and the darwin-only tap tests* above — and their patterns
are deleted from `GAP_PATTERNS` in `scripts/dev-verify.sh`, so a regression in
either file is now a real failure. That leaves:

- `app/frontend/bindings/` is gitignored, so a clean clone cannot run
  `npx tsc --noEmit` or `npm run build` until `wails3 generate bindings ./...`
  has run in `app/`. That is the correct policy (they are build outputs), and
  the cost is that the frontend typecheck needs the CLI while every Go check
  does not. `scripts/dev-verify.sh` reports it as a gap rather than a pass.
- Go stops reporting after 10 errors per package, so `core/cmd/crossos` can
  hide errors past the cap. `scripts/dev-verify.sh` therefore classifies every
  error *line* against a known-gap list and fails on anything unrecognised —
  an error riding along inside an already-failing command is reported, never
  absorbed. With the list empty that means a failing command in this tree is
  a failure whatever it printed, including nothing at all.
