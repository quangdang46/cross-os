import Foundation

/// The audit's report.
///
/// **The exit code is the result**: 1 when anything is a `FAIL`, 0 otherwise.
/// A `note` does not fail the run, because a note is a thing worth reading
/// and not a thing that is broken — the design system passing at 4.6:1 and a
/// page with forty rows are both notes, and a gate that fires on either is a
/// gate that gets switched off.
enum AuditPrinter {
    /// One page's findings, printed as they are measured.
    /// Set by `--verbose`.
    ///
    /// Every finding carries a `detail` explaining WHY the rule exists and
    /// what the measurement means. It was never printed, which means all of
    /// that was written for nobody — and, worse, it meant the two findings
    /// most worth reading (the ones about this codebase's own history) were
    /// the two a reader could not get at.
    nonisolated(unsafe) static var verbose = false

    static func emitPage(_ page: String, findings: [Audit.Finding]) {
        guard !findings.isEmpty else {
            print("  \(page) — clean")
            return
        }
        for finding in findings {
            print("  [\(finding.severity)] \(finding.page) — \(finding.what)")
            guard verbose, !finding.detail.isEmpty else { continue }
            print("        \(finding.detail)")
        }
    }

    /// The summary, counted from the same array the per-page lines came from.
    static func emitSummary(_ findings: [Audit.Finding], pages: Int) {
        let fails = findings.filter { $0.severity == .fail }
        let notes = findings.filter { $0.severity == .note }
        let blind = findings.filter { $0.severity == .blind }
        print("")
        print("DESIGN AUDIT — \(pages) pages, \(fails.count) fail, \(blind.count) unmeasurable, \(notes.count) note")
        if verbose {
            for finding in findings where finding.severity == .note {
                print("  [note] \(finding.page) — \(finding.what)")
                if !finding.detail.isEmpty { print("        \(finding.detail)") }
            }
        }
    }

}
