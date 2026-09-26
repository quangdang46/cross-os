import Foundation

/// A JSON value, for the parts of the protocol that are genuinely untyped.
///
/// The daemon speaks JSON-RPC over a socket, and its `result` is whatever the
/// method returns. For most methods that is a known shape and gets a real
/// `Codable` struct in `Types.swift`. For a handful it is not, and this is the
/// type those get.
///
/// Why not `[String: Any]`: the React side handles those seven methods with
/// narrowing helpers that return defaults on a mismatch
/// (`app/frontend/src/lib/wire.ts:15-90` — `asText`, `asNumber`, `asList`,
/// `asRecord`, `asBool`). Each of those is a runtime check that could have been
/// a type. Here the shape is declared once and a decode failure is an error
/// rather than a silently empty string.
public indirect enum JSONValue: Codable, Sendable, Equatable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: any Decoder) throws {
        let container = try decoder.singleValueContainer()
        // Order matters: `Bool` must be tried before `Double`, because JSON
        // `true` and `1` are both "a scalar" to a decoder and Swift will happily
        // decode `true` as 1.0 otherwise.
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? container.decode(Double.self) {
            self = .number(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else if let value = try? container.decode([String: JSONValue].self) {
            self = .object(value)
        } else {
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "a JSON value that is none of null/bool/number/string/array/object"
            )
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .null: try container.encodeNil()
        case .bool(let value): try container.encode(value)
        case .number(let value): try container.encode(value)
        case .string(let value): try container.encode(value)
        case .array(let value): try container.encode(value)
        case .object(let value): try container.encode(value)
        }
    }
}

extension JSONValue {
    /// The four accessors the untyped methods need, and the one the React side
    /// got for free from JavaScript's dynamic typing.
    ///
    /// Each returns nil rather than a default. The difference is the whole
    /// point: `asText` in wire.ts returns `""` for a value that is not a string,
    /// so a daemon that renames a field or changes its type shows up as an
    /// empty box rather than as an error. Here the caller decides what a missing
    /// value means, which for a status field and for a window title are
    /// different answers.
    public var stringValue: String? {
        if case .string(let value) = self { return value }
        return nil
    }

    public var boolValue: Bool? {
        if case .bool(let value) = self { return value }
        return nil
    }

    public var intValue: Int? {
        if case .number(let value) = self { return Int(value) }
        return nil
    }

    public var doubleValue: Double? {
        if case .number(let value) = self { return value }
        return nil
    }

    public var arrayValue: [JSONValue]? {
        if case .array(let value) = self { return value }
        return nil
    }

    public var objectValue: [String: JSONValue]? {
        if case .object(let value) = self { return value }
        return nil
    }

    /// Whether this is JSON `null`, which the daemon uses to mean "no such
    /// value" in several places — and which Go's `*string` and a Swift `String`
    /// disagree about.
    public var isNull: Bool {
        if case .null = self { return true }
        return false
    }
}
