// Callback latency recorder — bead criterion 4: "feasibility verdict is a
// number, not an adjective" (target p95 ~100µs).
//
// Allocation-light by design: fixed ring buffer, no heap per sample, so the
// recorder itself can sit on the fast path without blowing the budget it
// measures. Percentile is computed on demand (copy + insertion sort is fine
// for the spike-scale N; the product path in cross-os-wge will use an HDR
// histogram if it needs streaming p95).
package spikea

import "time"

// LatencyCap bounds the ring; 4096 samples ≈ 68s at 60 keys/s.
const LatencyCap = 4096

// Latency records callback durations in nanoseconds.
type Latency struct {
	buf  [LatencyCap]int64
	n    int
	head int
}

// Observe records one callback duration. Lock-free contract: the tap callback
// is serial (one event at a time per tap), so no mutex — same reasoning as
// spike B's atomic-only hot path, but here even atomics are unnecessary.
func (l *Latency) Observe(d time.Duration) {
	l.buf[l.head] = int64(d)
	l.head = (l.head + 1) % LatencyCap
	if l.n < LatencyCap {
		l.n++
	}
}

// Samples returns the number of recorded samples.
func (l *Latency) Samples() int { return l.n }

// P95 returns the 95th percentile of recorded durations, or 0 with no data.
// Uses nearest-rank on a copied snapshot; does not disturb the ring. The
// copy is wrap-safe: percentile needs the multiset, not insertion order, so
// reading buf[0:n] is correct even after head has wrapped.
func (l *Latency) P95() time.Duration {
	if l.n == 0 {
		return 0
	}
	tmp := make([]int64, l.n)
	for i := 0; i < l.n; i++ {
		tmp[i] = l.buf[i]
	}
	// Insertion sort: fine at spike scale, no dependency.
	for i := 1; i < len(tmp); i++ {
		v := tmp[i]
		j := i - 1
		for j >= 0 && tmp[j] > v {
			tmp[j+1] = tmp[j]
			j--
		}
		tmp[j+1] = v
	}
	rank := (95*l.n + 99) / 100 // ceil(0.95*n), 1-based
	return time.Duration(tmp[rank-1])
}

// P50 returns the median, for the stage log alongside p95.
func (l *Latency) P50() time.Duration {
	if l.n == 0 {
		return 0
	}
	tmp := make([]int64, l.n)
	for i := 0; i < l.n; i++ {
		tmp[i] = l.buf[i]
	}
	for i := 1; i < len(tmp); i++ {
		v := tmp[i]
		j := i - 1
		for j >= 0 && tmp[j] > v {
			tmp[j+1] = tmp[j]
			j--
		}
		tmp[j+1] = v
	}
	rank := (50*l.n + 99) / 100
	return time.Duration(tmp[rank-1])
}
