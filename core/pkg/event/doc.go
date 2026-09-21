// Package event implements the deterministic synchronous EventRouter behind
// the EventBus-named interface.
//
// Bead: cross-os-z9v. Plan: COMPREHENSIVE_PLAN.md §3.2 + Data Flow.
//
// Decision path (synchronous, bounded, inside the native callback):
// input → normalize → context → rule → action, fully resolved before the
// callback returns. Nothing about the current event is queued first.
//
// Observation path (async, best-effort): the already-decided event is
// enqueued as a bounded record AFTER the callback returns. The slow path
// never feeds back into the same event's decision.
//
// Consume-vs-Replace predicate (locked here, closing the TODO(dispatcher)
// from package rule): a winning rule carries Emit. Emit=true → the winner
// dispatches an Intent through the Capability API → DecisionReplace.
// Emit=false → the winner absorbs the event with no dispatch →
// DecisionConsume. No winner → DecisionPass. Rule authors declare Emit at
// registration; the router never guesses.
package event
