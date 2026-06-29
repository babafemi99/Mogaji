package finder

import (
	"testing"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

// makeTx is a helper that creates a minimal Transaction for testing.
func makeTx(referenceID string, amount int64) *domain.Transaction {
	return &domain.Transaction{
		ReferenceID:      referenceID,
		AmountMinorUnits: amount,
		Currency:         "NGN",
		Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC),
		SourceName:       "test_source",
		SourceRole:       domain.SourceRoleExternal,
	}
}

func TestNewReferenceFinder_EmptyPool(t *testing.T) {
	rf := NewReferenceFinder([]*domain.Transaction{})
	if rf.Len() != 0 {
		t.Errorf("Len(): got %d want 0", rf.Len())
	}
}

func TestNewReferenceFinder_SingleTransaction(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
	}
	rf := NewReferenceFinder(pool)

	if rf.Len() != 1 {
		t.Errorf("Len(): got %d want 1", rf.Len())
	}
}

func TestNewReferenceFinder_MultipleTransactions(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
		makeTx("ONY_002", 250000),
		makeTx("ONY_003", 100000),
	}
	rf := NewReferenceFinder(pool)

	if rf.Len() != 3 {
		t.Errorf("Len(): got %d want 3", rf.Len())
	}
}

func TestNewReferenceFinder_BlankReferencesExcluded(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("", 500000),
		makeTx("", 250000),
		makeTx("ONY_001", 100000),
	}
	rf := NewReferenceFinder(pool)

	// Only ONY_001 should be indexed — blank references excluded.
	if rf.Len() != 1 {
		t.Errorf("Len(): got %d want 1 — blank references should not be indexed", rf.Len())
	}
}

func TestNewReferenceFinder_DuplicateReferencesBothIndexed(t *testing.T) {
	tx1 := makeTx("ONY_001", 500000)
	tx2 := makeTx("ONY_001", 492500) // same reference, different amount (fee deducted)

	pool := []*domain.Transaction{tx1, tx2}
	rf := NewReferenceFinder(pool)

	// One distinct key but two transactions under it.
	if rf.Len() != 1 {
		t.Errorf("Len(): got %d want 1 — duplicates share one key", rf.Len())
	}

	query := makeTx("ONY_001", 500000)
	candidates := rf.FindCandidates(query)
	if len(candidates) != 2 {
		t.Errorf("FindCandidates(): got %d candidates want 2", len(candidates))
	}
}

func TestNewReferenceFinder_MixedPool(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("", 500000),        // blank — excluded
		makeTx("ONY_001", 500000), // valid
		makeTx("ONY_001", 492500), // duplicate — also indexed
		makeTx("ONY_002", 250000), // valid
		makeTx("", 100000),        // blank — excluded
	}
	rf := NewReferenceFinder(pool)

	// Two distinct keys: ONY_001 (with 2 entries) and ONY_002.
	if rf.Len() != 2 {
		t.Errorf("Len(): got %d want 2", rf.Len())
	}
}

func TestFindCandidates_EmptyReferenceReturnsNil(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
	}
	rf := NewReferenceFinder(pool)

	query := makeTx("", 500000)
	candidates := rf.FindCandidates(query)
	if candidates != nil {
		t.Errorf("FindCandidates(): got %v want nil for empty reference", candidates)
	}
}

func TestFindCandidates_ReferenceNotInIndex(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
	}
	rf := NewReferenceFinder(pool)

	query := makeTx("ONY_999", 500000)
	candidates := rf.FindCandidates(query)
	if candidates != nil {
		t.Errorf("FindCandidates(): got %v want nil for missing reference", candidates)
	}
}

func TestFindCandidates_SingleMatch(t *testing.T) {
	tx := makeTx("ONY_001", 500000)
	rf := NewReferenceFinder([]*domain.Transaction{tx})

	query := makeTx("ONY_001", 500000)
	candidates := rf.FindCandidates(query)

	if len(candidates) != 1 {
		t.Fatalf("FindCandidates(): got %d candidates want 1", len(candidates))
	}
	if candidates[0] != tx {
		t.Error("FindCandidates(): returned wrong transaction pointer")
	}
}

func TestFindCandidates_MultipleMatches(t *testing.T) {
	tx1 := makeTx("ONY_001", 500000)
	tx2 := makeTx("ONY_001", 492500)
	rf := NewReferenceFinder([]*domain.Transaction{tx1, tx2})

	query := makeTx("ONY_001", 500000)
	candidates := rf.FindCandidates(query)

	if len(candidates) != 2 {
		t.Fatalf("FindCandidates(): got %d candidates want 2", len(candidates))
	}
}

func TestFindCandidates_CaseSensitive(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
	}
	rf := NewReferenceFinder(pool)

	// Lowercase variant should NOT match.
	query := makeTx("ony_001", 500000)
	candidates := rf.FindCandidates(query)
	if candidates != nil {
		t.Errorf("FindCandidates(): reference matching should be case-sensitive, got %v", candidates)
	}
}

func TestLen_EmptyPool(t *testing.T) {
	rf := NewReferenceFinder([]*domain.Transaction{})
	if rf.Len() != 0 {
		t.Errorf("Len(): got %d want 0", rf.Len())
	}
}

func TestLen_CountsDistinctKeys(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
		makeTx("ONY_001", 492500), // duplicate — same key
		makeTx("ONY_002", 250000),
		makeTx("ONY_003", 100000),
	}
	rf := NewReferenceFinder(pool)

	// 4 transactions but only 3 distinct keys.
	if rf.Len() != 3 {
		t.Errorf("Len(): got %d want 3 — should count distinct keys not total transactions", rf.Len())
	}
}

func TestReferenceFinder_BlankKeyNeverInIndex(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("", 500000),
		makeTx("", 250000),
		makeTx("", 100000),
	}
	rf := NewReferenceFinder(pool)

	// Index should be completely empty.
	if rf.Len() != 0 {
		t.Errorf("blank references polluted the index: Len()=%d want 0", rf.Len())
	}

	// Querying with blank reference should return nil — not an empty slice.
	query := makeTx("", 500000)
	candidates := rf.FindCandidates(query)
	if candidates != nil {
		t.Errorf("FindCandidates() with blank reference: got %v want nil", candidates)
	}
}

func TestReferenceFinder_NeverReturnsEmptySlice(t *testing.T) {
	pool := []*domain.Transaction{
		makeTx("ONY_001", 500000),
		makeTx("ONY_002", 250000),
	}
	rf := NewReferenceFinder(pool)

	queries := []string{"ONY_001", "ONY_002", "ONY_999", ""}

	for _, ref := range queries {
		t.Run(ref, func(t *testing.T) {
			query := makeTx(ref, 500000)
			candidates := rf.FindCandidates(query)

			// Result must be nil or non-empty — never an empty slice.
			if candidates != nil && len(candidates) == 0 {
				t.Errorf("FindCandidates(%q) returned empty slice — should be nil or non-empty", ref)
			}
		})
	}
}
