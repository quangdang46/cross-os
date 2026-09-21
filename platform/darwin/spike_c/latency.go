// Per-query latency recorder (bead criterion 3).
//
// These numbers are the written justification for the jiz cache: synchronous
// AX/UIA queries cost orders of magnitude more than the <1ms fast-path
// budget, so keydown-time context must come from cache updated on
// app/window change — never queried per keydown. Same fixed-ring,
// allocation-light shape as spike A's recorder.
package spikec

import "time"

// QueryLatencyCap bounds the ring.
const QueryLatencyCap = 1024

// QueryLatency records synchronous query durations in nanoseconds.
type QueryLatency struct {
	buf  [QueryLatencyCap]int64
	n    int
	head int
}

// Observe records one query duration.
func (l *QueryLatency) Observe(d time.Duration) {
	l.buf[l.head] = int64(d)
	l.head = (l.head + 1) % QueryLatencyCap
	if l.n < QueryLatencyCap {
		l.n++
	}
}

// Samples returns the recorded sample count.
func (l *QueryLatency) Samples() int { return l.n }

func (l *QueryLatency) sorted() []int64 {
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
	return tmp
}

func percentile(sorted []int64, pct int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := (pct*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return time.Duration(sorted[rank-1])
}

// P50 returns the median query latency.
func (l *QueryLatency) P50() time.Duration { return percentile(l.sorted(), 50) }

// P95 returns the 95th percentile query latency.
func (l *QueryLatency) P95() time.Duration { return percentile(l.sorted(), 95) }
