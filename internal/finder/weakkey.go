package finder

import (
	"fmt"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

// WeakKeyFinder finds candidate transactions using a composite weak key:
//
//	amount_minor_units + currency + minute_bucket
//
// This is PASS 2 — used when a transaction has no ReferenceID, or when
// ReferenceFinder returned no candidates.
//
// The index uses minute-level time truncation intentionally. Exact timestamp
// matching belongs to PASS 1. The weak key is a tolerance window — transactions
// within the same minute are candidates for each other.
//
// To handle transactions that straddle a minute boundary (e.g. 13:30:59 vs
// 13:31:01 due to clock drift or timezone conversion rounding), FindCandidates
// queries three neighboring buckets: the transaction's own minute, the minute
// before, and the minute after. All candidates are merged and deduplicated
// before being returned.
//
// Important: a weak key match is a candidate, not a confirmation.
// The engine validates candidates and handles ambiguity — multiple transactions
// with the same amount, currency, and minute are common in high-volume systems
// (salary runs, bulk payouts, flash sales). The engine will classify these as
// AmbiguousMatch, not silently pick one.
type WeakKeyFinder struct {
	// idx maps weak key → all transactions in the opposing pool with that key.
	idx map[string][]*domain.Transaction
}

// NewWeakKeyFinder builds a WeakKeyFinder seeded with the opposing source pool.
//
// Call this once per run after LoadCSV, with the transactions from the side
// you want to search against. Typically the external/provider pool.
//
// Every transaction is indexed under its own minute bucket only.
// Neighboring bucket lookups happen at query time in FindCandidates.
func NewWeakKeyFinder(pool []*domain.Transaction) *WeakKeyFinder {
	idx := make(map[string][]*domain.Transaction)

	for _, tx := range pool {
		key := weakKey(tx.AmountMinorUnits, tx.Currency, tx.Timestamp)
		idx[key] = append(idx[key], tx)
	}

	return &WeakKeyFinder{idx: idx}
}

// FindCandidates returns all transactions in the opposing pool that share
// the same amount, currency, and are within ±1 minute of the query transaction.
//
// Queries three buckets:
//  1. tx.Timestamp truncated to minute        (own bucket)
//  2. tx.Timestamp truncated to minute - 1m   (previous bucket)
//  3. tx.Timestamp truncated to minute + 1m   (next bucket)
//
// Results from all three buckets are merged and deduplicated by pointer address
// before being returned. The engine receives a flat, deduplicated candidate list.
//
// Returns nil when no candidates exist in any of the three buckets.
func (w *WeakKeyFinder) FindCandidates(tx *domain.Transaction) []*domain.Transaction {
	minute := tx.Timestamp.UTC().Truncate(time.Minute)

	buckets := []time.Time{
		minute.Add(-time.Minute), // previous
		minute,                   // own
		minute.Add(time.Minute),  // next
	}

	// seen deduplicates by pointer — same *Transaction appearing in multiple
	// buckets (shouldn't happen but defensive) is only returned once.
	seen := make(map[*domain.Transaction]struct{})
	var candidates []*domain.Transaction

	for _, bucket := range buckets {
		key := weakKey(tx.AmountMinorUnits, tx.Currency, bucket)
		for _, candidate := range w.idx[key] {
			if _, exists := seen[candidate]; !exists {
				seen[candidate] = struct{}{}
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates
}

// Len returns the number of distinct weak keys in the index.
// Useful for diagnostics — a high number relative to transaction count
// suggests low collision rate (good). A low number suggests many transactions
// share the same amount+currency+minute (ambiguity risk, engine will flag these).
func (w *WeakKeyFinder) Len() int {
	return len(w.idx)
}

// weakKey produces a canonical string key for a given amount, currency, and time.
// Time is always truncated to the minute before formatting.
// All times are converted to UTC before truncation.
//
// Format: "{amount_minor_units}:{currency}:{YYYY-MM-DDTHH:MM}Z"
// Example: "500000:NGN:2026-06-21T13:30Z"
func weakKey(amountMinorUnits int64, currency string, t time.Time) string {
	bucket := t.UTC().Truncate(time.Minute)
	return fmt.Sprintf("%d:%s:%sZ",
		amountMinorUnits,
		currency,
		bucket.Format("2006-01-02T15:04"),
	)
}
