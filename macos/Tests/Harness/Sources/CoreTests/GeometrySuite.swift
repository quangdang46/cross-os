import CoreGraphics
import Foundation

@testable import CrossOSCore

// The tile grid, against the six groups its TypeScript predecessor's tests
// covered.
//
// `switcher.test.tsx` mirrored `alt-tab-macos`'s
// `TileGridLayoutTests.swift` one for one — group A (one row), B (wrapping),
// C (right-to-left), D (tile size), E (centring), F (the auto size) — and those
// groups are the contract. The arithmetic came to this project as a reverse
// port with its reasons written down, and porting it back is only correct if
// the same cases still hold. They are here because a grid that is 2px wrong
// looks fine until somebody has eleven windows instead of six.

@MainActor func runGeometrySuite() {
    func grid(
        _ widths: [CGFloat],
        height: CGFloat = 100,
        padding: CGFloat = 8,
        max: CGFloat = 720,
        ltr: Bool = true
    ) -> TileGrid {
        tileGrid(TileGridInput(
            widths: widths, tileHeight: height,
            padding: padding, widthMax: max, isLeftToRight: ltr
        ))
    }

    // A: one row.
    do {
        let g = grid([300, 300, 300])
        expectEqual(g.origins.count, 3, "A: three tiles placed")
        // 8+308 = 316 and +308 = 624, both inside 720: two tiles, one row.
        let two = grid([300, 300])
        expectEqual(two.rows.count, 1, "A: two 300s are one row in 720")
        expectEqual(two.rows[0], [0, 1], "A: both are in it")
        expectEqual(two.maxX, 624, "A: and the extent counts the trailing padding")

        // 8 + 300 + 8, then 300 + 8 — the padding that FOLLOWS the last tile is
        // part of what the panel is sized from.
        expectEqual(g.maxX, 624, "A: maxX counts the trailing padding")
        expectEqual(g.maxY, 224, "A: maxY is the bottom of the LAST row, not the reserved one")
    }

    // A second: no tiles at all still reserves a row.
    do {
        let g = grid([])
        expectEqual(g.origins.count, 0, "A: no tiles, no origins")
        expectEqual(g.rows.count, 1, "A: the panel still holds one row")
        expectEqual(g.maxY, 116, "A: and still has the height of one")
    }

    // B: wrapping.
    do {
        // 8+300+8 fills 316; three of them is 948, past 720. The third wraps.
        let g = grid([300, 300, 300])
        expectEqual(g.rows.count, 2, "B: 3x300 wraps in 720 — the third crosses, not the second")
        let wider = grid([300, 300, 300], max: 640)
        expectEqual(wider.rows.count, 2, "B: 3x300 wraps in 640")
        expectEqual(wider.rows[0], [0, 1], "B: the first row holds two")
        expectEqual(wider.rows[1], [2], "B: the second holds the one that wrapped")
        // A tile that OPENS a row does not widen the grid.
        expectEqual(wider.maxX, 624, "B: maxX is row 1's extent, not a wrap tile's")
    }

    // B second: an edge landing EXACTLY on the boundary does not wrap.
    do {
        // 8 + 300 + 8 = 316; 316 + 300 + 8 = 624; 624 + 84 + 8 = 716 <= 720.
        let exact = grid([300, 300, 84])
        expectEqual(exact.rows.count, 1, "B: a tile that lands exactly on the edge does not wrap")
        let over = grid([300, 300, 96])
        expectEqual(over.rows.count, 2, "B: one pixel past it does")
    }

    // B third: a first tile wider than the whole grid.
    do {
        let g = grid([900], max: 720)
        expectEqual(g.rows.count, 2, "B: an oversized first tile opens a row of its own")
        expectEqual(g.maxX, 0, "B: and leaves maxX at zero")
    }

    // C: right to left.
    do {
        let ltr = grid([300, 300])
        let rtl = grid([300, 300], ltr: false)
        expectEqual(rtl.origins[0].x, 720 - 8 - 300, "C: the first tile's origin is its trailing edge less its width")
        expect(ltr.origins[0].x != rtl.origins[0].x, "C: the two directions differ")
        // maxX is measured from the RIGHT edge in RTL, so it is the same number.
        expectEqual(rtl.maxX, ltr.maxX, "C: the extent is the same either way")
    }

    // D: tile size.
    do {
        for size in TileSize.candidates {
            let g = grid([300, 300], height: size.tileHeight)
            expectEqual(g.maxY, size.tileHeight + 16, "D: \(size.name) reserves its own height")
        }
        // Largest first — the order `auto` walks.
        expectEqual(TileSize.candidates.first?.name, "large", "D: the first candidate is the largest")
        expectEqual(TileSize.candidates.last?.name, "small", "D: the last is the smallest")
    }

    // E: centring. A row is `padding + Σ(tile + padding)`, so two 300s with
    // padding 8 is 8 + 308 + 308 = 624; its slack in 720 is 96 and half of
    // that is 48. One tile is 316, slack 404, half 202.
    let offsets = centeringOffsets(
        rowWidths: [[300, 300], [300]],
        padding: 8,
        within: 720
    )
    expectEqual(offsets[0], 48, "E: a full-ish row moves half its slack")
    // Row two is centred in the PANEL, not under the middle of the tiles —
    // which is why `within` is the caller's width and not the grid's maxX.
    expectEqual(offsets[1], 202, "E: a short row centres under the PANEL, not under the tiles")
    expectEqual(centeringOffsets(rowWidths: [[]], padding: 8, within: 720)[0], 0, "E: an empty row stays put")
    expectEqual(centeringOffsets(rowWidths: [[900]], padding: 8, within: 720)[0], 0, "E: an over-wide row is not pulled back")

    // Rounding is away from zero, and the two halves prove it: 721-624=97
    // gives 48.5 and 719-624=95 gives 47.5. Rounded-to-even would send both to
    // 48 and the row would sit a half pixel off centre depending on which
    // side of the panel it landed on.
    expectEqual(centeringOffsets(rowWidths: [[300, 300]], padding: 8, within: 721)[0], 49, "E: 48.5 rounds to 49, not 48")
    expectEqual(centeringOffsets(rowWidths: [[300, 300]], padding: 8, within: 719)[0], 48, "E: 47.5 rounds to 48")

    // F: the auto size. A large grid is 132 + 16 = 148 tall, so a cap of 140
    // admits the medium and not the large: it is the first candidate THAT FITS,
    // not the first one.
    let chosen = firstSizeThatFits(TileSize.candidates, heightMax: 140) { $0.tileHeight + 16 }
    expectEqual(chosen?.name, "medium", "F: the first candidate that FITS wins, not the first")
    let large = firstSizeThatFits(TileSize.candidates, heightMax: 200) { $0.tileHeight + 16 }
    expectEqual(large?.name, "large", "F: with room for it, the largest is taken")

    // Room for nothing: the LAST candidate, so a panel always has a size and
    // never ends up with none because the window was too short.
    let forced = firstSizeThatFits(TileSize.candidates, heightMax: 10) { $0.tileHeight }
    expectEqual(forced?.name, "small", "F: when none fit, the last candidate is used")

    // Every candidate up to and including the chosen one is measured, IN ORDER.
    // The reference's measure closure applies the size to the appearance before
    // it can measure it, so the count and the order are part of the contract
    // rather than an accident — a version that short-circuited would draw a
    // size it never applied.
    var measured: [String] = []
    _ = firstSizeThatFits(TileSize.candidates, heightMax: 110) { size in
        measured.append(size.name)
        return size.tileHeight
    }
    expectEqual(measured, ["large", "medium"], "F: candidates are measured in order, up to the one chosen")
    // The wrap is measured WITH the padding that follows the tile, and the two
    // cases where that changes the answer are the reason the arithmetic is a port
    // rather than a simplification.
    do {
        // 3 x 215 with no padding is 645, past 640, so it wraps.
        expectEqual(grid([215, 215, 215], padding: 0, max: 640).rows.count, 2, "wrap: 645 > 640 without padding")
        // 3 x 210 with no padding is 630, which fits.
        expectEqual(grid([210, 210, 210], padding: 0, max: 640).rows.count, 1, "wrap: 630 <= 640 without padding")
        // The same three with padding 8 is 654, so it wraps where it did not.
        expectEqual(grid([210, 210, 210], padding: 8, max: 640).rows.count, 2, "wrap: the padding that follows a tile is counted")
    }
}
