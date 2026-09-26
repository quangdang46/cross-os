import Foundation
import Network

/// The daemon, as the UI needs it.
///
/// A protocol rather than a concrete client so every view can be tested
/// without a socket — the same job `app/frontend/src/test/fixtures.ts` (1,124
/// lines) does today, and the reason the React suite could run at all. A client
/// that only the app can construct is a client nothing tests.
public protocol CoreClient: Sendable {
    /// One `core.status`. **Once**, not once per field — see `DaemonStatus`.
    func status() async throws -> DaemonStatus

    /// One `plugin.list`.
    func plugins() async throws -> [PluginState]

    /// The switcher long-poll. Returns a `triggered: false` trigger when the
    /// budget expires; that is a value, not an error.
    func switcherWait(timeoutMS: Int) async throws -> SwitcherTrigger

    /// The switcher's tile list (`core.windows`). The daemon owns the MRU, so
    /// the order of this array IS the order to draw.
    func windows() async throws -> [WindowRow]

    /// The Home pane's checklist (`core.readiness`).
    func readiness() async throws -> [ReadinessRow]

    /// The daemon's recent log lines (`core.eventLogs`).
    func eventLogs() async throws -> [String]

    /// Apply a profile (`core.profileApply`).
    ///
    /// The way BACK out is `core.profileDeactivate`, and both live on this
    /// protocol for the same reason: a page that can only turn a profile on is
    /// a half-feature. The store records what the profile overwrote, and
    /// without a way to put it back the person who applied one has neither a
    /// door nor a way to find out that a door should exist.
    func applyProfile(id: String) async throws -> JSONValue
    func deactivateProfile(id: String) async throws -> JSONValue

    /// Switch a plugin on or off (`plugin.setEnabled`).
    ///
    /// It scans `plugin.list` first to fail closed on an unknown id, so a typo
    /// is a refusal rather than a silent no-op (bridge.go:294-300).
    func setPluginEnabled(id: String, enabled: Bool) async throws

    /// Set one file type (`core.setFileType`).
    ///
    /// The answer is the WHOLE catalog, not the row. A partial answer would
    /// leave the table showing a row the daemon has already replaced, and the
    /// next poll would have no way to tell which of the two is true.
    func setFileType(entry: FileTypeRow, enabled: Bool) async throws -> [FileTypeRow]

    /// The shortcut table (`config.getShortcuts`). Untyped on the Go side
    /// (`[]map[string]any`), so it comes back as `JSONValue` and the view
    /// narrows it — the seven untyped methods the React side handles with
    /// `wire.ts` helpers, and the reason `JSONValue` exists.
    func shortcuts() async throws -> [JSONValue]

    /// Replace the shortcut table (`config.setShortcuts`). Answers the count
    /// it accepted, which is not the count it was given: a table with a
    /// duplicate chord accepts fewer rows than it was sent, and a caller that
    /// assumed they matched would report success for a partial write.
    func setShortcuts(_ rows: [JSONValue]) async throws -> Int

    /// Zones (`config.getZones`) and the editor that places them.
    func zones() async throws -> [ZoneRow]
    func setZones(_ zones: [ZoneRow]) async throws -> Int

    /// The user-rule table (`config.getUserRules`).
    func userRules() async throws -> [UserRuleRow]

    /// Installed apps for the rule builder's app picker (`core.apps`).
    func appsForRules() async throws -> [AppRow]

    /// Command-palette entries (`core.commands`). Commands arrive from
    /// plugins, so a new command must show up with no shell change.
    func commands() async throws -> [CommandRow]

    /// Plugin manifest facts (`core.pluginMeta`).
    func pluginMeta() async throws -> [PluginMetaRow]

    /// The trace timeline (`core.traces`) and the erase behind its clear
    /// button.
    func traces() async throws -> [TraceRow]
    func clearTraces() async throws -> [TraceRow]

    /// The file-type catalog (`core.fileTypes`).
    func fileTypes() async throws -> [FileTypeRow]

    /// The trial in flight (`safety.trialState`) and its three transitions.
    func trialState() async throws -> TrialState
    func beginTrial(plugin: String) async throws -> String
    func confirmTrial(plugin: String, healthy: Bool) async throws -> String
    func rollbackTrial(plugin: String) async throws -> String

    /// Panic stop and the way back (`safety.panicStop`, `safety.resume`).
    func panicStop() async throws -> JSONValue
    func resume() async throws -> JSONValue

    /// The behaviour matrix (`config.getMatrix`).
    func matrix() async throws -> [MatrixRow]

    /// The per-app overrides (`config.getOverrides`).
    func overrides() async throws -> [OverrideRow]

    /// Which rule wins each contested chord (`core.conflicts`).
    func conflicts() async throws -> [ConflictRow]

    /// What the recorder is doing (`core.observeState`).
    func observeState() async throws -> ObserveStateRow

    /// Installed apps (`core.apps`). An empty list is the answer on a platform
    /// with no enumeration, not a failure.
    func apps() async throws -> [AppRow]

    /// Profiles (`core.profiles`), in the daemon's declared order.
    func profiles() async throws -> [ProfileRow]

    /// Switch a rule on or off (`config.setRuleEnabled`).
    ///
    /// Returns the STORED state, not a success flag. The Go client decodes the
    /// answer's `enabled` straight into a bool and the React control reads it
    /// as "the new state" — reading it as "did it stick?" printed "The daemon
    /// kept Ctrl+C off." the moment somebody turned a rule OFF, on the one
    /// control whose whole job is making the machine stop remapping that
    /// chord (MatrixControl.tsx:34-41).
    func setRuleEnabled(ruleID: String, enabled: Bool) async throws -> Bool

    /// The settings pages the daemon is serving, in the order the nav will
    /// draw them.
    ///
    /// This used to be a shell-local memoized seed with no RPC behind it
    /// (`app/backend/service.go:32-35`), which meant a client in another
    /// language could not see a single page — the fifteen core ones lived in
    /// Go constants in `app/backend/pages.go` and were never sent anywhere.
    /// They now live beside the plugin API (`core/pkg/pluginapi/pages.go`) and
    /// the daemon serves them as `core.pages`.
    func pages() async throws -> [Page]

    // The rest of the surface, as the pages need it. Added with the pages that
    // call them — a method nothing calls is a method nothing tests, which is
    // the exact defect `TestEveryRegisteredKindIsReachable` exists to catch on
    // the renderer side.
}

// MARK: - Errors

public enum CoreError: Error, CustomStringConvertible {
    /// The daemon answered with a JSON-RPC error.
    case rpc(code: Int, message: String, method: String)
    /// The socket failed: not running, wrong path, permission denied.
    case transport(underlying: any Error, method: String)
    /// The daemon's answer did not match the declared shape. A renamed or
    /// retyped field lands here rather than as a silently empty string.
    case decode(method: String, underlying: any Error)
    /// The call outlived its budget.
    case timeout(method: String)

    public var description: String {
        switch self {
        case .rpc(let code, let message, let method):
            return "\(method): rpc code \(code): \(message)"
        case .transport(let underlying, let method):
            return "\(method): \(underlying)"
        case .decode(let method, let underlying):
            return "\(method): could not read the answer — \(underlying)"
        case .timeout(let method):
            return "\(method): timed out"
        }
    }
}

// MARK: - The live client

/// A `CoreClient` over the daemon's Unix domain socket.
///
/// One connection, kept open. The Go client dials per call
/// (ipc_client.go:71-75) because it is a bridge that never expected
/// concurrency; this one is an actor, so it can hold a connection and serialise
/// calls, which is what makes a 5s poll cost one socket setup rather than
/// eight.
public actor LiveCoreClient: CoreClient {
    /// The daemon's socket. `main.go:45` and
    /// `~/Library/Application Support/CrossOS/crossos.sock`.
    public static let defaultSocketPath =
        FileManager.default
        .homeDirectoryForCurrentUser
        .appendingPathComponent("Library/Application Support/CrossOS/crossos.sock")
            .path

    private let path: String
    private var connection: NWConnection?
    private var inbound = Data()
    private var nextID = 0
    private var pending: [Int: CheckedContinuation<WireResponse, any Error>] = [:]

    public init(socketPath: String = LiveCoreClient.defaultSocketPath) {
        self.path = socketPath
    }

    // MARK: Connection

    private func connect() async throws -> NWConnection {
        if let connection { return connection }

        let endpoint = NWEndpoint.unix(path: path)
        let parameters = NWParameters.tcp
        // A Unix socket is a stream with no meaningful keepalive story, and a
        // half-open socket would surface as a poll that never answers — which
        // looks exactly like a busy daemon. The timeout is what turns "hung"
        // into a diagnosable error.
        parameters.allowLocalEndpointReuse = true

        let connection = NWConnection(to: endpoint, using: parameters)
        self.connection = connection
        // Every state callback hops onto the actor. `stateUpdateHandler` runs on
        // a Network queue, so calling an isolated method directly is a
        // concurrency error rather than a race — this is the seam, not a
        // convenience.
        let ready = ReadyGate()
        connection.stateUpdateHandler = { [weak self] state in
            switch state {
            case .ready:
                ready.open()
                Task { await self?.beginReading() }
            case .failed(let error):
                Task { await self?.tearDown(error: CoreError.transport(
                    underlying: error, method: "connect"
                )) }
            case .cancelled:
                Task { await self?.tearDown(error: CoreError.transport(
                    underlying: NWError.posix(.ECANCELED), method: "connect"
                )) }
            default:
                break
            }
        }
        connection.start(queue: .global(qos: .userInitiated))

        // NWConnection has no `waitForReady`. Waiting for `.ready` is done with
        // a continuation the state handler resumes, and it needs a deadline: a
        // daemon that is not running accepts nothing and fails nothing, so an
        // unbounded wait here is a window that never returns rather than an
        // error.
        do {
            try await withThrowingTaskGroup(of: Void.self) { group in
                group.addTask { try await ready.wait() }
                group.addTask {
                    try await Task.sleep(for: .seconds(5))
                    throw CoreError.transport(
                        underlying: NSError(
                            domain: "CrossOS", code: 4,
                            userInfo: [NSLocalizedDescriptionKey:
                                "no answer from \(self.path) — is the daemon running? (./scripts/run.sh)"]
                        ),
                        method: "connect"
                    )
                }
                try await group.next()
                group.cancelAll()
            }
        } catch {
            // Unblock the gate before cancelling, or the waiting child stays
            // suspended past the group's lifetime and the cancellation is a
            // promise rather than a fact.
            ready.fail(error)
            connection.cancel()
            self.connection = nil
            throw error
        }
        return connection
    }

    /// A one-shot gate, so `connect` can await `.ready` without keeping a
    /// continuation around for the life of the connection.
    private final class ReadyGate: @unchecked Sendable {
        private let lock = NSLock()
        private var continuation: CheckedContinuation<Void, any Error>?
        private var isOpen = false
        private var failure: (any Error)?

        func open() {
            lock.lock()
            defer { lock.unlock() }
            isOpen = true
            failure = nil
            continuation?.resume()
            continuation = nil
        }

        func fail(_ error: any Error) {
            lock.lock()
            defer { lock.unlock() }
            failure = error
            isOpen = true
            continuation?.resume(throwing: error)
            continuation = nil
        }

        func wait() async throws {
            try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, any Error>) in
                lock.lock()
                if isOpen {
                    let failure = self.failure
                    lock.unlock()
                    if let failure { cont.resume(throwing: failure) } else { cont.resume() }
                    return
                }
                continuation = cont
                lock.unlock()
            }
        }
    }

    /// The read loop. One per connection, owned by the actor.
    private func beginReading() {
        connection?.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, error in
            guard let self else { return }
            Task { await self.received(data, isComplete: isComplete, error: error) }
        }
    }

    private func received(_ data: Data?, isComplete: Bool, error: NWError?) {
        if let data, !data.isEmpty {
            inbound.append(data)
            drain()
        }
        if error != nil || isComplete {
            tearDown(error: error.map { CoreError.transport(underlying: $0, method: "read") }
                ?? CoreError.transport(underlying: NSError(domain: "CrossOS", code: 2), method: "read"))
            return
        }
        if connection != nil { beginReading() }
    }

    /// Hand each complete line to whoever is waiting for its id. A line whose id
    /// nobody awaits is dropped rather than parked: the Go client opens a
    /// connection per call so it never sees a late answer, and a continuation
    /// that is resumed twice traps.
    private func drain() {
        // `try?` on a function returning an OPTIONAL collapses two nil meanings
        // into one, so the loop is written as a do/catch over the throwing call
        // and an explicit nil for "no complete line yet". Both nils are
        // different events and only one of them ends the loop.
        while true {
            let next: (line: Data, consumed: Int)?
            do {
                next = try Wire.takeLine(from: &inbound)
            } catch {
                failAll(with: error)
                return
            }
            guard let next else { return }
            let response: WireResponse
            do {
                response = try Wire.decode(next.line)
            } catch {
                failAll(with: error)
                return
            }
            guard let id = response.id, let waiter = pending.removeValue(forKey: id) else {
                continue
            }
            waiter.resume(returning: response)
        }
    }

    private func failAll(with error: any Error) {
        let waiters = pending
        pending.removeAll()
        for (_, waiter) in waiters { waiter.resume(throwing: error) }
    }

    private func tearDown(error: any Error) {
        connection?.stateUpdateHandler = nil
        connection?.cancel()
        connection = nil
        inbound.removeAll()
        failAll(with: error)
    }

    // MARK: The call

    /// One round trip. The one place a request is written and an answer awaited.
    private func call(_ method: String, _ params: JSONValue? = nil) async throws -> JSONValue {
        let connection = try await connect()
        nextID += 1
        let id = nextID
        let request = try Wire.encode(WireRequest(method: method, params: params, id: id))

        let response: WireResponse = try await withCheckedThrowingContinuation { waiter in
            pending[id] = waiter
            connection.send(content: request, completion: .contentProcessed { [weak self] error in
                guard let self, let error else { return }
                Task { await self.failOne(id: id, with: CoreError.transport(underlying: error, method: method)) }
            })
        }

        if let error = response.error {
            throw CoreError.rpc(code: error.code, message: error.message, method: method)
        }
        // A method with no result answers `null`, which is a value here and not
        // a missing answer. `void` methods decode to nil and the caller returns.
        return response.result ?? .null
    }

    private func failOne(id: Int, with error: any Error) {
        pending.removeValue(forKey: id)?.resume(throwing: error)
    }

    private func decode<T: Decodable>(_ type: T.Type, from value: JSONValue, method: String) throws -> T {
        guard let data = try? JSONEncoder().encode(value) else {
            throw CoreError.decode(method: method, underlying: NSError(domain: "CrossOS", code: 3))
        }
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw CoreError.decode(method: method, underlying: error)
        }
    }

    // MARK: CoreClient

    public func status() async throws -> DaemonStatus {
        try decode(DaemonStatus.self, from: await call("core.status"), method: "core.status")
    }

    public func plugins() async throws -> [PluginState] {
        try decode([PluginState].self, from: await call("plugin.list"), method: "plugin.list")
    }

    /// The switcher long-poll, and the one call in the whole protocol with a
    /// deadline worth naming.
    ///
    /// The caller asks for a budget. **The budget is not the deadline.** The Go
    /// client sets its read deadline from the daemon's own 30s cap plus 5s of
    /// slack, whatever was asked for, and ipc_client.go:798-805 says why: a poll
    /// for 300ms can still be served 30s later if the daemon is busy, and
    /// cutting at the ask would "turn a slow answer into a broken switcher".
    ///
    /// So this call may block for 35 seconds. A caller in a `Task` should not
    /// hold a main-actor hop for that, and a cancelled caller does not stop it —
    /// the orphan runs to the daemon's own answer, which is the same behaviour
    /// the React overlay has (`switcher.tsx:152-155` bumps the generation and
    /// clears the timer but leaves the in-flight call alone).
    ///
    /// `triggered: false` is the daemon's "budget expired". It comes back as a
    /// value, and the caller re-polls on it.
    public func switcherWait(timeoutMS: Int) async throws -> SwitcherTrigger {
        let params = JSONValue.object(["timeoutMs": .number(Double(timeoutMS))])
        let raw = try await call("core.switcherWait", params)
        // The daemon answers a pointer deliberately: a literal JSON `null` is
        // rejected as an error rather than decoded to a zero struct
        // (ipc_client.go:813-819), so a triggered=false is a real object.
        return try decode(SwitcherTrigger.self, from: raw, method: "core.switcherWait")
    }

    public func windows() async throws -> [WindowRow] {
        try decode([WindowRow].self, from: await call("core.windows"), method: "core.windows")
    }

    public func readiness() async throws -> [ReadinessRow] {
        try decode([ReadinessRow].self, from: await call("core.readiness"), method: "core.readiness")
    }

    public func eventLogs() async throws -> [String] {
        try decode([String].self, from: await call("core.eventLogs"), method: "core.eventLogs")
    }

    public func pages() async throws -> [Page] {
        try Page.decodeList(await call("core.pages"))
    }

    public func matrix() async throws -> [MatrixRow] {
        try decode([MatrixRow].self, from: await call("config.getMatrix"), method: "config.getMatrix")
    }

    public func overrides() async throws -> [OverrideRow] {
        try decode([OverrideRow].self, from: await call("config.getOverrides"), method: "config.getOverrides")
    }

    public func conflicts() async throws -> [ConflictRow] {
        try decode([ConflictRow].self, from: await call("core.conflicts"), method: "core.conflicts")
    }

    public func observeState() async throws -> ObserveStateRow {
        try decode(ObserveStateRow.self, from: await call("core.observeState"), method: "core.observeState")
    }

    public func apps() async throws -> [AppRow] {
        try decode([AppRow].self, from: await call("core.apps"), method: "core.apps")
    }

    public func profiles() async throws -> [ProfileRow] {
        try decode([ProfileRow].self, from: await call("core.profiles"), method: "core.profiles")
    }

    public func applyProfile(id: String) async throws -> JSONValue {
        try await call("core.profileApply", .object(["id": .string(id)]))
    }

    public func deactivateProfile(id: String) async throws -> JSONValue {
        try await call("core.profileDeactivate", .object(["id": .string(id)]))
    }

    public func setPluginEnabled(id: String, enabled: Bool) async throws {
        // The scan first, because `plugin.setEnabled` on an unknown id is a
        // success that did nothing — and a plugin page whose switch works
        // visually while the plugin is not there is the exact failure the
        // React bridge closed with the same check.
        let known = try await plugins()
        guard known.contains(where: { $0.id == id }) else {
            throw CoreError.rpc(code: -32602, message: "no such plugin: \(id)", method: "plugin.setEnabled")
        }
        _ = try await call(
            "plugin.setEnabled",
            .object(["id": .string(id), "enabled": .bool(enabled)])
        )
    }

    public func setFileType(entry: FileTypeRow, enabled: Bool) async throws -> [FileTypeRow] {
        let params = JSONValue.object([
            "ext": .string(entry.ext),
            "baseName": .string(entry.baseName),
            "enabled": .bool(enabled),
        ])
        return try decode([FileTypeRow].self, from: await call("core.setFileType", params), method: "core.setFileType")
    }

    public func shortcuts() async throws -> [JSONValue] {
        try decode([JSONValue].self, from: await call("config.getShortcuts"), method: "config.getShortcuts")
    }

    public func setShortcuts(_ rows: [JSONValue]) async throws -> Int {
        let params = JSONValue.array(rows)
        let raw = try await call("config.setShortcuts", params)
        return raw.intValue ?? 0
    }

    public func zones() async throws -> [ZoneRow] {
        try decode([ZoneRow].self, from: await call("config.getZones"), method: "config.getZones")
    }

    public func setZones(_ zones: [ZoneRow]) async throws -> Int {
        let data = try JSONEncoder().encode(zones)
        let rows = try JSONDecoder().decode([JSONValue].self, from: data)
        let raw = try await call("config.setZones", .array(rows))
        return raw.intValue ?? 0
    }

    public func userRules() async throws -> [UserRuleRow] {
        try decode([UserRuleRow].self, from: await call("config.getUserRules"), method: "config.getUserRules")
    }

    public func appsForRules() async throws -> [AppRow] {
        try decode([AppRow].self, from: await call("core.apps"), method: "core.apps")
    }

    public func commands() async throws -> [CommandRow] {
        try decode([CommandRow].self, from: await call("core.commands"), method: "core.commands")
    }

    public func pluginMeta() async throws -> [PluginMetaRow] {
        try decode([PluginMetaRow].self, from: await call("core.pluginMeta"), method: "core.pluginMeta")
    }

    public func traces() async throws -> [TraceRow] {
        try decode([TraceRow].self, from: await call("core.traces"), method: "core.traces")
    }

    /// The erase behind the Activity page's clear button.
    ///
    /// It answers with the rows that REMAIN, not a count. That is deliberate
    /// (bridge.go:228-231): a count after a clear is a number that cannot be
    /// wrong, so it is the answer that hides a clear that did not happen, and
    /// the table that re-renders from it is the thing that would show the
    /// truth.
    public func clearTraces() async throws -> [TraceRow] {
        try decode([TraceRow].self, from: await call("core.tracesClear"), method: "core.tracesClear")
    }

    public func fileTypes() async throws -> [FileTypeRow] {
        try decode([FileTypeRow].self, from: await call("core.fileTypes"), method: "core.fileTypes")
    }

    public func trialState() async throws -> TrialState {
        try decode(TrialState.self, from: await call("safety.trialState"), method: "safety.trialState")
    }

    public func beginTrial(plugin: String) async throws -> String {
        let raw = try await call("safety.beginTrial", .object(["plugin": .string(plugin)]))
        return raw.stringValue ?? ""
    }

    public func confirmTrial(plugin: String, healthy: Bool) async throws -> String {
        let params = JSONValue.object([
            "plugin": .string(plugin),
            "confirmed": .bool(true),
            "healthy": .bool(healthy),
        ])
        let raw = try await call("safety.confirmTrial", params)
        return raw.stringValue ?? ""
    }

    public func rollbackTrial(plugin: String) async throws -> String {
        let raw = try await call("safety.rollbackTrial", .object(["plugin": .string(plugin)]))
        return raw.stringValue ?? ""
    }

    public func panicStop() async throws -> JSONValue {
        try await call("safety.panicStop", .object([:]))
    }

    public func resume() async throws -> JSONValue {
        try await call("safety.resume", .object([:]))
    }

    public func setRuleEnabled(ruleID: String, enabled: Bool) async throws -> Bool {
        let params = JSONValue.object(["rule_id": .string(ruleID), "enabled": .bool(enabled)])
        // The answer is an object with the stored state in it, not a bare
        // bool — the Go client unwraps `.enabled` from it.
        let raw = try await call("config.setRuleEnabled", params)
        let object = raw.objectValue ?? [:]
        return object["enabled"]?.boolValue ?? false
    }
}
