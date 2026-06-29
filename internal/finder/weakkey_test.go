package finder

import (
	"testing"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

// makeTxAt creates a transaction with a specific timestamp for WeakKeyFinder tests.
func makeTxAt(referenceID string, amount int64, currency string, t time.Time) *domain.Transaction {
	return &domain.Transaction{
		ReferenceID:      referenceID,
		AmountMinorUnits: amount,
		Currency:         currency,
		Timestamp:        t,
		SourceName:       "test_source",
		SourceRole:       domain.SourceRoleExternal,
	}
}

func TestNewWeakKeyFinder_EmptyPool(t *testing.T) {
	wf := NewWeakKeyFinder([]*domain.Transaction{})
	if wf.Len() != 0 {
		t.Errorf("Len(): got %d want 0", wf.Len())
	}
}

func TestNewWeakKeyFinder_SingleTransaction(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	if wf.Len() != 1 {
		t.Errorf("Len(): got %d want 1", wf.Len())
	}
}

func TestNewWeakKeyFinder_TransactionsInSameMinuteBucket(t *testing.T) {
	// Both transactions are in the same minute — same bucket, one key.
	ts1 := time.Date(2026, 6, 21, 9, 0, 10, 0, time.UTC)
	ts2 := time.Date(2026, 6, 21, 9, 0, 45, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts1),
		makeTxAt("ONY_002", 500000, "NGN", ts2),
	}
	wf := NewWeakKeyFinder(pool)

	// Same amount + currency + minute → one key, two transactions.
	if wf.Len() != 1 {
		t.Errorf("Len(): got %d want 1 — same minute bucket should share one key", wf.Len())
	}
}

func TestNewWeakKeyFinder_TransactionsInDifferentMinuteBuckets(t *testing.T) {
	ts1 := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 21, 9, 1, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts1),
		makeTxAt("ONY_002", 500000, "NGN", ts2),
	}
	wf := NewWeakKeyFinder(pool)

	// Different minutes → two distinct keys.
	if wf.Len() != 2 {
		t.Errorf("Len(): got %d want 2 — different minute buckets should be distinct keys", wf.Len())
	}
}

func TestNewWeakKeyFinder_DifferentCurrenciesSeparateBuckets(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
		makeTxAt("ONY_002", 500000, "USD", ts),
	}
	wf := NewWeakKeyFinder(pool)

	// Same amount + minute but different currency → two keys.
	if wf.Len() != 2 {
		t.Errorf("Len(): got %d want 2 — different currencies should be distinct keys", wf.Len())
	}
}

// ── FindCandidates ────────────────────────────────────────────────────────────

func TestWeakKeyFinder_FindCandidates_ExactMinuteMatch(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 30, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d candidates want 1", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_NoMatch(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	// Different amount — should not match.
	query := makeTxAt("", 250000, "NGN", ts)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d candidates want 0 for different amount", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_NeighboringBucket_Previous(t *testing.T) {
	// External transaction at 09:30:59 — truncates to 09:30 bucket.
	externalTS := time.Date(2026, 6, 21, 9, 30, 59, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", externalTS),
	}
	wf := NewWeakKeyFinder(pool)

	// Internal transaction at 09:31:01 — truncates to 09:31 bucket.
	// The external is in the previous bucket relative to internal.
	internalTS := time.Date(2026, 6, 21, 9, 31, 1, 0, time.UTC)
	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d candidates want 1 — neighboring bucket should be checked", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_NeighboringBucket_Next(t *testing.T) {
	// External transaction at 09:31:01 — truncates to 09:31 bucket.
	externalTS := time.Date(2026, 6, 21, 9, 31, 1, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", externalTS),
	}
	wf := NewWeakKeyFinder(pool)

	// Internal transaction at 09:30:59 — truncates to 09:30 bucket.
	// The external is in the next bucket relative to internal.
	internalTS := time.Date(2026, 6, 21, 9, 30, 59, 0, time.UTC)
	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d candidates want 1 — neighboring bucket should be checked", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_BeyondNeighboringBucket(t *testing.T) {
	// External at 09:00, internal at 09:02 — two minutes apart, outside ±1 window.
	externalTS := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	internalTS := time.Date(2026, 6, 21, 9, 2, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", externalTS),
	}
	wf := NewWeakKeyFinder(pool)

	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d candidates want 0 — 2 minutes apart is outside ±1 window", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_CurrencyMismatch(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	// Same amount and time but different currency.
	query := makeTxAt("", 500000, "USD", ts)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d candidates want 0 — currency mismatch should not match", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_MultipleCandidates(t *testing.T) {
	// Two external transactions with same amount + currency + minute.
	// Common in bulk payouts or salary runs.
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
		makeTxAt("ONY_002", 500000, "NGN", ts),
		makeTxAt("ONY_003", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 3 {
		t.Errorf("FindCandidates(): got %d candidates want 3", len(candidates))
	}
}

func TestWeakKeyFinder_FindCandidates_DeduplicatesAcrossBuckets(t *testing.T) {
	// Transaction exactly on a minute boundary — appears in multiple bucket queries
	// but should only be returned once.
	ts := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC) // exactly on minute boundary

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	// Query with same timestamp — own bucket will find it.
	query := makeTxAt("", 500000, "NGN", ts)
	candidates := wf.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d candidates want 1 — dedup should prevent duplicates", len(candidates))
	}
}

func TestWeakKey_Format(t *testing.T) {
	ts := time.Date(2026, 6, 21, 13, 30, 45, 0, time.UTC)
	got := weakKey(500000, "NGN", ts)
	want := "500000:NGN:2026-06-21T13:30Z"

	if got != want {
		t.Errorf("weakKey()\n  got  %q\n  want %q", got, want)
	}
}

func TestWeakKey_TruncatesToMinute(t *testing.T) {
	// Two timestamps in the same minute should produce the same key.
	ts1 := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 21, 9, 0, 59, 0, time.UTC)

	key1 := weakKey(500000, "NGN", ts1)
	key2 := weakKey(500000, "NGN", ts2)

	if key1 != key2 {
		t.Errorf("weakKey() should produce same key for timestamps in same minute\n  ts1: %q\n  ts2: %q", key1, key2)
	}
}

func TestWeakKey_DifferentMinutesDifferentKeys(t *testing.T) {
	ts1 := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 21, 9, 1, 0, 0, time.UTC)

	key1 := weakKey(500000, "NGN", ts1)
	key2 := weakKey(500000, "NGN", ts2)

	if key1 == key2 {
		t.Errorf("weakKey() should produce different keys for different minutes: both got %q", key1)
	}
}

func TestWeakKey_AlwaysUTC(t *testing.T) {
	// Same moment in time expressed in different timezones should produce same key.
	lagos, _ := time.LoadLocation("Africa/Lagos")

	utcTS := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	lagosTS := time.Date(2026, 6, 21, 10, 0, 0, 0, lagos) // 10:00 WAT = 09:00 UTC

	keyUTC := weakKey(500000, "NGN", utcTS)
	keyLagos := weakKey(500000, "NGN", lagosTS)

	if keyUTC != keyLagos {
		t.Errorf("weakKey() should normalize to UTC\n  UTC key:   %q\n  Lagos key: %q", keyUTC, keyLagos)
	}
}

func TestWeakKeyFinder_NeverReturnsEmptySlice(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	wf := NewWeakKeyFinder(pool)

	queries := []*domain.Transaction{
		makeTxAt("", 500000, "NGN", ts),                    // match
		makeTxAt("", 999999, "NGN", ts),                    // no match — different amount
		makeTxAt("", 500000, "USD", ts),                    // no match — different currency
		makeTxAt("", 500000, "NGN", ts.Add(2*time.Minute)), // no match — too far
	}

	for i, query := range queries {
		candidates := wf.FindCandidates(query)
		if candidates != nil && len(candidates) == 0 {
			t.Errorf("query[%d]: FindCandidates() returned empty slice — should be nil or non-empty", i)
		}
	}
}
