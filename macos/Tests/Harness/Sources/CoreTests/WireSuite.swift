import CrossOSCore
import Foundation

// The wire, tested without a socket.
//
// Framing is the one thing in this port that is a TRANSCRIPTION rather than a
// design, which makes it exactly the thing that can be subtly wrong in a way
// that still compiles. The Go implementation is ipc_client.go:89-122; these
// tests are the same cases it already handles, written against the Swift that
// has to handle them identically.

@MainActor func runWireSuite() throws {
    suite("The wire — encoding")

    let encoded = try Wire.encode(WireRequest(method: "core.status", params: nil, id: 1))
    let encodedText = String(decoding: encoded, as: UTF8.self)

    expect(encodedText.hasSuffix("\n"), "a request ends with a newline")
    expect(encodedText.filter { $0 == "\n" }.count == 1, "exactly one newline — no pretty-printing")

    if let object = (try? JSONSerialization.jsonObject(with: encoded)) as? [String: Any] {
        expectEqual(object["jsonrpc"] as? String, "2.0", "the protocol version is 2.0")
        expectEqual(object["method"] as? String, "core.status", "the method name survives")
        expectEqual(object["id"] as? Int, 1, "the id survives")
        // A method with no params omits the key rather than sending null:
        // `omitempty` on the Go side (ipc_client.go:27).
        expect(object["params"] == nil, "a no-arg method omits params, not null")
    } else {
        expect(false, "a request decodes as a JSON object")
    }

    suite("The wire — framing")

    do {
        var buffer = Data("{\"jsonrpc\":\"2.0\"".utf8)
        expect({ try Wire.takeLine(from: &buffer) == nil }, "a partial line is not a line")
        expect(!buffer.isEmpty, "an incomplete line stays buffered, not dropped")

        buffer.append(contentsOf: Array("}".utf8))
        expect({ try Wire.takeLine(from: &buffer) == nil }, "still no newline, still no line")

        buffer.append(0x0A)
        guard let taken = try Wire.takeLine(from: &buffer) else {
            expect(false, "the newline completes the line")
            report()
        }
        expectEqual(
            String(decoding: taken.line, as: UTF8.self),
            "{\"jsonrpc\":\"2.0\"}",
            "the line is the bytes before the newline"
        )
        expect(taken.consumed == 18, "consumed counts the newline too")
        expect(buffer.isEmpty, "the consumed line leaves the buffer")
    }

    do {
        // Two answers arriving in one read is the normal case for a poll, and the
        // reason framing has to be a policy rather than a transport accident.
        var buffer = Data("{\"id\":1}\n{\"id\":2}\n".utf8)
        let first = try Wire.takeLine(from: &buffer)
        let second = try Wire.takeLine(from: &buffer)
        expectEqual(first?.line, Data("{\"id\":1}".utf8), "line one")
        expectEqual(second?.line, Data("{\"id\":2}".utf8), "line two")
        expect({ try Wire.takeLine(from: &buffer) == nil }, "the buffer is empty after two lines")
    }

    suite("The wire — the 1 MiB ceiling")

    do {
        // The Go client checks the ceiling as it reads and fails the call
        // (ipc_client.go:111-113). A client that reads what the daemon will not
        // send is a client that can be made to exhaust memory.
        var unterminated = Data(repeating: 0x41, count: Wire.maxResponseBytes + 1)
        expectThrows("an unterminated line past the ceiling throws") {
            _ = try Wire.takeLine(from: &unterminated)
        }

        var terminated = Data(repeating: 0x41, count: Wire.maxResponseBytes + 1)
        terminated.append(0x0A)
        expectThrows("a terminated line past the ceiling throws too") {
            _ = try Wire.takeLine(from: &terminated)
        }
    }

    do {
        var buffer = Data(repeating: 0x41, count: Wire.maxResponseBytes)
        buffer.append(0x0A)
        let taken = try Wire.takeLine(from: &buffer)
        expectEqual(taken?.line.count, Wire.maxResponseBytes, "a line exactly at the ceiling is not truncated")
    }

    suite("The wire — responses")

    do {
        // An rpc error carries the daemon's own message. `core.windows` on a
        // machine without Accessibility consent answers exactly this, and the
        // message is the only thing that tells the user what to grant — a client
        // that swallowed it showed a blank page, which is what the React shell did
        // for a week.
        let line = Data(#"{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"adapter: accessibility permission denied"}}"#.utf8)
        let response = try Wire.decode(line)
        expect(response.result == nil, "an error answer carries no result")
        expectEqual(response.error?.code, -32603, "the code survives")
        expectEqual(
            response.error?.message, "adapter: accessibility permission denied",
            "the daemon's message survives, verbatim"
        )
    }

    do {
        // `void` methods answer null. Treating that as absent would make every
        // no-argument mutation look like it had failed.
        //
        // The check is `== nil` and not `== .null`, which is a distinction worth
        // keeping: Swift represents `Optional.some(.null)` as nil, so a decoded
        // JSON null and an absent key are indistinguishable here. That is fine for
        // this protocol — the daemon always sends `result` and never `error`
        // together, and `CoreClient.call` turns the nil into `.null` explicitly —
        // but it is why the assertion is written the way it is rather than the way
        // it looks like it should be.
        let response = try Wire.decode(Data(#"{"jsonrpc":"2.0","id":1,"result":null}"#.utf8))
        expect(response.error == nil, "a null result is not an error")
        expect(response.id == 1, "and the id still comes back, which is what matches it to its question")
    }

    suite("JSON values")

    do {
        // JSON `true` and `1` are both scalars. Without checking Bool first, a
        // Swift decoder hands back 1.0 for `true` — and `triggered` is exactly a
        // bool that must not become 1.
        let value = try JSONDecoder().decode(JSONValue.self, from: Data("true".utf8))
        expect(value == .bool(true), "true decodes as a bool")
        expectEqual(value.boolValue, true, "and reads back as one")
        expect(value.intValue == nil, "a bool is not an int")
    }

    do {
        let value = try JSONDecoder().decode(JSONValue.self, from: Data("0.5".utf8))
        expectEqual(value.doubleValue, 0.5, "a number keeps its fraction")
    }

    do {
        // wire.ts:15-90 returns "" and 0 for a mismatched shape, so a renamed field
        // shows up as an empty box. A nil is a decision the caller makes instead.
        let text = try JSONDecoder().decode(JSONValue.self, from: Data(#""hi""#.utf8))
        expectEqual(text.stringValue, "hi", "a string reads back")
        expect(text.intValue == nil, "and does not invent an int")
        expect(text.boolValue == nil, "and does not invent a bool")
    }
}
