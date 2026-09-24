import Cocoa
import FinderSync
import os

// CrossOS Finder Sync Platform Extension (bead cross-os-vbl.1).
//
// COPY/ADAPT source (§9.10 newfile row, commit b3f665a): Extension/FinderSync.swift
// — FIFinderSync subclass, tag-indexed menu snapshot, target-dir resolution,
// submenu building. Adapted: the settings store and host-app launch are gone.
//
// This file runs INSIDE the appex sandbox, which decides the whole design
// (tmp/research/mac-finder-menu/docs/FINDINGS.md:59-68): a child process the
// extension spawns inherits the seatbelt, and a CLI launched that way dies
// silently while reporting success. So the appex never runs anything and
// never passes argv — argv does not survive the trip out of a sandboxed
// caller (FINDINGS.md:88-95) either. Every action is a socket request to the
// daemon, which is unsandboxed and does the deciding.
//
// The menu is likewise data, not local truth: finder.menuEntries is the
// daemon's own table (core/pkg/findermenu), so the menu Finder draws and the
// verbs the daemon executes cannot be two lists. A first-run user who has
// never opened the settings app still gets the built-in file types, because
// the catalog ships with the daemon rather than with a store this process
// would have had to write to first.
//
// Merge gate (§9.11): attribution in third_party/newfile/ATTRIBUTION.md:1-7.

private let log = Logger(subsystem: "dev.crossos.finder-sync", category: "extension")

/// Daemon socket path shared with the Go IPC client (extensions/finder-sync/ipc.go).
private let daemonSocketPath = "\(NSHomeDirectory())/Library/Application Support/CrossOS/crossos.sock"

/// Menu-open must not be felt in Finder: a slow daemon hides the menu
/// instead of holding the right-click.
private let queryTimeoutMs = 150

/// Actions run off the click, so they get the Go client's full budget.
private let actionTimeoutMs = 500

// MARK: - JSON-RPC 2.0 wire

/// The request the appex writes. `id` is always present — the daemon rejects
/// a notification, so omitting it would make every call a silent no-op.
private struct RPCRequest: Encodable {
    var jsonrpc = "2.0"
    var method: String
    var params: RPCParams
    var id: Int
}

/// One flat payload for every method, because JSON-RPC params are an untyped
/// object and the daemon reads the keys its own handler needs. The key names
/// are the daemon's (core/cmd/crossos/findermenu.go); a param spelled
/// differently here is a missing-param refusal the user sees as a menu item
/// that does nothing.
private struct RPCParams: Encodable {
    var context: String?
    var dir: String?
    var path: String?
    var baseDir: String?
    var paths: [String]?
    var ext: String?
    var baseName: String?
    var editorID: String?

    init(context: String? = nil, dir: String? = nil, path: String? = nil,
         baseDir: String? = nil, paths: [String]? = nil, ext: String? = nil,
         baseName: String? = nil, editorID: String? = nil) {
        self.context = context
        self.dir = dir
        self.path = path
        self.baseDir = baseDir
        self.paths = paths
        self.ext = ext
        self.baseName = baseName
        self.editorID = editorID
    }
}

private struct RPCError: Decodable, Error {
    var code: Int
    var message: String
}

/// The response. A refusal is `error`, never a result carrying ok:false —
/// otherwise a caller can read a failure as success.
private struct RPCResponse: Decodable {
    var jsonrpc: String?
    var result: MenuPayload?
    var error: RPCError?
    var id: Int?
}

/// finder.menuEntries' result. Every field is optional and defaults to
/// empty: a source with no handler must render an empty menu, not fail to
/// decode and take the whole right-click with it.
private struct MenuPayload: Decodable {
    var contexts: [String]?
    var items: [MenuItemPayload]?
    var editors: [EditorPayload]?
    var fileTypes: [FileTypePayload]?
}

private struct MenuItemPayload: Decodable {
    var id: String
    var title: String
    var contexts: [String]?
    var needsPaths: Bool?
    var enabled: Bool?
}

/// One entry of the Open in Editor submenu. id is the bundle id — the same
/// string the daemon launches, so the menu and the launch cannot name an
/// editor two different ways.
private struct EditorPayload: Decodable {
    var id: String
    var name: String
}

private struct FileTypePayload: Decodable {
    var ext: String
    var baseName: String?
    var displayName: String?
    var enabled: Bool?

    /// The daemon's own label rule: explicit displayName, else "New .<ext>".
    /// Computed here rather than sent, because filetype.FileType — what
    /// finder.menuEntries actually returns — carries no menuTitle field.
    var menuTitle: String {
        if let displayName, !displayName.trimmingCharacters(in: .whitespaces).isEmpty {
            return displayName
        }
        return "New .\(ext)"
    }
}

// MARK: - Pending action

/// One clickable row, carried across the FinderSync XPC boundary by index:
/// representedObject cannot hold a Swift struct here, which is why newfile
/// indexes by NSMenuItem.tag and so does this.
private struct PendingAction {
    var verb: String
    var ext: String?
    var baseName: String?
    var editorID: String?
}

final class FinderSync: FIFinderSync {

    private var menuEntrySnapshot: [PendingAction] = []

    private let idLock = NSLock()
    private var nextID = 1

    override init() {
        super.init()
        FIFinderSyncController.default().directoryURLs = [URL(fileURLWithPath: "/")]
        log.info("CrossOS FinderSync init")
    }

    /// Monotonic per-process call id. JSON-RPC only needs ids distinct among
    /// calls in flight, and this is the simplest thing that is.
    private func takeID() -> Int {
        idLock.lock()
        defer { idLock.unlock() }
        let id = nextID
        nextID += 1
        return id
    }

    // MARK: - Menus

    override func menu(for menu: FIMenuKind) -> NSMenu? {
        // Daemon-down: hide (nil menu), never block Finder. There is no
        // separate reachability probe — the menu query IS the probe, so an
        // idle right-click costs one round-trip and not two.
        let context = selectionContext()
        let payload: MenuPayload
        do {
            payload = try queryMenu(context: context, timeoutMs: queryTimeoutMs)
        } catch {
            log.info("menu hidden: \(String(describing: error))")
            return nil
        }
        return buildMenu(payload: payload, context: context)
    }

    private func buildMenu(payload: MenuPayload, context: String) -> NSMenu {
        let menu = NSMenu(title: "")
        menuEntrySnapshot = []
        let rows = (payload.items ?? []).filter { row in
            guard row.enabled ?? true else { return false }
            return row.contexts?.contains(context) == true
        }
        if rows.isEmpty {
            // The daemon is up and answered, but nothing applies here. One
            // inert row beats an empty right-click the user reads as a
            // broken extension.
            let item = NSMenuItem(title: "Nothing to do here in CrossOS", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
            return menu
        }
        for row in rows {
            switch row.id {
            case "newFile":
                addNewFileSubmenu(to: menu, fileTypes: payload.fileTypes ?? [])
            case "openEditor":
                addEditorSubmenu(to: menu, editors: payload.editors ?? [])
            case "newFolder":
                addRow(verb: "finder.createFolder", title: row.title, to: menu)
            case "copyPath":
                addRow(verb: "finder.copyPath", title: row.title, to: menu)
            case "copyRelativePath":
                addRow(verb: "finder.copyRelativePath", title: row.title, to: menu)
            case "openTerminal":
                addRow(verb: "finder.openTerminal", title: row.title, to: menu)
            case "duplicateWithName":
                addRow(verb: "finder.duplicateWithName", title: row.title, to: menu)
            default:
                // A row this build does not know how to fire. Rendered as
                // inert rather than silently dropped: a row the daemon added
                // and the appex has not caught up with should be visible as
                // "not yet", not invisible as "not offered".
                let item = NSMenuItem(title: row.title, action: nil, keyEquivalent: "")
                item.isEnabled = false
                menu.addItem(item)
            }
        }
        return menu
    }

    /// The enabled file types the daemon serves. A first-run user gets the
    /// built-in set without having opened a settings app once, because the
    /// catalog lives in the daemon — not in a store this process writes.
    private func addNewFileSubmenu(to menu: NSMenu, fileTypes: [FileTypePayload]) {
        let enabled = fileTypes.filter { $0.enabled ?? true }
        let parent = NSMenuItem(title: "New File", action: nil, keyEquivalent: "")
        let sub = NSMenu(title: "New File")
        for fileType in enabled {
            let action = PendingAction(verb: "finder.createFile", ext: fileType.ext,
                                       baseName: fileType.baseName, editorID: nil)
            let item = NSMenuItem(title: fileType.menuTitle,
                                  action: #selector(dispatchFromMenuItem(_:)), keyEquivalent: "")
            item.target = self
            item.tag = menuEntrySnapshot.count
            menuEntrySnapshot.append(action)
            sub.addItem(item)
        }
        parent.submenu = sub
        menu.addItem(parent)
    }

    /// The editor catalog. The submenu offers exactly what the daemon can
    /// launch: a submenu entry the daemon would refuse is a menu item that
    /// fails on click, so the one list feeds both ends.
    private func addEditorSubmenu(to menu: NSMenu, editors: [EditorPayload]) {
        let parent = NSMenuItem(title: "Open in Editor", action: nil, keyEquivalent: "")
        let sub = NSMenu(title: "Open in Editor")
        for editor in editors {
            let action = PendingAction(verb: "finder.openEditor", ext: nil,
                                       baseName: nil, editorID: editor.id)
            let item = NSMenuItem(title: editor.name,
                                  action: #selector(dispatchFromMenuItem(_:)), keyEquivalent: "")
            item.target = self
            item.tag = menuEntrySnapshot.count
            menuEntrySnapshot.append(action)
            sub.addItem(item)
        }
        parent.submenu = sub
        menu.addItem(parent)
    }

    private func addRow(verb: String, title: String, to menu: NSMenu) {
        let action = PendingAction(verb: verb, ext: nil, baseName: nil, editorID: nil)
        let item = NSMenuItem(title: title, action: #selector(dispatchFromMenuItem(_:)), keyEquivalent: "")
        item.target = self
        item.tag = menuEntrySnapshot.count
        menuEntrySnapshot.append(action)
        menu.addItem(item)
    }

    // MARK: - Actions

    @objc private func dispatchFromMenuItem(_ sender: AnyObject?) {
        guard let item = sender as? NSMenuItem,
              menuEntrySnapshot.indices.contains(item.tag) else {
            log.error("dispatch: tag out of range (snapshot=\(menuEntrySnapshot.count))")
            NSSound.beep()
            return
        }
        let action = menuEntrySnapshot[item.tag]
        do {
            let response = try call(method: action.verb,
                                    params: params(for: action),
                                    timeoutMs: actionTimeoutMs)
            if let error = response.error {
                log.error("daemon refused \(action.verb): \(error.message)")
                NSSound.beep()
            }
        } catch {
            // Daemon-down at action time: fail gracefully (beep), never block.
            log.error("daemon unreachable: \(String(describing: error))")
            NSSound.beep()
        }
    }

    /// The payload each verb's handler reads. The split is the daemon's:
    /// creation acts on a folder, opening acts on one target, copy and
    /// duplicate act on the selection. A new file has no path of its own yet,
    /// so it cannot be sent one.
    private func params(for action: PendingAction) -> RPCParams {
        switch action.verb {
        case "finder.createFile", "finder.createFolder":
            return RPCParams(dir: creationDirectory())
        case "finder.openTerminal":
            return RPCParams(path: creationDirectory())
        case "finder.openEditor":
            return RPCParams(path: primaryPath(), editorID: action.editorID)
        case "finder.copyPath", "finder.duplicateWithName":
            return RPCParams(paths: selectionPaths())
        case "finder.copyRelativePath":
            return RPCParams(paths: selectionPaths(), baseDir: creationDirectory())
        default:
            return RPCParams()
        }
    }

    // MARK: - Selection

    /// The context the daemon should build a menu for, read from the
    /// controller rather than assumed.
    ///
    /// A live selection wins over the target: a right-click on a selected item
    /// and a right-click on the folder containing it both report something as
    /// the target, and only the selection says which one the user meant. With
    /// neither, the click was on empty space.
    private func selectionContext() -> String {
        let selected = FIFinderSyncController.default().selectedItemURLs() ?? []
        if !selected.isEmpty {
            return selected.allSatisfy(isDirectory) ? "folder" : "file"
        }
        if let targeted = FIFinderSyncController.default().targetedURL() {
            return isDirectory(targeted) ? "folder" : "file"
        }
        return "empty"
    }

    private func isDirectory(_ url: URL) -> Bool {
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: url.path, isDirectory: &isDir) else { return false }
        return isDir.boolValue
    }

    /// The paths a copy or a duplicate acts on: the selection when there is
    /// one, else the item that was clicked.
    private func selectionPaths() -> [String] {
        let controller = FIFinderSyncController.default()
        let selected = controller.selectedItemURLs() ?? []
        if !selected.isEmpty { return selected.map(\.path) }
        if let targeted = controller.targetedURL() { return [targeted.path] }
        return []
    }

    /// The single item an Open verb acts on.
    private func primaryPath() -> String {
        return selectionPaths().first ?? ""
    }

    /// The folder a create or a terminal acts in (adapted from newfile's
    /// directoryForCreation / resolvedDirectory).
    private func creationDirectory() -> String? {
        let controller = FIFinderSyncController.default()
        let target = controller.targetedURL() ?? controller.selectedItemURLs()?.first
        guard let url = target else { return nil }
        return isDirectory(url) ? url.path : url.deletingLastPathComponent().path
    }

    // MARK: - IPC client (Unix socket, JSON-RPC 2.0 — mirrors ipc.go Send)

    private func queryMenu(context: String, timeoutMs: Int) throws -> MenuPayload {
        let response = try call(method: "finder.menuEntries",
                                params: RPCParams(context: context),
                                timeoutMs: timeoutMs)
        // An empty result is an empty menu, not a decode failure: the daemon
        // being up but offering nothing must not read as a broken extension.
        return response.result ?? MenuPayload(contexts: [], items: [], editors: [], fileTypes: [])
    }

    private func call(method: String, params: RPCParams, timeoutMs: Int) throws -> RPCResponse {
        let payload = try JSONEncoder().encode(RPCRequest(method: method, params: params, id: takeID()))
        let fd = try connect(timeoutMs: timeoutMs)
        defer { close(fd) }
        try writeAll(payload, to: fd)
        let raw = try readLine(from: fd)
        return try JSONDecoder().decode(RPCResponse.self, from: raw)
    }

    private func connect(timeoutMs: Int) throws -> Int32 {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw IPCError.noSocket }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let path = daemonSocketPath
        _ = path.withCString { ptr in
            withUnsafeMutablePointer(to: &addr.sun_path.0) { dest in
                strncpy(dest, ptr, MemoryLayout.size(ofValue: addr.sun_path) - 1)
            }
        }
        // Bound the whole exchange, dial included: a connect() that blocks is
        // a Finder that stops responding to right-clicks.
        var tv = timeval(tv_sec: 0, tv_usec: Int32(timeoutMs * 1000))
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        let status = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard status == 0 else {
            close(fd)
            throw IPCError.unreachable
        }
        return fd
    }

    private func writeAll(_ payload: Data, to fd: Int32) throws {
        var written = 0
        try payload.withUnsafeBytes { buf in
            while written < payload.count {
                let n = write(fd, buf.baseAddress!.advanced(by: written), payload.count - written)
                if n <= 0 { throw IPCError.io }
                written += n
            }
        }
    }

    /// The daemon writes one JSON object per line, so a newline ends the
    /// reply. Reading to EOF instead would wait for the daemon to close,
    /// which it does not do between calls.
    private func readLine(from fd: Int32) throws -> Data {
        var out = Data()
        var chunk = [UInt8](repeating: 0, count: 4096)
        while true {
            let n = read(fd, &chunk, chunk.count)
            if n > 0 {
                out.append(contentsOf: chunk[..<n])
                if out.contains(0x0A) { break }
                continue
            }
            if n == 0 { break } // peer closed without a newline
            throw IPCError.io
        }
        guard !out.isEmpty else { throw IPCError.io }
        return out
    }

    private enum IPCError: Error {
        case noSocket
        case unreachable
        case io
    }
}
