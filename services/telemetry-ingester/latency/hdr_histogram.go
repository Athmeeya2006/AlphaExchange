// Package latency provides O(1)-record latency percentile estimation.
package latency

import "math"

const maxBucket = 10_000_000 // cap at 10s in microseconds

// HDRHistogram is a simplified high-dynamic-range histogram over [0, 10s] in
// 1µs buckets. Record is O(1); Percentile is O(buckets).
type HDRHistogram struct {
	buckets     []int64
	totalCount  int64
	totalSum    int64
	maxObserved int64
	minObserved int64
}

// NewHDRHistogram constructs an empty histogram.
func NewHDRHistogram() *HDRHistogram {
	return &HDRHistogram{
		buckets:     make([]int64, maxBucket),
		minObserved: math.MaxInt64,
	}
}

// RecordValue records a latency in microseconds.
func (h *HDRHistogram) RecordValue(latencyUs int64) {
	if latencyUs < 0 {
		latencyUs = 0
	}
	if latencyUs >= maxBucket {
		latencyUs = maxBucket - 1
	}
	h.buckets[latencyUs]++
	h.totalCount++
	h.totalSum += latencyUs
	if latencyUs > h.maxObserved {
		h.maxObserved = latencyUs
	}
	if latencyUs < h.minObserved {
		h.minObserved = latencyUs
	}
}

// Percentile returns the latency at the given percentile (e.g. 0.99).
func (h *HDRHistogram) Percentile(pct float64) int64 {
	if h.totalCount == 0 {
		return 0
	}
	target := int64(float64(h.totalCount) * pct)
	if target < 1 {
		target = 1
	}
	var cumulative int64
	for i, c := range h.buckets {
		cumulative += c
		if cumulative >= target {
			return int64(i)
		}
	}
	return h.maxObserved
}

// Mean returns the average latency.
func (h *HDRHistogram) Mean() float64 {
	if h.totalCount == 0 {
		return 0
	}
	return float64(h.totalSum) / float64(h.totalCount)
}

// Count returns the number of recorded values.
func (h *HDRHistogram) Count() int64 { return h.totalCount }

// Reset zeroes the histogram for reuse.
func (h *HDRHistogram) Reset() {
	for i := range h.buckets {
		h.buckets[i] = 0
	}
	h.totalCount = 0
	h.totalSum = 0
	h.maxObserved = 0
	h.minObserved = math.MaxInt64
}

// MergeFrom adds another histogram's counts into this one. Not concurrency-safe;
// the caller must hold any necessary lock.
func (h *HDRHistogram) MergeFrom(other *HDRHistogram) {
	for i, c := range other.buckets {
		h.buckets[i] += c
	}
	h.totalCount += other.totalCount
	h.totalSum += other.totalSum
	if other.maxObserved > h.maxObserved {
		h.maxObserved = other.maxObserved
	}
}
