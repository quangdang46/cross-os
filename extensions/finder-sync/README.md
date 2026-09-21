# CrossOS Finder Sync Platform Extension (bead cross-os-vbl.1)

Native FIFinderSync appex in this directory (Xcode project). Go-side IPC
client + menu model live alongside (`menu.go`, `ipc.go`, `visibility.go`,
`naming.go`) — same wire format (JSON over Unix socket), shared socket path
convention `~/Library/Application Support/CrossOS/crossos.sock`.

## Build (requires Xcode — NOT Command Line Tools only)

No `.xcodeproj` in tree yet: the Xcode wiring step (new target of type
Finder Sync Extension, add `FinderSync.swift` + `Info.plist` +
`CrossOSFinderSync.entitlements`, set Team ID so `$(TEAM_ID)` resolves,
embed the appex in the signed .app) lands when a machine with full Xcode
does it — hand-writing an xcodeproj without Xcode is error-prone. Until
then:

```bash
# Manual target setup on a full-Xcode machine:
# 1. New target → Finder Sync Extension, product CrossOSFinderSync.
# 2. Add FinderSync.swift, Info.plist, CrossOSFinderSync.entitlements.
# 3. Set DEVELOPMENT_TEAM so $(TEAM_ID) resolves to the Team-ID-prefixed
#    app group (containermanagerd rejects non-prefixed groups — newfile
#    v0.2.1 lesson).
# 4. xcodebuild -project <generated>.xcodeproj -scheme CrossOSFinderSync -configuration Release
```

`xcodebuild` here with only CLT installed fails with "requires Xcode" —
that is an environment gap, not a source gap (CI uses macos-latest with
full Xcode; see .github/workflows/ci.yml). Syntax is verified meanwhile
via `swiftc -parse FinderSync.swift` (EXIT 0).

## Files

| File | Source |
|---|---|
| `FinderSync.swift` | ADAPT newfile `Extension/FinderSync.swift` (menu + snapshot + target-dir; IPC replaces DistributedNotificationCenter) |
| `Info.plist` | NSExtension com.apple.FinderSync principal class |
| `CrossOSFinderSync.entitlements` | sandbox + user-selected rw + temp-exception (newfile shape) + Team-ID-prefixed app group |
| `naming.go` | ADAPT newfile `Shared/FilenameGenerator.swift` + `FileTypeEntry.swift` + `SeedPresets.swift` |

No newfile host-app UI in tree (SwiftUI prefs / template editor) — settings
are declarative (§3.6c). Attribution: `third_party/newfile/`.
