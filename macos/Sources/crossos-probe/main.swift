import CrossOSCore
import Foundation

// crossos-probe — every RPC the UI can make, from a terminal.
//
// This is how phase 1 of the React-to-AppKit port is judged. The port is done
// when this reproduces the numbers in docs/baseline/ from the daemon alone,
// with no React, no webview and no Wails anywhere in the path:
//
//   crossos-probe status      → docs/baseline/webkit-palette.json's daemon half
//   crossos-probe windows     → the switcher's tile list
//   crossos-probe readiness   → the Home pane's four rows
//   crossos-probe contrast    → nothing; see below
//
// It is an executable target rather than a test because a test that needs a
// running daemon is a test that gets skipped. This is the thing you run to see
// whether the daemon is there.

let socketPath = CommandLine.arguments.dropFirst().first.flatMap { arg in
    arg.hasPrefix("--socket=") ? String(arg.dropFirst("--socket=".count)) : nil
} ?? LiveCoreClient.defaultSocketPath

let client = LiveCoreClient(socketPath: socketPath)
let command = CommandLine.arguments.dropFirst().first { !$0.hasPrefix("--") } ?? "status"

func emit(_ value: some Encodable) {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
    guard let data = try? encoder.encode(value) else {
        print("(could not encode)")
        return
    }
    print(String(decoding: data, as: UTF8.self))
}

/// Pad to a column, so a list of pages reads as a table in a terminal instead
/// of a ragged list.
extension String {
    func padded(_ width: Int) -> String {
        count >= width ? self : self + String(repeating: " ", count: width - count)
    }
}

/// Print the error the way a person needs to read it: the description carries
/// the daemon's own message and the method that failed, and a socket error on
/// a machine where nothing is running should say so rather than showing an
/// `errno` nobody can act on.
func fail(_ error: any Error) -> Never {
    FileHandle.standardError.write(Data("crossos-probe: \(error)\n\n".utf8))
    FileHandle.standardError.write(Data("  socket: \(socketPath)\n".utf8))
    FileHandle.standardError.write(Data("  is the daemon running?  ./scripts/run.sh\n".utf8))
    exit(1)
}

do {
    switch command {
    case "status":
        // ONE `core.status` call for all six fields. The Go bridge made seven
        // round trips for this same answer; if this ever needs a second call,
        // that is a regression with a name.
        emit(try await client.status())

    case "plugins":
        emit(try await client.plugins())

    case "windows":
        emit(try await client.windows())

    case "readiness":
        emit(try await client.readiness())

    case "events":
        emit(try await client.eventLogs())

    case "pages":
        // The list a nav is built from. Summarised rather than dumped: fifteen
        // pages with their whole schemas is a wall of JSON, and what a person
        // checking this wants is the order, the group each sits in, and which
        // page a fresh profile lands on.
        let pages = try await client.pages()
        for page in pages {
            let kinds = (page.schema?.controls ?? []).map(\.kind).joined(separator: ",")
            let flags = [
                page.firstRun ? "firstRun" : nil,
                page.symbol.isEmpty ? nil : "symbol=\(page.symbol)",
            ].compactMap { $0 }.joined(separator: " ")
            print("\(page.id.padded(22)) group=\(page.group.padded(11)) order=\(page.order) \(flags)")
            print("  controls: \(kinds.isEmpty ? "(none)" : kinds)")
        }
        print("\n\(pages.count) pages")

    case "wait":
        // The long-poll, with the budget it will really respect. `triggered:
        // false` is a VALUE, so this prints it and exits 0 — a poll that
        // expires is not a failure, and a probe that exited non-zero would
        // make the common case look broken.
        let budget = CommandLine.arguments.dropFirst().first { $0.hasPrefix("--timeout=") }
            .flatMap { Int($0.dropFirst("--timeout=".count)) } ?? 1000
        emit(try await client.switcherWait(timeoutMS: budget))

    case "help", "--help", "-h":
        print("""
        usage: crossos-probe [--socket=PATH] <command>

          status       one core.status — the whole answer in one round trip
          plugins      plugin.list
          windows      core.windows — the switcher's tile list
          readiness    core.readiness — the Home pane's rows
          events       core.eventLogs
          wait         core.switcherWait (--timeout=N, default 1000)
                       NOTE: the daemon's own 30s cap governs this call, not N
          help         this text
        """)

    default:
        FileHandle.standardError.write(Data("crossos-probe: unknown command \"\(command)\" — try `help`\n".utf8))
        exit(2)
    }
} catch {
    fail(error)
}
