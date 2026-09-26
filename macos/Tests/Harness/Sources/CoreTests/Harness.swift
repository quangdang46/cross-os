import Darwin
import Foundation

// A test runner with no framework underneath.
//
// Neither XCTest nor swift-testing is reachable from this toolchain: the
// machine has Command Line Tools rather than the full Xcode, and `import
// Testing` fails with "no such module" even though
// `/Library/Developer/CommandLineTools/Library/Developer/Frameworks/Testing.framework`
// is on disk. swiftc finds it with an explicit `-F`; SwiftPM does not pass one,
// so a `.testTarget` is dead on arrival here.
//
// That is an environment fact, not a design choice, so the fix is not to design
// around XCTest. It is to make the assertions the harness rather than the
// framework, so the same test source runs under `swift test` on a machine with
// Xcode and under `swift run CoreTests` here. Two ways to run one set of tests
// beats one way that does not run at all.
//
// The macro surface is deliberately tiny — the four shapes below are the four
// this codebase's tests actually use. When Xcode lands, `swift test` is the
// command; the assertions do not move.

// The runner is single-threaded by construction — one file, run top to
// bottom, exit at the end — so these are @MainActor rather than
// locked. Swift 6 is right that a mutable global needs a reason, and
// this is the reason.
@MainActor public var failures: [String] = []
@MainActor public var checks = 0

/// The common case: a check that cannot throw, written as a bare expression.
/// The trailing-closure form below is for a check that decodes, dials or
/// unwraps.
@MainActor public func expect(_ condition: @autoclosure () -> Bool, _ what: String) {
    checks += 1
    if !condition() {
        failures.append(what)
        print("  ✗ \(what)")
    }
}

/// A check that can throw, written as a closure.
@MainActor public func expect(_ condition: () throws -> Bool, _ what: String) {
    checks += 1
    let passed: Bool
    do {
        passed = try condition()
    } catch {
        failures.append("\(what) — threw \(error)")
        print("  ✗ \(what) — threw \(error)")
        return
    }
    if !passed {
        failures.append(what)
        print("  ✗ \(what)")
    }
}

@MainActor public func expectEqual<T: Equatable>(_ actual: T, _ expected: T, _ what: String) {
    checks += 1
    if actual != expected {
        failures.append("\(what) — got \(actual), wanted \(expected)")
        print("  ✗ \(what) — got \(actual), wanted \(expected)")
    }
}

/// Unwrap an optional or fail the test. `#require` in swift-testing, and the
/// difference from a plain `guard` is that a nil is reported rather than
/// silently skipping the rest of the test.
@MainActor public func require<T>(_ value: T?, _ what: String) -> T? {
    checks += 1
    guard let value else {
        failures.append("\(what) — required a value, got nil")
        print("  ✗ \(what) — required a value, got nil")
        return nil
    }
    return value
}

/// Assert that a closure throws. `#expect(throws:)` in swift-testing.
@MainActor public func expectThrows(_ what: String, _ body: () throws -> Void) {
    checks += 1
    do {
        try body()
        failures.append("\(what) — expected a throw, got none")
        print("  ✗ \(what) — expected a throw, got none")
    } catch {
        // expected
    }
}

/// The summary and an exit code, so a CI step and a human read the same thing.
@MainActor public func report() -> Never {
    print("")
    if failures.isEmpty {
        print("✓ \(checks) checks, no failures")
        exit(0)
    }
    print("✗ \(failures.count) of \(checks) checks failed:")
    for failure in failures { print("    \(failure)") }
    exit(1)
}

@MainActor public func suite(_ name: String) {
    print("\n\(name)")
}
