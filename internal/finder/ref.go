package finder

import "github.com/babafemi99/Mogaji/internal/domain"

// ReferenceFinder finds candidate transactions by matching ReferenceID.
//
// This is PASS 1 — the highest confidence lookup strategy. When a transaction
// has a non-empty ReferenceID, this finder performs an O(1) map lookup against
// the opposing source pool.
//
// A ReferenceID may map to multiple candidates if the opposing source contains
// duplicate reference IDs that survived ingest (e.g. a provider that reuses
// reference IDs across different transactions). The engine handles disambiguation.
//
// Transactions with an empty ReferenceID are never indexed and never returned
// as candidates. The engine must fall through to the next Rule for those.
type ReferenceFinder struct {
	// idx maps ReferenceID → all transactions in the opposing pool with that ID.
	// Slice because duplicate reference IDs in the wild are real and must be
	// surfaced as ambiguous rather than silently dropped.
	idx map[string][]*domain.Transaction
}

// NewReferenceFinder builds a ReferenceFinder seeded with the opposing source pool.
//
// Call this once per run with the transactions from the side you want to search.
// If reconciling internal against external, seed with the external transactions.
//
// Transactions with empty ReferenceID are silently excluded from the index —
// they cannot be found by reference and will be handled by weaker finders.
func NewReferenceFinder(pool []*domain.Transaction) *ReferenceFinder {
	idx := make(map[string][]*domain.Transaction)

	for _, tx := range pool {
		if tx.ReferenceID == "" {
			continue // never index blank references — map[""] is a disaster
		}
		idx[tx.ReferenceID] = append(idx[tx.ReferenceID], tx)
	}

	return &ReferenceFinder{idx: idx}
}

// FindCandidates returns all transactions in the opposing pool whose
// ReferenceID exactly matches the query transaction's ReferenceID.
//
// Returns nil when:
//   - the query transaction has an empty ReferenceID
//   - no match exists in the index
//
// Returns multiple candidates when the opposing pool contains more than one
// transaction with the same ReferenceID — the engine classifies this as
// DuplicateExternal or DuplicateInternal depending on which side is which.
func (r *ReferenceFinder) FindCandidates(tx *domain.Transaction) []*domain.Transaction {
	if tx.ReferenceID == "" {
		return nil
	}
	return r.idx[tx.ReferenceID]
}

// Len returns the number of distinct ReferenceIDs in the index.
// Useful for diagnostics and logging.
func (r *ReferenceFinder) Len() int {
	return len(r.idx)
}
