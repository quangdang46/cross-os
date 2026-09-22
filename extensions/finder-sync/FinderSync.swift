import Cocoa
import FinderSync
import os

// CrossOS Finder Sync Platform Extension (bead cross-os-vbl.1).
//
// COPY/ADAPT source (§9.10 newfile row, commit b3f665a): Extension/FinderSync.swift
// — FIFinderSync subclass, tag-indexed menu snapshot, target-dir resolution,
// create + reveal. Adapted: DistributedNotificationCenter + host-app launch
// REPLACED by the Unix-socket IPC client (D2 transport decision, §13b D2).
// The extension sends one JSON request {action, targetDir, ext, baseName}
// and applies the daemon's reply. Daemon-down → nil menu (hide, never block).
// Host-app UI (SwiftUI prefs, template editor) is NOT copied — settings are
// declarative (§3.6c). (No app-group SettingsStore exists in tree yet; the
// enabledEntries stub below names the deferred vbl.2/vbl.4 wiring.)
//
// Merge gate (§9.11): attribution in third_party/newfile/.

private let log = Logger(subsystem: "dev.crossos.finder-sync", category: "extension")

/// Daemon socket path shared with the Go IPC client (extensions/finder-sync/ipc.go).
private let daemonSocketPath = "\(NSHomeDirectory())/Library/Application Support/CrossOS/crossos.sock"

struct MenuEntry: Codable {
    var title: String
    var action: String
    var ext: String
    var baseName: String
}

struct DaemonRequest: Codable {
    var action: String
    var targetDir: String
    var ext: String?
    var baseName: String?
}

struct DaemonResponse: Codable {
    var ok: Bool
    var error: String?
}

final class FinderSync: FIFinderSync {

    private var menuEntrySnapshot: [MenuEntry] = []

    override init() {
        super.init()
        FIFinderSyncController.default().directoryURLs = [URL(fileURLWithPath: "/")]
        log.info("CrossOS FinderSync init")
    }

    // MARK: - Menus

    override func menu(for menu: FIMenuKind) -> NSMenu? {
        // Daemon-down: hide (nil menu), never block Finder. Probe with a
        // short connect; the action path redials with its own deadline.
        guard daemonReachable(timeoutMs: 150) else { return nil }
        return buildContextMenu()
    }

    private func buildContextMenu() -> NSMenu {
        let menu = NSMenu(title: "")
        menuEntrySnapshot = []
        let entries = enabledEntries()
        if entries.isEmpty {
            let item = NSMenuItem(title: "Enable a file type in CrossOS…", action: #selector(openPreferences(_:)), keyEquivalent: "")
            item.target = self
            menu.addItem(item)
            return menu
        }
        for entry in entries { addRow(for: entry, to: menu) }
        return menu
    }

    private func addRow(for entry: MenuEntry, to menu: NSMenu) {
        let item = NSMenuItem(title: entry.title, action: #selector(createFromMenuItem(_:)), keyEquivalent: "")
        item.target = self
        item.tag = menuEntrySnapshot.count
        menuEntrySnapshot.append(entry)
        menu.addItem(item)
    }

    @objc func openPreferences(_ sender: AnyObject?) {
        // Declarative settings live in the CrossOS shell (§3.6c); the
        // extension only latches intent — it never hosts prefs UI.
        log.info("openPreferences requested")
    }

    // MARK: - Actions

    @objc func createFromMenuItem(_ sender: AnyObject?) {
        guard let item = sender as? NSMenuItem,
              menuEntrySnapshot.indices.contains(item.tag) else {
            log.error("createFromMenuItem: tag out of range (snapshot=\(self.menuEntrySnapshot.count))")
            NSSound.beep()
            return
        }
        performCreate(entry: menuEntrySnapshot[item.tag])
    }

    private func performCreate(entry: MenuEntry) {
        let controller = FIFinderSyncController.default()
        let target = controller.targetedURL()
        let selected = controller.selectedItemURLs()
        guard let directory = directoryForCreation(target: target, selected: selected) else {
            log.error("No target directory available")
            NSSound.beep()
            return
        }
        // Use the entry's own verb (review: cross-os-ed): today's stubs are
        // all file-creation, but hardcoding "createFile" here would make the
        // MenuEntry action field decorative the day a second verb exists.
        let req = DaemonRequest(action: entry.action, targetDir: directory.path, ext: entry.ext, baseName: entry.baseName)
        do {
            let resp = try sendToDaemon(req)
            if !resp.ok {
                log.error("daemon refused: \(resp.error ?? "unknown")")
                NSSound.beep()
            }
        } catch {
            // Daemon-down at action time: fail gracefully (beep), never block.
            log.error("daemon unreachable: \(error.localizedDescription)")
            NSSound.beep()
        }
    }

    // MARK: - IPC client (Unix socket, JSON — mirrors ipc.go Send)

    private func sendToDaemon(_ req: DaemonRequest) throws -> DaemonResponse {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { throw IPCError.noSocket }
        defer { close(fd) }
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let path = daemonSocketPath
        _ = path.withCString { ptr in
            withUnsafeMutablePointer(to: &addr.sun_path.0) { dest in
                strncpy(dest, ptr, MemoryLayout.size(ofValue: addr.sun_path) - 1)
            }
        }
        let status = withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard status == 0 else { throw IPCError.unreachable }
        var tv = timeval(tv_sec: 0, tv_usec: 500 * 1000)
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        let payload = try JSONEncoder().encode(req)
        var written = 0
        try payload.withUnsafeBytes { buf in
            while written < payload.count {
                let n = write(fd, buf.baseAddress!.advanced(by: written), payload.count - written)
                if n <= 0 { throw IPCError.io }
                written += n
            }
        }
        var out = Data()
        var chunk = [UInt8](repeating: 0, count: 4096)
        while true {
            let n = read(fd, &chunk, chunk.count)
            if n <= 0 { break }
            out.append(contentsOf: chunk[..<n])
            if out.contains(0x0A) { break }
        }
        guard !out.isEmpty else { throw IPCError.io }
        return try JSONDecoder().decode(DaemonResponse.self, from: out)
    }

    private func daemonReachable(timeoutMs: Int) -> Bool {
        let fd = socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else { return false }
        defer { close(fd) }
        var tv = timeval(tv_sec: 0, tv_usec: Int32(timeoutMs * 1000))
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        _ = daemonSocketPath.withCString { ptr in
            withUnsafeMutablePointer(to: &addr.sun_path.0) { dest in
                strncpy(dest, ptr, MemoryLayout.size(ofValue: addr.sun_path) - 1)
            }
        }
        return withUnsafePointer(to: &addr) {
            $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        } == 0
    }

    enum IPCError: Error {
        case noSocket
        case unreachable
        case io
    }

    // MARK: - Helpers (adapted from newfile: directoryForCreation/resolvedDirectory)

    private func enabledEntries() -> [MenuEntry] {
        // App-group store (daemon mirrors enabled types here for the
        // menu-open path; seeded from the parity list on first read).
        // Store missing (no entitlement in this context) → [] → empty row.
        guard let store = CrossOSSettingsStore.appGroupStore() else { return [] }
        return store.enabledTypes.map { $0.menuEntry() }
    }

    private func directoryForCreation(target: URL?, selected: [URL]?) -> URL? {
        if let target { return resolvedDirectory(for: target) }
        if let first = selected?.first { return resolvedDirectory(for: first) }
        return nil
    }

    private func resolvedDirectory(for url: URL) -> URL {
        var isDir: ObjCBool = false
        if FileManager.default.fileExists(atPath: url.path, isDirectory: &isDir),
           isDir.boolValue {
            return url
        }
        return url.deletingLastPathComponent()
    }
}
