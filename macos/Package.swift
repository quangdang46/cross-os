// swift-tools-version: 6.0
import PackageDescription

// CrossOS.app — the native macOS shell.
//
// This package is the UI half of the product. The other half is the Go daemon
// (`core/`, unchanged), and the two speak JSON-RPC 2.0 over a Unix domain
// socket. There is no third layer: the Wails bridge that used to sit between
// them is gone, because it was never really a layer — `app/backend/` imported
// nothing from Wails except in one file, and its 7,735 lines were a typed
// mirror of a wire protocol that already existed underneath it.
//
// CrossOSCore is the client. It is a library and not an executable so that the
// UI target, the CLI probe and the tests all exercise the SAME code — a
// client that only the app can reach is a client nothing tests.
let package = Package(
    name: "CrossOS",
    platforms: [.macOS(.v14)],
    products: [
        .library(name: "CrossOSCore", targets: ["CrossOSCore"]),
        .executable(name: "crossos-probe", targets: ["crossos-probe"]),
        .executable(name: "CrossOS", targets: ["CrossOSApp"]),
    ],
    targets: [
        .target(
            name: "CrossOSCore",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        // The app. AppKit, no SwiftUI: the closest analogue in this ecosystem
        // is Rectangle, 31,065 lines of Swift with zero `import SwiftUI`, and
        // the surface this app needs — a source list, a stack of rows, a
        // borderless key window for the switcher — is what AppKit draws
        // natively. A webview was the thing being replaced.
        .executableTarget(
            name: "CrossOSApp",
            dependencies: ["CrossOSCore"],
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        // The probe is a thin CLI over the client: every RPC the UI can make,
        // callable from a terminal. It is how the port is verified — phase 1
        // is done when `crossos-probe` reproduces the numbers in
        // docs/baseline/ from the daemon alone.
        .executableTarget(
            name: "crossos-probe",
            dependencies: ["CrossOSCore"],
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        // The tests, as an executable. See Harness.swift for why this is not a
        // `.testTarget` — in short, neither XCTest nor swift-testing is
        // reachable from Command Line Tools, and a test suite that cannot run
        // is worse than one that runs the wrong way.
        .executableTarget(
            name: "CoreTests",
            dependencies: ["CrossOSCore"],
            path: "Tests/Harness/Sources/CoreTests",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
