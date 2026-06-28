package finder

import (
	"fmt"
	"math"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

// AmountWindowFinder finds candidate transactions using a fee-aware amount range.
//
// This is PASS 3 — used when both ReferenceFinder and WeakKeyFinder returned
// no candidates. It is designed for transactions where:
//   - the reference ID is missing on one or both sides
//   - the amounts differ due to provider fees, FX spreads, or rounding
//
// Strategy:
//  1. Index external transactions by currency + minute bucket
//  2. At query time, retrieve all candidates in the same minute bucket
//     (plus neighboring buckets for boundary safety)
//  3. Filter candidates whose amount falls within ±fee_tolerance_percent
//     of the query transaction's amount
//  4. Further filter candidates whose timestamp falls within ±time_window_seconds
//
// This is deliberately the weakest deterministic finder. Matches here should
// always be returned with lower confidence and scrutinised by the engine.
// A high volume of PASS 3 matches in a report is a signal that your provider
// is not returning stable reference IDs — worth investigating.
type AmountWindowFinder struct {
	// idx maps "currency:minute_bucket" → all transactions in that bucket.
	// Indexed by currency + time only — amount filtering happens at query time.
	idx map[string][]*domain.Transaction

	// feeTolerancePercent is the maximum allowed percentage difference
	// between internal and external amounts for a candidate to qualify.
	// Example: 1.5 means amounts within 1.5% of each other are candidates.
	feeTolerancePercent float64

	// timeWindowSeconds is the maximum allowed time difference in seconds
	// between internal and external timestamps for a candidate to qualify.
	timeWindowSeconds int64
}

// NewAmountWindowFinder builds an AmountWindowFinder seeded with the opposing pool.
//
// feeTolerancePercent and timeWindowSeconds come from the run config:
//
//	run:
//	  fee_tolerance_percent: 1.5
//	  time_window_seconds: 86400
//
// Transactions are indexed by currency + minute bucket only.
// Amount filtering and time window validation happen at query time.
func NewAmountWindowFinder(pool []*domain.Transaction, feeTolerancePercent float64, timeWindowSeconds int64) *AmountWindowFinder {
	idx := make(map[string][]*domain.Transaction)

	for _, tx := range pool {
		key := amountWindowKey(tx.Currency, tx.Timestamp)
		idx[key] = append(idx[key], tx)
	}

	return &AmountWindowFinder{
		idx:                 idx,
		feeTolerancePercent: feeTolerancePercent,
		timeWindowSeconds:   timeWindowSeconds,
	}
}

// FindCandidates returns all transactions in the opposing pool that:
//  1. Share the same currency
//  2. Fall within the same minute bucket (±1 neighboring buckets)
//  3. Have an amount within ±feeTolerancePercent of the query amount
//  4. Have a timestamp within ±timeWindowSeconds of the query timestamp
//
// Returns nil when no candidates survive all four filters.
func (a *AmountWindowFinder) FindCandidates(tx *domain.Transaction) []*domain.Transaction {
	minute := tx.Timestamp.UTC().Truncate(time.Minute)

	// Query three neighboring minute buckets — same boundary safety as WeakKeyFinder.
	buckets := []time.Time{
		minute.Add(-time.Minute),
		minute,
		minute.Add(time.Minute),
	}

	seen := make(map[*domain.Transaction]struct{})
	var candidates []*domain.Transaction

	for _, bucket := range buckets {
		key := amountWindowKey(tx.Currency, bucket)
		for _, candidate := range a.idx[key] {
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}

			// Filter 1: amount within fee tolerance.
			if !withinFeeTolerance(tx.AmountMinorUnits, candidate.AmountMinorUnits, a.feeTolerancePercent) {
				continue
			}

			// Filter 2: timestamp within time window.
			if !withinTimeWindow(tx.Timestamp, candidate.Timestamp, a.timeWindowSeconds) {
				continue
			}

			candidates = append(candidates, candidate)
		}
	}

	return candidates
}

// Len returns the number of distinct currency+minute keys in the index.
func (a *AmountWindowFinder) Len() int {
	return len(a.idx)
}

// amountWindowKey produces a canonical index key for a currency and timestamp.
// Keyed by currency + minute bucket only — amount is not part of the key.
//
// Format: "{currency}:{YYYY-MM-DDTHH:MM}Z"
// Example: "NGN:2026-06-21T09:30Z"
func amountWindowKey(currency string, t time.Time) string {
	bucket := t.UTC().Truncate(time.Minute)
	return fmt.Sprintf("%s:%sZ", currency, bucket.Format("2006-01-02T15:04"))
}

// withinFeeTolerance returns true if candidate amount is within ±tolerancePercent
// of the query amount.
//
// Example: query=500000, candidate=492500, tolerance=1.5%
//
//	diff = |500000 - 492500| = 7500
//	pct  = 7500 / 500000 * 100 = 1.5%
//	1.5% <= 1.5% → true
func withinFeeTolerance(queryAmount, candidateAmount int64, tolerancePercent float64) bool {
	if tolerancePercent <= 0 {
		// Fee tolerance disabled — only exact amounts qualify.
		return queryAmount == candidateAmount
	}

	diff := math.Abs(float64(queryAmount - candidateAmount))
	pct := diff / float64(queryAmount) * 100

	return pct <= tolerancePercent
}

// withinTimeWindow returns true if the absolute difference between two timestamps
// is within the given number of seconds.
func withinTimeWindow(a, b time.Time, windowSeconds int64) bool {
	diff := a.UTC().Sub(b.UTC())
	if diff < 0 {
		diff = -diff
	}
	return int64(diff.Seconds()) <= windowSeconds
}
