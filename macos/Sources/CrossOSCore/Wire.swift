import Foundation

// The wire.
//
// This is not a design. It is a transcription of a protocol that already exists
// and already has a Go implementation that ships:
//
//   app/backend/ipc_client.go:24-33   the request shape
//   app/backend/ipc_client.go:35-49   the response shape
//   app/backend/ipc_client.go:89      json.Marshal of the request
//   app/backend/ipc_client.go:94      raw = append(raw, '\n')   ← the framing
//   app/backend/ipc_client.go:98-112  read one byte at a time until '\n'
//   app/backend/ipc_client.go:115     the 1 MiB response ceiling
//
// One JSON-RPC object per line, newline-terminated, in both directions. The Go
// reader consumes a byte at a time rather than buffering, which is why the
// ceiling is checked per byte; `readLine` below reproduces that contract
// exactly, including the error when a response exceeds it.
//
// Everything here is internal plumbing. Nothing in this file knows what a
// status is or what a window is — those are in `Types.swift`, and the methods
// that use them are in `CoreClient.swift`.

/// A JSON-RPC 2.0 request. Field names and JSON keys are fixed by the protocol;
/// `id` is a number the daemon echoes back so a response can be matched to its
/// question.
public struct WireRequest: Encodable {
    let jsonrpc = "2.0"
    public let method: String
    public let params: JSONValue?
    public let id: Int

    public init(method: String, params: JSONValue?, id: Int) {
        self.method = method
        self.params = params
        self.id = id
    }
}

/// A JSON-RPC 2.0 response. Exactly one of `result` and `error` is present, and
/// which one is the answer to the call.
public struct WireResponse: Decodable {
    public let result: JSONValue?
    public let error: WireError?
    public let id: Int?
}

public struct WireError: Decodable, Error, CustomStringConvertible {
    public let code: Int
    public let message: String

    public init(code: Int, message: String) {
        self.code = code
        self.message = message
    }

    public var description: String { "code \(code): \(message)" }
}

/// The framing, split out from the transport so it can be tested against a byte
/// buffer without a socket. The Go implementation is
/// `IPCCore.callWithin` (ipc_client.go:66-122).
///
/// Public because the tests live in a separate executable target and framing
/// is the one thing in this port that must be provably identical to the Go
/// client's. That is not a reason to widen the API generally — it is a reason
/// this one type is visible from outside.
public enum Wire {
    /// Go caps a response at 1 MiB and fails the call rather than growing
    /// without bound (ipc_client.go:111-113). The same ceiling, the same
    /// failure, because a client that reads what the daemon will not send is a
    /// client that can be made to exhaust memory.
    public static let maxResponseBytes = 1024 * 1024

    /// Encode one request, newline-terminated, ready for `write`.
    public static func encode(_ request: WireRequest) throws -> Data {
        var data = try JSONEncoder().encode(request)
        data.append(0x0A)
        return data
    }

    /// Pull one complete line out of `buffer`, returning it without the
    /// newline and the number of bytes consumed, or `nil` if the line is not
    /// there yet.
    ///
    /// The `nil`-until-`\n` shape is what makes framing a matter of policy
    /// rather than of transport. The Go client dials a fresh connection per
    /// call and reads a byte at a time; this client keeps one connection and
    /// reads in bulk, so the accumulation has to live somewhere. It lives
    /// here, and it is the only place a partial line can exist.
    public static func takeLine(from buffer: inout Data) throws -> (line: Data, consumed: Int)? {
        guard let newline = buffer.firstIndex(of: 0x0A) else {
            if buffer.count > maxResponseBytes {
                throw WireError(code: -1, message: "response too large")
            }
            return nil
        }
        let line = buffer[buffer.startIndex..<newline]
        let consumed = buffer.distance(from: buffer.startIndex, to: newline) + 1
        buffer.removeSubrange(buffer.startIndex..<newline + 1)
        if line.count > maxResponseBytes {
            throw WireError(code: -1, message: "response too large")
        }
        return (Data(line), consumed)
    }

    /// Decode one response line.
    public static func decode(_ line: Data) throws -> WireResponse {
        try JSONDecoder().decode(WireResponse.self, from: line)
    }
}
