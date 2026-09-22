import Foundation
import os

private let storeLog = Logger(subsystem: "dev.crossos.finder-sync", category: "store")

// CrossOS app-group settings store (bead cross-os-dnj).
//
// COPY/ADAPT source (§9.10 newfile row, commit b3f665a):
// Shared/SettingsStore.swift + Shared/FileTypeEntry.swift +
// Shared/SeedPresets.swift — adapted, not forked:
//   - appGroupID is CrossOS's own group ($(TEAM_ID).dev.crossos.finder-sync,
//     matching CrossOSFinderSync.entitlements), NOT newfile's Q7VD7MTRL8 ID.
//   - FileTypeEntry gains menuAction/baseName parity with the Go FileType
//     (extensions/finder-sync/naming.go) so menu rows serialize identically.
//   - SeedPresets parity: same 8 built-ins, same enabled default (txt only).
//   - Schema versioning kept (currentSchema 2, same migration shape).
// Host-app UI (SwiftUI prefs, template editor) is NOT copied — settings are
// declarative (§3.6c); the daemon is source of truth, the store mirrors it
// for the menu-open path.

struct CrossOSFileType: Codable, Equatable {
    var ext: String
    var baseName: String
    var displayName: String
    var template: String
    var enabled: Bool
    var isBuiltIn: Bool

    /// Menu label: explicit displayName, else derived "New .<ext>"
    /// (parity with Go FileType.MenuTitle).
    var menuTitle: String {
        let t = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? "New .\(ext)" : t
    }

    /// MenuEntry payload for FinderSync rows + daemon requests.
    func menuEntry() -> MenuEntry {
        MenuEntry(title: menuTitle, action: "createFile", ext: ext, baseName: baseName)
    }
}

enum CrossOSSeedPresets {
    static let builtIns: [CrossOSFileType] = [
        CrossOSFileType(ext: "txt", baseName: "New Text File", displayName: "New Text File", template: "", enabled: true, isBuiltIn: true),
        CrossOSFileType(ext: "md", baseName: "Untitled", displayName: "New Markdown", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "env", baseName: "", displayName: "New .env", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "json", baseName: "data", displayName: "New JSON", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "yml", baseName: "config", displayName: "New YAML", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "sh", baseName: "script", displayName: "New Shell Script", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "gitignore", baseName: "", displayName: "New .gitignore", template: "", enabled: false, isBuiltIn: true),
        CrossOSFileType(ext: "html", baseName: "index", displayName: "New HTML", template: "", enabled: false, isBuiltIn: true),
    ]
}

final class CrossOSSettingsStore {
    // Team-ID-prefixed group (containermanagerd rejects non-prefixed groups).
    // Must match CrossOSFinderSync.entitlements
    // ($(TEAM_ID).dev.crossos.finder-sync). DEBUG uses a stable suite name
    // so unit tests run without an entitlement context.
    static let appGroupID: String = {
        #if DEBUG
        return "GROUP.crossos.finder-sync"
        #else
        // Fail LOUD on missing TEAM_ID (review: cross-os-8d): silently
        // falling back to a wrong suite name yields an empty menu that is
        // hard to debug (looks like "no enabled types", not misconfig).
        guard let team = ProcessInfo.processInfo.environment["TEAM_ID"], !team.isEmpty else {
            storeLog.error("TEAM_ID missing — app-group suite unresolvable; check signing environment")
            return "MISSING-TEAM-ID.crossos.finder-sync"
        }
        return "\(team).dev.crossos.finder-sync"
        #endif
    }()

    private enum Key {
        static let fileTypes = "fileTypes"
        static let schema = "schemaVersion"
    }

    private static let currentSchema = 2

    private let defaults: UserDefaults

    /// Production: UserDefaults(suiteName: appGroupID)!. Tests: isolated suite.
    init(defaults: UserDefaults) {
        self.defaults = defaults
    }

    static func appGroupStore() -> CrossOSSettingsStore? {
        guard let suite = UserDefaults(suiteName: appGroupID) else { return nil }
        return CrossOSSettingsStore(defaults: suite)
    }

    var fileTypes: [CrossOSFileType] {
        get {
            if let data = defaults.data(forKey: Key.fileTypes),
               let decoded = try? JSONDecoder().decode([CrossOSFileType].self, from: data) {
                return decoded
            }
            let seeded = CrossOSSeedPresets.builtIns
            persist(seeded)
            defaults.set(Self.currentSchema, forKey: Key.schema)
            return seeded
        }
        set { persist(newValue) }
    }

    var enabledTypes: [CrossOSFileType] {
        fileTypes.filter { $0.enabled }
    }

    private func persist(_ types: [CrossOSFileType]) {
        guard let data = try? JSONEncoder().encode(types) else { return }
        defaults.set(data, forKey: Key.fileTypes)
    }
}
