import CoreGraphics
import Foundation

// The switcher's tile grid.
//
// **This is a port that goes home.** The arithmetic arrived in this project as
// TypeScript — a reverse port of `alt-tab-macos`'
// `TileGridLayoutSpecs.md:5-49` and its `TileGridLayoutTests.swift` — and the
// two functions at the top of the TypeScript said so in their own doc comments:
// `floorTo` was "Swift's .rounded(.down): a projection is floored so
// accumulated rounding can never push a row that fits onto the next one", and
// `rounded` was "Swift's .rounded(): half away from zero, so 22.5 is 23 and
// -22.5 is -23".
//
// So the comments that explain the subtle cases — a tile that opens a row not
// widening the grid, maxY counting one row before the panel knows what is in
// it, an edge landing exactly on the boundary not wrapping — were written to
// explain why a hand-written `floor` in another language was needed. In Swift
// they are properties of the type, and the functions are two lines long.
//
// What the TS version had that this does not: `Math.floor` on a number that
// came from a DOM, where it might be a half-pixel. Here the inputs are
// `CGFloat` measured from real `NSView` frames, and the arithmetic is the
// same arithmetic on the same type the reference used, which is why the
// `.rounded(.down)` and `.rounded()` comments referred to the language at all.
//
// **What transfers is the ARITHMETIC, not the drawing.** The reference fills an
// `NSView` inside a flipped document view; this is a pure function over widths,
// because the switcher is not the reference and copying its view hierarchy
// would be copying a thing that works for a different window.

/// Where a tile goes, in the flipped document space the panel draws in.
public struct TileOrigin: Equatable {
    public var x: CGFloat
    public var y: CGFloat
    public init(x: CGFloat, y: CGFloat) {
        self.x = x
        self.y = y
    }
}

public struct TileGridInput: Equatable {
    /// One width per tile, in the order the daemon sent them.
    public var widths: [CGFloat]
    public var tileHeight: CGFloat
    /// The gap the wrap is measured WITH: a tile that would cross the edge
    /// counting the padding that follows it is the tile that wraps.
    public var padding: CGFloat
    /// The width the grid wraps at.
    public var widthMax: CGFloat
    public var isLeftToRight: Bool

    public init(
        widths: [CGFloat],
        tileHeight: CGFloat,
        padding: CGFloat,
        widthMax: CGFloat,
        isLeftToRight: Bool = true
    ) {
        self.widths = widths
        self.tileHeight = tileHeight
        self.padding = padding
        self.widthMax = widthMax
        self.isLeftToRight = isLeftToRight
    }
}

public struct TileGrid: Equatable {
    public var origins: [TileOrigin]
    /// Input positions grouped by row. Always holds at least one row.
    public var rows: [[Int]]
    /// What the panel is sized from: the far edge of the widest run of tiles
    /// (including the padding that follows the last) and the bottom of the
    /// last row.
    public var maxX: CGFloat
    public var maxY: CGFloat
}

/// Places the tiles, as the reference does.
///
/// The two subtleties it keeps are the ones that break a grid when they are
/// simplified, and both were spelled out in the TypeScript that preceded it:
///
/// - **A tile that opens a row does NOT widen the grid.** Row 1 always fills to
///   within one tile of `widthMax` before it wraps, so no later row can be
///   wider than row 1 already recorded — and the one exception, a first tile
///   wider than the whole grid, is left at `maxX` 0.
/// - **`maxY` counts one row even with no tiles at all**, because the panel
///   reserves a row before it knows what is in it.
///
/// An edge that lands EXACTLY on the boundary does not wrap, which is what
/// `needsNewRow` is strict-inequality about.
public func tileGrid(_ input: TileGridInput) -> TileGrid {
    let startingX = input.isLeftToRight ? input.padding : input.widthMax - input.padding
    var currentX = startingX
    var currentY = input.padding
    var maxX: CGFloat = 0
    // The reserved row, before anything is in it.
    var maxY = currentY + input.tileHeight + input.padding
    var origins: [TileOrigin] = []
    var rows: [[Int]] = [[]]

    /// In LTR `currentX` is the tile's leading edge; in RTL it is the
    /// trailing one.
    func projectedX(_ currentX: CGFloat, _ width: CGFloat) -> CGFloat {
        input.isLeftToRight
            ? currentX + width + input.padding
            : currentX - width - input.padding
    }

    func needsNewRow(_ projected: CGFloat) -> Bool {
        input.isLeftToRight ? projected > input.widthMax : projected < 0
    }

    /// The frame's origin. In RTL `currentX` is the trailing edge, so the
    /// origin is one tile width before it.
    func originX(_ currentX: CGFloat, _ width: CGFloat) -> CGFloat {
        input.isLeftToRight ? currentX : currentX - width
    }

    for (position, width) in input.widths.enumerated() {
        let nextX = projectedX(currentX, width).rounded(.down)
        if needsNewRow(nextX) {
            currentX = startingX
            currentY = (currentY + input.tileHeight + input.padding).rounded(.down)
            origins.append(TileOrigin(x: originX(currentX, width), y: currentY))
            currentX = projectedX(currentX, width).rounded(.down)
            maxY = max(currentY + input.tileHeight + input.padding, maxY)
            rows.append([])
        } else {
            origins.append(TileOrigin(x: originX(currentX, width), y: currentY))
            currentX = nextX
            maxX = max(input.isLeftToRight ? currentX : input.widthMax - currentX, maxX)
        }
        rows[rows.count - 1].append(position)
    }
    return TileGrid(origins: origins, rows: rows, maxX: maxX, maxY: maxY)
}

/// How far each row has to move along the writing direction to sit centred in
/// `within`.
///
/// Zero for an empty row and for a row already at least that wide, so a full
/// row is never pulled backwards off the edge.
///
/// `within` is the width the CALLER chooses — the panel's own width, not the
/// grid's `maxX` — which is why the last row of a short list sits under the
/// middle of the panel rather than under the middle of the tiles.
///
/// The TS version wrote this as `rounded((within - rowWidth) / 2)`, hand-rolled
/// because JavaScript has no `.rounded()`. Here it is the type's method, and
/// "rounds away from zero, and is never negative" is what `max(0, ·)` plus
/// `rounded(.toNearestOrAwayFromZero)` already say.
public func centeringOffsets(
    rowWidths: [[CGFloat]],
    padding: CGFloat,
    within: CGFloat
) -> [CGFloat] {
    rowWidths.map { widths in
        guard !widths.isEmpty else { return 0 }
        let rowWidth = padding + widths.reduce(0) { $0 + $1 + padding }
        return max(0, ((within - rowWidth) / 2).rounded(.toNearestOrAwayFromZero))
    }
}

/// The auto size: the first candidate whose grid fits in `heightMax`, and the
/// LAST candidate when none do.
///
/// `measure` is called once for every candidate up to and including the chosen
/// one, **in order** — the reference's measure closure applies the size to the
/// appearance before it can measure it, so the call order and count are part of
/// the contract rather than an accident.
public func firstSizeThatFits<Size>(
    _ candidates: [Size],
    heightMax: CGFloat,
    measure: (Size) -> CGFloat
) -> Size? {
    for (position, candidate) in candidates.enumerated() {
        if measure(candidate) <= heightMax || position == candidates.count - 1 {
            return candidate
        }
    }
    return nil
}

/// The tile sizes, largest first: the order `auto` walks, and the one measured
/// first is the one a person sees when the space is generous.
public struct TileSize: Equatable, Sendable {
    public var name: String
    public var tileHeight: CGFloat

    public static let candidates: [TileSize] = [
        TileSize(name: "large", tileHeight: 132),
        TileSize(name: "medium", tileHeight: 104),
        TileSize(name: "small", tileHeight: 76),
    ]
}

public enum SwitcherGeometry {
    /// The panel's own max width. Both the grid's wrap edge and the space a
    /// short row is centred within.
    public static let panelMaxWidth: CGFloat = 720

    /// The gap between tiles.
    public static let tilePadding: CGFloat = 8
}
