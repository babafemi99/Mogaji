package finder

import (
	"testing"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

func TestNewAmountWindowFinder_EmptyPool(t *testing.T) {
	af := NewAmountWindowFinder([]*domain.Transaction{}, 1.5, 86400)
	if af.Len() != 0 {
		t.Errorf("Len(): got %d want 0", af.Len())
	}
}

func TestNewAmountWindowFinder_IndexedByCurrencyAndMinute(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
		makeTxAt("ONY_002", 250000, "NGN", ts), // same currency + minute, different amount
		makeTxAt("ONY_003", 500000, "USD", ts), // different currency — separate bucket
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	// NGN:2026-06-21T09:00Z and USD:2026-06-21T09:00Z → two distinct keys.
	if af.Len() != 2 {
		t.Errorf("Len(): got %d want 2", af.Len())
	}
}

func TestNewAmountWindowFinder_StoresConfiguredThresholds(t *testing.T) {
	var pool []*domain.Transaction
	af := NewAmountWindowFinder(pool, 2.5, 3600)

	if af.feeTolerancePercent != 2.5 {
		t.Errorf("feeTolerancePercent: got %f want 2.5", af.feeTolerancePercent)
	}
	if af.timeWindowSeconds != 3600 {
		t.Errorf("timeWindowSeconds: got %d want 3600", af.timeWindowSeconds)
	}
}

func TestAmountWindowFinder_FindCandidates_ExactMatch(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d candidates want 1", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_WithinFeeTolerance(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	// External settled ₦492,500 — 1.5% fee deducted from ₦500,000.
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 492500, "NGN", ts),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	// Internal expects ₦500,000.
	query := makeTxAt("", 500000, "NGN", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d want 1 — 1.5%% fee within tolerance", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_OutsideFeeTolerance(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	// External settled ₦480,000 — 4% difference, outside 1.5% tolerance.
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 480000, "NGN", ts),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d want 0 — 4%% difference outside 1.5%% tolerance", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_ZeroTolerance_ExactOnly(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 492500, "NGN", ts), // slightly different
		makeTxAt("ONY_002", 500000, "NGN", ts), // exact match
	}
	af := NewAmountWindowFinder(pool, 0, 86400) // zero tolerance — exact only

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d want 1 — zero tolerance should only match exact amount", len(candidates))
	}
	if candidates[0].AmountMinorUnits != 500000 {
		t.Errorf("wrong candidate returned: got amount %d want 500000", candidates[0].AmountMinorUnits)
	}
}

func TestAmountWindowFinder_FindCandidates_WithinTimeWindow(t *testing.T) {
	externalTS := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	internalTS := time.Date(2026, 6, 21, 9, 0, 30, 0, time.UTC) // 30 seconds later

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", externalTS),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400) // 24h window

	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d want 1 — 30s within 24h window", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_OutsideTimeWindow(t *testing.T) {
	externalTS := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	internalTS := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC) // 25 hours later

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", externalTS),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400) // 24h window = 86400 seconds

	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := af.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d want 0 — 25h outside 24h window", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_CurrencyMismatch(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	query := makeTxAt("", 500000, "USD", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 0 {
		t.Errorf("FindCandidates(): got %d want 0 — currency mismatch", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_NeighboringBucket(t *testing.T) {
	// External at 09:30:59, internal at 09:31:01 — straddles minute boundary.
	externalTS := time.Date(2026, 6, 21, 9, 30, 59, 0, time.UTC)
	internalTS := time.Date(2026, 6, 21, 9, 31, 1, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 492500, "NGN", externalTS),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	query := makeTxAt("", 500000, "NGN", internalTS)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d want 1 — neighboring bucket should be checked", len(candidates))
	}
}

func TestAmountWindowFinder_FindCandidates_BothFiltersApplied(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 492500, "NGN", ts),                   // within tolerance, within window ✓
		makeTxAt("ONY_002", 480000, "NGN", ts),                   // outside tolerance ✗
		makeTxAt("ONY_003", 492500, "NGN", ts.Add(25*time.Hour)), // within tolerance, outside window ✗
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	query := makeTxAt("", 500000, "NGN", ts)
	candidates := af.FindCandidates(query)

	if len(candidates) != 1 {
		t.Errorf("FindCandidates(): got %d want 1 — only ONY_001 passes both filters", len(candidates))
	}
}

func TestWithinFeeTolerance(t *testing.T) {
	tests := []struct {
		name      string
		query     int64
		candidate int64
		tolerance float64
		want      bool
	}{
		{
			name:      "exact match — always within tolerance",
			query:     500000,
			candidate: 500000,
			tolerance: 1.5,
			want:      true,
		},
		{
			name:      "exactly at boundary — 1.5% of 500000 = 7500",
			query:     500000,
			candidate: 492500,
			tolerance: 1.5,
			want:      true,
		},
		{
			name:      "just outside boundary",
			query:     500000,
			candidate: 492499, // 7501 diff = 1.5002%
			tolerance: 1.5,
			want:      false,
		},
		{
			name:      "zero tolerance — exact only",
			query:     500000,
			candidate: 492500,
			tolerance: 0,
			want:      false,
		},
		{
			name:      "zero tolerance — exact match",
			query:     500000,
			candidate: 500000,
			tolerance: 0,
			want:      true,
		},
		{
			name:      "negative tolerance treated as zero — exact only",
			query:     500000,
			candidate: 492500,
			tolerance: -1.0,
			want:      false,
		},
		{
			name:      "candidate higher than query — within tolerance",
			query:     492500,
			candidate: 500000,
			tolerance: 1.6, // 7500/492500 * 100 = 1.523%
			want:      true,
		},
		{
			name:      "small amounts — 1 kobo difference",
			query:     100,
			candidate: 99,
			tolerance: 1.5, // 1/100 * 100 = 1%
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withinFeeTolerance(tt.query, tt.candidate, tt.tolerance)
			if got != tt.want {
				t.Errorf("withinFeeTolerance(%d, %d, %.1f)\n  got  %v\n  want %v",
					tt.query, tt.candidate, tt.tolerance, got, tt.want)
			}
		})
	}
}

func TestWithinTimeWindow(t *testing.T) {
	base := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		a             time.Time
		b             time.Time
		windowSeconds int64
		want          bool
	}{
		{
			name:          "same timestamp — always within window",
			a:             base,
			b:             base,
			windowSeconds: 86400,
			want:          true,
		},
		{
			name:          "exactly at boundary",
			a:             base,
			b:             base.Add(86400 * time.Second),
			windowSeconds: 86400,
			want:          true,
		},
		{
			name:          "one second outside boundary",
			a:             base,
			b:             base.Add(86401 * time.Second),
			windowSeconds: 86400,
			want:          false,
		},
		{
			name:          "b before a — absolute difference",
			a:             base,
			b:             base.Add(-3600 * time.Second),
			windowSeconds: 86400,
			want:          true,
		},
		{
			name:          "zero window — only exact match",
			a:             base,
			b:             base.Add(time.Second),
			windowSeconds: 0,
			want:          false,
		},
		{
			name:          "zero window — exact match",
			a:             base,
			b:             base,
			windowSeconds: 0,
			want:          true,
		},
		{
			name:          "1 hour window",
			a:             base,
			b:             base.Add(3599 * time.Second),
			windowSeconds: 3600,
			want:          true,
		},
		{
			name:          "1 hour window — just outside",
			a:             base,
			b:             base.Add(3601 * time.Second),
			windowSeconds: 3600,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withinTimeWindow(tt.a, tt.b, tt.windowSeconds)
			if got != tt.want {
				t.Errorf("withinTimeWindow()\n  got  %v\n  want %v", got, tt.want)
			}
		})
	}
}

func TestAmountWindowKey_Format(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 30, 45, 0, time.UTC)
	got := amountWindowKey("NGN", ts)
	want := "NGN:2026-06-21T09:30Z"

	if got != want {
		t.Errorf("amountWindowKey()\n  got  %q\n  want %q", got, want)
	}
}

func TestAmountWindowKey_TruncatesToMinute(t *testing.T) {
	ts1 := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 21, 9, 30, 59, 0, time.UTC)

	if amountWindowKey("NGN", ts1) != amountWindowKey("NGN", ts2) {
		t.Error("amountWindowKey() should produce same key for timestamps in same minute")
	}
}

func TestAmountWindowKey_DifferentCurrenciesDifferentKeys(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC)

	ngn := amountWindowKey("NGN", ts)
	usd := amountWindowKey("USD", ts)

	if ngn == usd {
		t.Errorf("amountWindowKey() should produce different keys for different currencies: both got %q", ngn)
	}
}

func TestAmountWindowFinder_NeverReturnsEmptySlice(t *testing.T) {
	ts := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	pool := []*domain.Transaction{
		makeTxAt("ONY_001", 500000, "NGN", ts),
	}
	af := NewAmountWindowFinder(pool, 1.5, 86400)

	queries := []*domain.Transaction{
		makeTxAt("", 500000, "NGN", ts),                   // match
		makeTxAt("", 999999, "NGN", ts),                   // no match — amount too different
		makeTxAt("", 500000, "USD", ts),                   // no match — currency
		makeTxAt("", 500000, "NGN", ts.Add(25*time.Hour)), // no match — time window
	}

	for i, query := range queries {
		candidates := af.FindCandidates(query)
		if candidates != nil && len(candidates) == 0 {
			t.Errorf("query[%d]: FindCandidates() returned empty slice — should be nil or non-empty", i)
		}
	}
}
