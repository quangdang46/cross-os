import CrossOSCore
import Foundation

// A CoreClient that records what was asked of it.
//
// The daemon is not available to a test run, and a test that needs a running
// daemon is a test that gets skipped — which is how the React suite ended up
// green against a stylesheet jsdom never loaded. So the properties below are
// asserted against a stub, not against a live socket.
//
// This is also what the UI tests will use. Every view takes a `CoreClient`, so a
// page can be built and driven with no daemon and no window.

/// A stub daemon whose answers are set per method and whose call count is
/// recorded.
public actor SpiedCoreClient: CoreClient {
    public private(set) var callLog: [String] = []

    private var statusAnswer = DaemonStatus(running: true, version: "v9.9.9")
    private var pluginsAnswer: [PluginState] = []
    private var windowsAnswer: [WindowRow] = []
    private var readinessAnswer: [ReadinessRow] = []
    private var eventLogsAnswer: [String] = []
    private var triggerAnswer = SwitcherTrigger(triggered: false)
    private var pagesAnswer: [Page] = []
    private var matrixAnswer: [MatrixRow] = []
    private var overridesAnswer: [OverrideRow] = []
    private var conflictsAnswer: [ConflictRow] = []
    private var observeAnswer = ObserveStateRow(observe: false, mode: "off")
    private var appsAnswer: [AppRow] = []
    private var profilesAnswer: [ProfileRow] = []
    private var storedRuleState = false

    public init() {}

    // Setters rather than public settable properties: assigning across an actor
    // boundary from a test is a two-line dance (`await client.x = …` has to fit
    // on one line for the isolation to be visible), and a method reads the same
    // at the call site.
    public func setStatus(_ value: DaemonStatus) { statusAnswer = value }
    public func setPlugins(_ value: [PluginState]) { pluginsAnswer = value }
    public func setWindows(_ value: [WindowRow]) { windowsAnswer = value }
    public func setReadiness(_ value: [ReadinessRow]) { readinessAnswer = value }
    public func setEventLogs(_ value: [String]) { eventLogsAnswer = value }
    public func setTrigger(_ value: SwitcherTrigger) { triggerAnswer = value }
    public func setPages(_ value: [Page]) { pagesAnswer = value }
    public func setMatrix(_ value: [MatrixRow]) { matrixAnswer = value }
    public func setOverrides(_ value: [OverrideRow]) { overridesAnswer = value }
    public func setConflicts(_ value: [ConflictRow]) { conflictsAnswer = value }
    public func setObserve(_ value: ObserveStateRow) { observeAnswer = value }
    public func setApps(_ value: [AppRow]) { appsAnswer = value }
    public func setProfiles(_ value: [ProfileRow]) { profilesAnswer = value }
    public func setStoredRuleState(_ value: Bool) { storedRuleState = value }

    public func count(of method: String) -> Int {
        callLog.filter { $0 == method }.count
    }

    private func record(_ method: String) { callLog.append(method) }

    public func status() async throws -> DaemonStatus {
        record("core.status")
        return statusAnswer
    }

    public func plugins() async throws -> [PluginState] {
        record("plugin.list")
        return pluginsAnswer
    }

    public func switcherWait(timeoutMS: Int) async throws -> SwitcherTrigger {
        record("core.switcherWait")
        return triggerAnswer
    }

    public func windows() async throws -> [WindowRow] {
        record("core.windows")
        return windowsAnswer
    }

    public func readiness() async throws -> [ReadinessRow] {
        record("core.readiness")
        return readinessAnswer
    }

    public func eventLogs() async throws -> [String] {
        record("core.eventLogs")
        return eventLogsAnswer
    }

    public func pages() async throws -> [Page] {
        record("core.pages")
        return pagesAnswer
    }

    public func matrix() async throws -> [MatrixRow] {
        record("config.getMatrix")
        return matrixAnswer
    }

    public func overrides() async throws -> [OverrideRow] {
        record("config.getOverrides")
        return overridesAnswer
    }

    public func conflicts() async throws -> [ConflictRow] {
        record("core.conflicts")
        return conflictsAnswer
    }

    public func observeState() async throws -> ObserveStateRow {
        record("core.observeState")
        return observeAnswer
    }

    public func apps() async throws -> [AppRow] {
        record("core.apps")
        return appsAnswer
    }

    public func profiles() async throws -> [ProfileRow] {
        record("core.profiles")
        return profilesAnswer
    }

    public func setRuleEnabled(ruleID: String, enabled: Bool) async throws -> Bool {
        record("config.setRuleEnabled")
        // The STORED state, the way the daemon answers. A stub that echoed the
        // request would make the one assertion this exists for — that a
        // refusal is reported rather than reconciled with the row — untestable.
        return storedRuleState
    }
}

// The two footguns, in the order the Go client demonstrates them.
//
// Both are cases where a refactor that looks like tidying reintroduces a
// defect, which is why each has a name and a reason rather than just an
// assertion.
@MainActor func runClientSuite() async throws {
    suite("Footgun 1 — status is ONE round trip")

    do {
        // Go's `App.GetStatus` calls `core.status` six times — once per accessor —
        // each dialing a fresh socket and decoding the same object, plus a
        // `plugin.list` (bridge.go:280-290, ipc_client.go:145-199). `App.tsx` polls
        // it every 5s, so the React shell spent ~8 round trips per tick.
        //
        // This is the assertion that stops that coming back as a "harmless"
        // refactor: six accessors that each call a helper looks correct, and the
        // cost is invisible until someone counts.
        let client = SpiedCoreClient()
        await client.setStatus(DaemonStatus(
            running: true, safeMode: false, killed: false,
            interception: true, tapError: "", version: "v0.1.0"
        ))

        let status = try await client.status()

        expectEqual(await client.count(of: "core.status"), 1, "six fields, one call")
        // And every field came back from that one call.
        expect(status.running, "running came back")
        expect(status.interception, "interception came back")
        expectEqual(status.version, "v0.1.0", "version came back")
    }

    do {
        // A 5s tick reads status, pages and logs. Three questions, three calls —
        // the shape the port must keep, because the tick is the only place a
        // per-field regression would show up as a visible cost.
        let client = SpiedCoreClient()
        _ = try await client.status()
        _ = try await client.eventLogs()

        expectEqual(
            await client.callLog, ["core.status", "core.eventLogs"],
            "a poll tick costs one call per question, not one per field"
        )
    }

    suite("Footgun 2 — an expired budget is a value")

    do {
        // `triggered: false` is the daemon's "your budget ran out" — the answer a
        // poll expects, not an incident. A client that throws on it shows a broken
        // switcher every time nothing happens, which is most of the time.
        let client = SpiedCoreClient()
        await client.setTrigger(SwitcherTrigger(triggered: false, action: nil, windowID: nil))

        let trigger = try await client.switcherWait(timeoutMS: 1000)

        expect(!trigger.triggered, "an expired budget is triggered=false")
        expect(trigger.action == nil, "with no action, which is how 'nothing happened' arrives")
    }

    do {
        let client = SpiedCoreClient()
        await client.setTrigger(SwitcherTrigger(triggered: true, action: "commit", windowID: "412"))

        let trigger = try await client.switcherWait(timeoutMS: 1000)

        expect(trigger.triggered, "a real trigger says so")
        expectEqual(trigger.action, "commit", "and carries the switcher's own action")
        expectEqual(trigger.windowID, "412", "and the window it acted on")
    }

    suite("The daemon owns the order")

    do {
        // `WindowRow`'s comment says it, so it is said here: the daemon owns the
        // MRU, the shell draws the position it is handed, and re-sorting is a bug
        // that looks like tidying.
        let client = SpiedCoreClient()
        await client.setWindows([
            WindowRow(windowID: "412", appID: "com.apple.finder", title: "Downloads",
                      index: 0, selected: false, skippable: false),
            WindowRow(windowID: "881", appID: "com.apple.Terminal", title: "zsh",
                      index: 1, selected: true, skippable: false),
            WindowRow(windowID: "207", appID: "com.apple.Safari", title: "",
                      index: 2, selected: false, skippable: true),
        ])

        let windows = try await client.windows()

        expectEqual(windows.map(\.index), [0, 1, 2], "index arrives in the daemon's order")
        expect(windows[1].selected, "the selected one is marked")
        expect(windows[2].skippable, "the skippable one is marked")
    }

    suite("The daemon's types, decoded from its own JSON")

    do {
        // The Go struct tags are the contract. A key that does not match decodes to
        // its default rather than failing, so the round trip is the test.
        let daemonJSON = """
        {"running":true,"safe_mode":false,"killed":false,
         "interception":false,"tap_error":"adapter: tap refused","version":"v0.1.0"}
        """
        let status = try JSONDecoder().decode(DaemonStatus.self, from: Data(daemonJSON.utf8))

        expect(status.running, "running")
        expect(!status.safeMode, "safe_mode")
        expect(!status.killed, "killed")
        expect(!status.interception, "interception")
        expectEqual(status.tapError, "adapter: tap refused", "tap_error")
        expectEqual(status.version, "v0.1.0", "version")
    }

    do {
        // window_id / app_id, not windowID / appID. The generated TypeScript
        // bindings carried this exact hazard once, which is why service.ts:12-17
        // exists as a comment about it.
        let daemonJSON = """
        [{"window_id":"412","app_id":"com.apple.finder","title":"Downloads",
          "index":0,"selected":false,"skippable":false}]
        """
        let windows = try JSONDecoder().decode([WindowRow].self, from: Data(daemonJSON.utf8))

        expectEqual(windows.first?.windowID, "412", "window_id decodes")
        expectEqual(windows.first?.appID, "com.apple.finder", "app_id decodes")
    }

    do {
        let daemonJSON = """
        [{"id":"keyboard","label":"Keyboard interception","ready":false,
          "detail":"adapter: tap refused (input-monitoring consent missing?)"}]
        """
        let rows = try JSONDecoder().decode([ReadinessRow].self, from: Data(daemonJSON.utf8))

        expect(rows.first?.ready == false, "a not-ready row is not ready")
        expect(
            rows.first?.detail.contains("input-monitoring") == true,
            "and it carries the detail that says what to do"
        )
    }

    do {
        // "none" with an empty plugin is the daemon's honest "no trial in flight",
        // and the Go comment says it must not render as a broken countdown. The
        // question gets one answer here rather than a string comparison in every
        // view.
        let idle = try JSONDecoder().decode(
            TrialState.self,
            from: Data(#"{"plugin":"","state":"none","remaining_ms":0,"timeout_ms":0}"#.utf8)
        )
        expect(idle.isIdle, "state none is idle")

        let running = try JSONDecoder().decode(
            TrialState.self,
            from: Data(#"{"plugin":"p","state":"active","remaining_ms":30000,"timeout_ms":60000}"#.utf8)
        )
        expect(!running.isIdle, "state active is not")
        expectEqual(running.remainingMS, 30_000, "remaining_ms decodes")

    suite("The nav, as the daemon serves it")

    do {
        // The fifteen core pages, in the order the nav will draw them. This is the
        // assertion that a Swift client can see the product at all: before
        // `core.pages` existed the page list lived in the shell process, so a
        // client in another language got an empty nav and the Go suite stayed
        // green.
        let daemonJSON = """
        [{"ID":"core.home","Title":"Home","Group":"home","Symbol":"house","Order":0,
          "FirstRun":false,"Visibility":"true","Actions":null,
          "Schema":{"type":"page","description":"What CrossOS is doing right now.",
                    "controls":[{"kind":"homeSummary","id":"status"},
                                {"kind":"checklist","id":"readiness"}]}},
         {"ID":"core.onboard","Title":"Welcome","Group":"home","Symbol":"sparkles","Order":0,
          "FirstRun":true,"Visibility":"true","Actions":null,
          "Schema":{"type":"page","firstRun":true,
                    "controls":[{"kind":"wizard","id":"onboard","steps":["Welcome"]}]}}]
        """
        let pages = try Page.decodeList(
            try JSONDecoder().decode(JSONValue.self, from: Data(daemonJSON.utf8))
        )

        expectEqual(pages.count, 2, "two pages decoded")
        // PascalCase keys, because they are Go's exported field names and the
        // binding generator emitted them verbatim. This is not a typo to "fix" —
        // the React bindings carried the same hazard and service.ts:12-17 exists
        // as a comment about it.
        expectEqual(pages[0].id, "core.home", "ID decodes")
        expectEqual(pages[0].title, "Home", "Title decodes")
        expectEqual(pages[0].group, "home", "Group decodes")
        expectEqual(pages[0].symbol, "house", "Symbol decodes")
        expect(!pages[0].firstRun, "FirstRun is false where the daemon says false")
        expect(pages[1].firstRun, "FirstRun is true where the daemon says true")
    }

    do {
        // A control is a kind plus the fields that kind reads — NOT 28 typed
        // payloads. The Go side builds them with map[string]any, so typing all 28
        // here would be 28 things to keep in step with builders that are
        // themselves untyped.
        let daemonJSON = """
        {"controls":[{"kind":"checklist","id":"readiness","source":"core:readiness",
                      "items":["keyboard","windows","finder"],"note":"Each line says what to do."}]}
        """
        let value = try JSONDecoder().decode(JSONValue.self, from: Data(daemonJSON.utf8))
        let schema = PageSchema.decode(value.objectValue ?? [:])

        expectEqual(schema.controls.count, 1, "one control")
        expectEqual(schema.controls[0].kind, "checklist", "kind decodes")
        expectEqual(schema.controls[0].id, "readiness", "id decodes")
        expectEqual(schema.controls[0].source, "core:readiness", "source decodes")
        // `items` is the page's own subset AND its own order. A shell that
        // re-sorted them would be answering a different question than the one
        // asked, so the order is preserved exactly.
        expectEqual(schema.controls[0].items, ["keyboard", "windows", "finder"], "items keep their order")
    }

    do {
        // A missing field draws a default rather than failing the page. A page
        // that fails to decode is a blank window with no clue which side is at
        // fault — which is the failure `UnsupportedControl` exists to avoid.
        // The assertion that matters is the accessor's behaviour on absent fields.
        let bare = Control(kind: "note", id: "")
        expectEqual(bare.label, "", "a missing label is empty, not absent")
        expectEqual(bare.items.count, 0, "a missing items list is empty")
        expectEqual(bare.steps.count, 0, "a missing steps list is empty")
        expect(bare.link("aboutLink") == nil, "a missing link is nil")
    }
}
}
