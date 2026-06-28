package ingest

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/sancho"
)

func TestParseTimestamp(t *testing.T) {
	lagos, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Fatalf("failed to load Lagos timezone: %v", err)
	}

	utc := time.UTC

	tests := []struct {
		name     string
		raw      string
		loc      *time.Location
		wantTime time.Time
		wantErr  bool
	}{
		// --- ISO 8601 with timezone offset ---
		{
			name:     "ISO 8601 with UTC offset",
			raw:      "2026-06-21T09:00:00Z",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
		},
		{
			name:     "ISO 8601 with +01:00 offset",
			raw:      "2026-06-21T09:00:00+01:00",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 8, 0, 0, 0, utc), // converted to UTC
		},

		// --- ISO 8601 without timezone — source loc applied ---
		{
			name:     "ISO 8601 no timezone Lagos loc",
			raw:      "2026-06-21T09:00:00",
			loc:      lagos,
			wantTime: time.Date(2026, 6, 21, 8, 0, 0, 0, utc), // WAT is UTC+1
		},
		{
			name:     "ISO 8601 no timezone UTC loc",
			raw:      "2026-06-21T09:00:00",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
		},

		// --- Common DB export format ---
		{
			name:     "YYYY-MM-DD HH:MM:SS UTC",
			raw:      "2026-06-21 09:00:00",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
		},
		{
			name:     "YYYY-MM-DD HH:MM:SS Lagos loc",
			raw:      "2026-06-21 09:00:00",
			loc:      lagos,
			wantTime: time.Date(2026, 6, 21, 8, 0, 0, 0, utc), // WAT → UTC
		},

		// --- With numeric offset ---
		{
			name:     "YYYY-MM-DD HH:MM:SS with +0100 offset",
			raw:      "2026-06-21 09:00:00 +0100",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 8, 0, 0, 0, utc),
		},

		// --- DD/MM/YYYY format — Nigerian bank exports ---
		{
			name:     "DD/MM/YYYY HH:MM:SS UTC",
			raw:      "21/06/2026 09:00:00",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
		},
		{
			name:     "DD/MM/YYYY HH:MM:SS Lagos loc",
			raw:      "21/06/2026 09:00:00",
			loc:      lagos,
			wantTime: time.Date(2026, 6, 21, 8, 0, 0, 0, utc),
		},

		// --- Date only formats ---
		{
			name:     "YYYY-MM-DD date only",
			raw:      "2026-06-21",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 0, 0, 0, 0, utc),
		},
		{
			name:     "DD/MM/YYYY date only",
			raw:      "21/06/2026",
			loc:      utc,
			wantTime: time.Date(2026, 6, 21, 0, 0, 0, 0, utc),
		},

		// --- Ambiguity: DD/MM wins over MM/DD because it's listed first ---
		{
			// "01/02/2026" is ambiguous — could be Jan 2 or Feb 1.
			// DD/MM/YYYY is listed first in formats so Feb 1 wins.
			name:     "ambiguous date parsed as DD/MM/YYYY",
			raw:      "01/02/2026",
			loc:      utc,
			wantTime: time.Date(2026, 2, 1, 0, 0, 0, 0, utc), // Feb 1, not Jan 2
		},

		// --- Timezone boundary: midnight ---
		{
			name:     "midnight Lagos converts to previous day UTC",
			raw:      "2026-06-21 00:00:00",
			loc:      lagos,
			wantTime: time.Date(2026, 6, 20, 23, 0, 0, 0, utc), // WAT midnight = UTC 23:00 prev day
		},

		// --- Error cases ---
		{
			name:    "empty string",
			raw:     "",
			loc:     utc,
			wantErr: true,
		},
		{
			name:    "completely invalid format",
			raw:     "not-a-date",
			loc:     utc,
			wantErr: true,
		},
		{
			name:    "partial date",
			raw:     "2026-06",
			loc:     utc,
			wantErr: true,
		},
		{
			name:     "American format MM/DD/YYYY effectively dead — parses as DD/MM if valid",
			raw:      "13/01/2026", // day=13 is valid for DD/MM, invalid for MM/DD
			loc:      utc,
			wantTime: time.Date(2026, 1, 13, 0, 0, 0, 0, utc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimestamp(tt.raw, tt.loc)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTimestamp(%q) expected error but got nil", tt.raw)
				}
				return
			}

			if err != nil {
				t.Errorf("parseTimestamp(%q) unexpected error: %v", tt.raw, err)
				return
			}

			if !got.Equal(tt.wantTime) {
				t.Errorf("parseTimestamp(%q)\n  got  %v\n  want %v", tt.raw, got, tt.wantTime)
			}

			// Always verify the result is UTC.
			if got.Location() != time.UTC {
				t.Errorf("parseTimestamp(%q) result is not UTC: got location %v", tt.raw, got.Location())
			}
		})
	}
}

// TestParseTimestampAlwaysUTC verifies the core invariant:
// regardless of input timezone, the result is always UTC.
func TestParseTimestampAlwaysUTC(t *testing.T) {
	locations := []string{
		"Africa/Lagos",
		"Africa/Nairobi",
		"America/New_York",
		"Asia/Tokyo",
		"UTC",
	}

	raw := "2026-06-21 12:00:00"

	for _, locName := range locations {
		t.Run(locName, func(t *testing.T) {
			loc, err := time.LoadLocation(locName)
			if err != nil {
				t.Fatalf("failed to load location %q: %v", locName, err)
			}

			got, err := parseTimestamp(raw, loc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Location() != time.UTC {
				t.Errorf("result is not UTC for location %q: got %v", locName, got.Location())
			}
		})
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		minorUnits bool
		multiplier int64
		want       int64
		wantErr    error // nil means no error expected
	}{
		// --- Minor units: already in kobo/cents, parse as integer ---
		{
			name:       "minor units: valid integer",
			raw:        "500000",
			minorUnits: true,
			multiplier: 100,
			want:       500000,
		},
		{
			name:       "minor units: zero",
			raw:        "0",
			minorUnits: true,
			multiplier: 100,
			want:       0,
		},
		{
			name:       "minor units: large value",
			raw:        "999999999",
			minorUnits: true,
			multiplier: 100,
			want:       999999999,
		},
		{
			name:       "minor units: with comma formatting stripped",
			raw:        "1,000,000",
			minorUnits: true,
			multiplier: 100,
			want:       1000000,
		},
		{
			name:       "minor units: with spaces stripped",
			raw:        "500 000",
			minorUnits: true,
			multiplier: 100,
			want:       500000,
		},
		{
			name:       "minor units: negative",
			raw:        "-500000",
			minorUnits: true,
			multiplier: 100,
			wantErr:    sancho.ErrNegativeAmount,
		},
		{
			name:       "minor units: invalid string",
			raw:        "abc",
			minorUnits: true,
			multiplier: 100,
			wantErr:    sancho.ErrInvalidMinorUnits,
		},
		{
			name:       "minor units: float string rejected",
			raw:        "5000.75",
			minorUnits: true,
			multiplier: 100,
			wantErr:    sancho.ErrInvalidMinorUnits,
		},
		{
			name:       "minor units: empty string",
			raw:        "",
			minorUnits: true,
			multiplier: 100,
			wantErr:    sancho.ErrInvalidMinorUnits,
		},

		// --- Major units: naira/dollars with decimal, convert to minor ---
		{
			name:       "major units: whole number",
			raw:        "5000",
			minorUnits: false,
			multiplier: 100,
			want:       500000, // ₦5000 → 500000 kobo
		},
		{
			name:       "major units: with decimal",
			raw:        "5000.75",
			minorUnits: false,
			multiplier: 100,
			want:       500075, // ₦5000.75 → 500075 kobo
		},
		{
			name:       "major units: with comma formatting stripped",
			raw:        "1,500.00",
			minorUnits: false,
			multiplier: 100,
			want:       150000,
		},
		{
			name:       "major units: zero",
			raw:        "0.00",
			minorUnits: false,
			multiplier: 100,
			want:       0,
		},
		{
			name:       "major units: rounding — .005 rounds up",
			raw:        "0.005",
			minorUnits: false,
			multiplier: 100,
			want:       1, // 0.005 * 100 = 0.5 → rounds to 1
		},
		{
			name:       "major units: rounding — .004 rounds down",
			raw:        "0.004",
			minorUnits: false,
			multiplier: 100,
			want:       0, // 0.004 * 100 = 0.4 → rounds to 0
		},
		{
			name:       "major units: Paystack typical fee deduction",
			raw:        "4925.00",
			minorUnits: false,
			multiplier: 100,
			want:       492500, // ₦4925.00 after 1.5% fee on ₦5000
		},
		{
			name:       "major units: negative",
			raw:        "-5000.00",
			minorUnits: false,
			multiplier: 100,
			wantErr:    sancho.ErrNegativeAmount,
		},
		{
			name:       "major units: invalid string",
			raw:        "abc",
			minorUnits: false,
			multiplier: 100,
			wantErr:    sancho.ErrInvalidDecimalAmount,
		},
		{
			name:       "major units: empty string",
			raw:        "",
			minorUnits: false,
			multiplier: 100,
			wantErr:    sancho.ErrInvalidDecimalAmount,
		},

		// --- Multiplier variations ---
		{
			name:       "multiplier 1000 — KWD 3 decimal places",
			raw:        "1.500",
			minorUnits: false,
			multiplier: 1000,
			want:       1500,
		},
		{
			name:       "multiplier 1 — JPY no decimal places",
			raw:        "1500",
			minorUnits: false,
			multiplier: 1,
			want:       1500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAmount(tt.raw, tt.minorUnits, tt.multiplier)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("parseAmount(%q) expected error %v but got nil", tt.raw, tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("parseAmount(%q)\n  got error  %v\n  want error %v", tt.raw, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("parseAmount(%q) unexpected error: %v", tt.raw, err)
				return
			}

			if got != tt.want {
				t.Errorf("parseAmount(%q)\n  got  %d\n  want %d", tt.raw, got, tt.want)
			}
		})
	}
}

// TestParseAmountNeverNegative verifies the core invariant:
// parseAmount never returns a negative value.
func TestParseAmountNeverNegative(t *testing.T) {
	cases := []struct {
		raw        string
		minorUnits bool
	}{
		{"-1", true},
		{"-0.01", false},
		{"-999999", true},
		{"-1,000.00", false},
	}

	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			got, err := parseAmount(c.raw, c.minorUnits, 100)
			if err == nil {
				t.Errorf("parseAmount(%q) should have returned error for negative amount, got %d", c.raw, got)
				return
			}
			if !errors.Is(err, sancho.ErrNegativeAmount) {
				t.Errorf("parseAmount(%q) wrong error type: got %v, want %v", c.raw, err, sancho.ErrNegativeAmount)
			}
			if got != 0 {
				t.Errorf("parseAmount(%q) returned non-zero value %d on error", c.raw, got)
			}
		})
	}
}

func TestNormalizeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// --- Examples from the function docs ---
		{
			name:  "space separated title case",
			input: "Transaction ID",
			want:  "transaction_id",
		},
		{
			name:  "hyphen separated",
			input: "transaction-id",
			want:  "transaction_id",
		},
		{
			name:  "all caps with space",
			input: "TRANSACTION ID",
			want:  "transaction_id",
		},
		{
			name:  "slash separator",
			input: "Date/Time",
			want:  "date_time",
		},
		{
			name:  "parentheses stripped",
			input: "amount (NGN)",
			want:  "amount_ngn",
		},

		// --- Casing ---
		{
			name:  "already lowercase",
			input: "txn_ref",
			want:  "txn_ref",
		},
		{
			name:  "mixed case",
			input: "Settled Amount",
			want:  "settled_amount",
		},
		{
			name:  "all caps",
			input: "CURRENCY",
			want:  "currency",
		},

		// --- Separators ---
		{
			name:  "dot separator",
			input: "transaction.date",
			want:  "transaction_date",
		},
		{
			name:  "multiple spaces",
			input: "transaction  date",
			want:  "transaction_date",
		},
		{
			name:  "mixed separators",
			input: "Transaction-Date/Time",
			want:  "transaction_date_time",
		},

		// --- Special characters stripped ---
		{
			name:  "brackets stripped",
			input: "amount[NGN]",
			want:  "amountngn",
		},
		{
			name:  "hash stripped",
			input: "txn#id",
			want:  "txnid",
		},
		{
			name:  "asterisk stripped",
			input: "amount*fee",
			want:  "amountfee",
		},
		{
			name:  "percent stripped",
			input: "fee%",
			want:  "fee",
		},

		// --- Underscore collapsing ---
		{
			name:  "consecutive underscores collapsed",
			input: "transaction__id",
			want:  "transaction_id",
		},
		{
			name:  "many consecutive underscores collapsed",
			input: "transaction____id",
			want:  "transaction_id",
		},

		// --- Leading and trailing underscore trimming ---
		{
			name:  "leading underscore trimmed",
			input: "_transaction_id",
			want:  "transaction_id",
		},
		{
			name:  "trailing underscore trimmed",
			input: "transaction_id_",
			want:  "transaction_id",
		},
		{
			name:  "leading and trailing special chars trimmed",
			input: "(transaction_id)",
			want:  "transaction_id",
		},

		// --- Whitespace ---
		{
			name:  "leading whitespace trimmed",
			input: "  transaction_id",
			want:  "transaction_id",
		},
		{
			name:  "trailing whitespace trimmed",
			input: "transaction_id  ",
			want:  "transaction_id",
		},
		{
			name:  "both sides whitespace trimmed",
			input: "  transaction_id  ",
			want:  "transaction_id",
		},

		// --- Numbers ---
		{
			name:  "numbers preserved",
			input: "address1",
			want:  "address1",
		},
		{
			name:  "numbers with spaces",
			input: "address 1",
			want:  "address_1",
		},

		// --- Real PSP header examples ---
		{
			name:  "Paystack: Transaction ID",
			input: "Transaction ID",
			want:  "transaction_id",
		},
		{
			name:  "Paystack: Settled Amount",
			input: "Settled Amount",
			want:  "settled_amount",
		},
		{
			name:  "Flutterwave: tx_ref",
			input: "tx_ref",
			want:  "tx_ref",
		},
		{
			name:  "Flutterwave: Amount Settled",
			input: "Amount Settled",
			want:  "amount_settled",
		},
		{
			name:  "bank export: Date/Time",
			input: "Date/Time",
			want:  "date_time",
		},
		{
			name:  "bank export: Tran. Amount",
			input: "Tran. Amount",
			want:  "tran_amount",
		},

		// --- Edge cases ---
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only special characters",
			input: "---",
			want:  "",
		},
		{
			name:  "only whitespace",
			input: "   ",
			want:  "",
		},
		{
			name:  "single character",
			input: "A",
			want:  "a",
		},
		{
			name:  "single underscore",
			input: "_",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeHeader(tt.input)
			if got != tt.want {
				t.Errorf("normalizeHeader(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeHeaderIdempotent verifies that running normalizeHeader twice
// produces the same result as running it once.
// A normalized header should already be in its canonical form.
func TestNormalizeHeaderIdempotent(t *testing.T) {
	inputs := []string{
		"Transaction ID",
		"Settled Amount",
		"Date/Time",
		"amount (NGN)",
		"TRANSACTION-ID",
		"tx_ref",
		"",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			once := normalizeHeader(input)
			twice := normalizeHeader(once)
			if once != twice {
				t.Errorf("normalizeHeader not idempotent for %q:\n  first:  %q\n  second: %q",
					input, once, twice)
			}
		})
	}
}

// TestNormalizeHeaderCollisionRisk documents known inputs that normalize
// to the same key — these would be caught by buildColumnIndex but are
// worth being explicit about.
func TestNormalizeHeaderCollisionRisk(t *testing.T) {
	collisions := [][]string{
		{"Transaction ID", "transaction_id", "TRANSACTION ID", "transaction-id"},
		{"Settled Amount", "settled_amount", "SETTLED AMOUNT", "settled-amount"},
	}

	for _, group := range collisions {
		t.Run(group[0], func(t *testing.T) {
			first := normalizeHeader(group[0])
			for _, input := range group[1:] {
				got := normalizeHeader(input)
				if got != first {
					t.Errorf("expected %q and %q to normalize to the same key\n  got %q and %q",
						group[0], input, first, got)
				}
			}
		})
	}
}

func TestBuildColumnIndex(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		want    map[string]int
		wantErr error
	}{
		// --- Happy path ---
		{
			name:    "simple headers",
			headers: []string{"txn_ref", "amount", "currency", "created_at"},
			want: map[string]int{
				"txn_ref":    0,
				"amount":     1,
				"currency":   2,
				"created_at": 3,
			},
		},
		{
			name:    "headers with mixed casing and spaces normalized",
			headers: []string{"Transaction ID", "Settled Amount", "Currency", "Transaction Date"},
			want: map[string]int{
				"transaction_id":   0,
				"settled_amount":   1,
				"currency":         2,
				"transaction_date": 3,
			},
		},
		{
			name:    "headers with hyphens and dots normalized",
			headers: []string{"txn-ref", "tran.amount", "Date/Time"},
			want: map[string]int{
				"txn_ref":     0,
				"tran_amount": 1,
				"date_time":   2,
			},
		},
		{
			name:    "single header",
			headers: []string{"amount"},
			want: map[string]int{
				"amount": 0,
			},
		},
		{
			name:    "empty headers slice",
			headers: []string{},
			want:    map[string]int{},
		},

		// --- Blank headers skipped ---
		{
			name:    "blank header skipped",
			headers: []string{"txn_ref", "", "amount"},
			want: map[string]int{
				"txn_ref": 0,
				"amount":  2,
			},
		},
		{
			name:    "whitespace only header skipped",
			headers: []string{"txn_ref", "   ", "amount"},
			want: map[string]int{
				"txn_ref": 0,
				"amount":  2,
			},
		},
		{
			name:    "special chars only header skipped",
			headers: []string{"txn_ref", "---", "amount"},
			want: map[string]int{
				"txn_ref": 0,
				"amount":  2,
			},
		},

		// --- Collision detection ---
		{
			name:    "collision: same string",
			headers: []string{"amount", "amount"},
			wantErr: sancho.ErrAmbiguousHeaders,
		},
		{
			name:    "collision: casing variants",
			headers: []string{"Transaction ID", "transaction_id"},
			wantErr: sancho.ErrAmbiguousHeaders,
		},
		{
			name:    "collision: space vs underscore",
			headers: []string{"Settled Amount", "settled_amount"},
			wantErr: sancho.ErrAmbiguousHeaders,
		},
		{
			name:    "collision: hyphen vs space",
			headers: []string{"txn-ref", "txn ref"},
			wantErr: sancho.ErrAmbiguousHeaders,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildColumnIndex(tt.headers)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("buildColumnIndex(%v) expected error %v but got nil", tt.headers, tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("buildColumnIndex(%v)\n  got error  %v\n  want error %v", tt.headers, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("buildColumnIndex(%v) unexpected error: %v", tt.headers, err)
				return
			}

			// Verify length matches.
			if len(got) != len(tt.want) {
				t.Errorf("buildColumnIndex(%v)\n  got  %d entries\n  want %d entries", tt.headers, len(got), len(tt.want))
				return
			}

			// Verify each expected key and index.
			for key, wantIdx := range tt.want {
				gotIdx, exists := got[key]
				if !exists {
					t.Errorf("buildColumnIndex(%v) missing key %q in result", tt.headers, key)
					continue
				}
				if gotIdx != wantIdx {
					t.Errorf("buildColumnIndex(%v) key %q\n  got index  %d\n  want index %d",
						tt.headers, key, gotIdx, wantIdx)
				}
			}
		})
	}
}

// TestBuildColumnIndexPreservesPosition verifies that the column index
// value correctly reflects the original position in the headers slice.
// This is critical — a wrong index means reading the wrong column from every row.
func TestBuildColumnIndexPreservesPosition(t *testing.T) {
	headers := []string{
		"Transaction ID",   // 0
		"Settled Amount",   // 1
		"Currency",         // 2
		"Transaction Date", // 3
		"Status",           // 4
	}

	idx, err := buildColumnIndex(headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectations := map[string]int{
		"transaction_id":   0,
		"settled_amount":   1,
		"currency":         2,
		"transaction_date": 3,
		"status":           4,
	}

	for key, wantPos := range expectations {
		gotPos, exists := idx[key]
		if !exists {
			t.Errorf("key %q not found in index", key)
			continue
		}
		if gotPos != wantPos {
			t.Errorf("key %q: got position %d, want position %d", key, gotPos, wantPos)
		}
	}
}

func TestOptionalCol(t *testing.T) {
	idx := map[string]int{
		"transaction_id": 0,
		"amount":         1,
		"currency":       2,
	}

	tests := []struct {
		name  string
		field string
		want  int
	}{
		{
			name:  "empty field returns -1",
			field: "",
			want:  -1,
		},
		{
			name:  "field not in index returns -1",
			field: "status",
			want:  -1,
		},
		{
			name:  "exact match found",
			field: "amount",
			want:  1,
		},
		{
			name:  "normalized match found — casing",
			field: "Transaction ID",
			want:  0,
		},
		{
			name:  "normalized match found — hyphen",
			field: "transaction-id",
			want:  0,
		},
		{
			name:  "normalized match found — space",
			field: "transaction id",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := optionalCol(idx, tt.field)
			if got != tt.want {
				t.Errorf("optionalCol(%q)\n  got  %d\n  want %d", tt.field, got, tt.want)
			}
		})
	}
}

// testSourceConfig returns a minimal SourceConfig for testing parseRow.
func testSourceConfig() domain.SourceConfig {
	return domain.SourceConfig{
		Name:     "test_source",
		File:     "test.csv",
		Role:     domain.SourceRoleExternal,
		Timezone: "UTC",
	}
}

// testColumnPositions returns column positions for a standard test CSV layout:
// col 0: reference_id
// col 1: amount
// col 2: currency
// col 3: timestamp
// col 4: status
func testColumnPositions() columnPositions {
	return columnPositions{
		reference: 0,
		amount:    1,
		currency:  2,
		timestamp: 3,
		status:    4,
	}
}

func TestParseRow(t *testing.T) {
	utc := time.UTC
	cfg := testSourceConfig()
	cols := testColumnPositions()
	multiplier := int64(100) // NGN — major units to kobo

	tests := []struct {
		name        string
		row         []string
		runCurrency string
		wantTx      *domain.Transaction
		wantErr     error
	}{
		// --- Happy path ---
		{
			name:        "valid complete row",
			row:         []string{"ONY_001", "5000.00", "NGN", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantTx: &domain.Transaction{
				ReferenceID:      "ONY_001",
				SourceName:       "test_source",
				SourceRole:       domain.SourceRoleExternal,
				AmountMinorUnits: 500000,
				Currency:         "NGN",
				Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
				RawStatus:        "success",
				SourceFile:       "test.csv",
				SourceRow:        2,
			},
		},
		{
			name:        "valid row with empty reference ID",
			row:         []string{"", "2500.00", "NGN", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantTx: &domain.Transaction{
				ReferenceID:      "",
				AmountMinorUnits: 250000,
				Currency:         "NGN",
				Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
				RawStatus:        "success",
			},
		},
		{
			name:        "valid row with comma formatted amount",
			row:         []string{"ONY_002", "1,500.00", "NGN", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantTx: &domain.Transaction{
				ReferenceID:      "ONY_002",
				AmountMinorUnits: 150000,
				Currency:         "NGN",
				Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, utc),
				RawStatus:        "success",
			},
		},
		{
			name:        "currency column lowercase normalized to uppercase",
			row:         []string{"ONY_003", "5000.00", "ngn", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantTx: &domain.Transaction{
				ReferenceID:      "ONY_003",
				AmountMinorUnits: 500000,
				Currency:         "NGN",
			},
		},
		{
			name:        "missing status column — empty string",
			row:         []string{"ONY_004", "5000.00", "NGN", "2026-06-21 09:00:00"},
			runCurrency: "NGN",
			wantTx: &domain.Transaction{
				ReferenceID:      "ONY_004",
				AmountMinorUnits: 500000,
				Currency:         "NGN",
				RawStatus:        "", // status col beyond row length
			},
		},

		// --- Currency mismatch ---
		{
			name:        "currency mismatch",
			row:         []string{"ONY_005", "5000.00", "USD", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrCurrencyMismatch,
		},

		// --- Amount errors ---
		{
			name:        "amount column missing — row too short",
			row:         []string{"ONY_006"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrAmountColumnMissing,
		},
		{
			name:        "invalid amount",
			row:         []string{"ONY_007", "not-a-number", "NGN", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrInvalidAmount,
		},
		{
			name:        "negative amount",
			row:         []string{"ONY_008", "-5000.00", "NGN", "2026-06-21 09:00:00", "success"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrInvalidAmount,
		},

		// --- Timestamp errors ---
		{
			name:        "timestamp column missing — row too short",
			row:         []string{"ONY_009", "5000.00", "NGN"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrTimestampColumnMissing,
		},
		{
			name:        "invalid timestamp",
			row:         []string{"ONY_010", "5000.00", "NGN", "not-a-date", "success"},
			runCurrency: "NGN",
			wantErr:     sancho.ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, rowErr := parseRow(tt.row, 2, cfg, cols, tt.runCurrency, multiplier, utc)

			// --- Error cases ---
			if tt.wantErr != nil {
				if rowErr == nil {
					t.Errorf("parseRow() expected error %v but got nil", tt.wantErr)
					return
				}
				if !errors.Is(rowErr, tt.wantErr) {
					t.Errorf("parseRow()\n  got error  %v\n  want error %v", rowErr.Err, tt.wantErr)
				}
				if tx != nil {
					t.Errorf("parseRow() expected nil transaction on error but got %+v", tx)
				}
				return
			}

			// --- Success cases ---
			if rowErr != nil {
				t.Errorf("parseRow() unexpected error: %v", rowErr)
				return
			}
			if tx == nil {
				t.Fatal("parseRow() returned nil transaction")
			}

			// Always verify these fields are set correctly.
			if tx.SourceName != cfg.Name {
				t.Errorf("SourceName: got %q want %q", tx.SourceName, cfg.Name)
			}
			if tx.SourceRole != cfg.Role {
				t.Errorf("SourceRole: got %q want %q", tx.SourceRole, cfg.Role)
			}
			if tx.SourceFile != cfg.File {
				t.Errorf("SourceFile: got %q want %q", tx.SourceFile, cfg.File)
			}
			if tx.SourceRow != 2 {
				t.Errorf("SourceRow: got %d want 2", tx.SourceRow)
			}
			if tx.Currency != tt.runCurrency {
				t.Errorf("Currency: got %q want %q", tx.Currency, tt.runCurrency)
			}
			if tx.Timestamp.Location() != time.UTC {
				t.Errorf("Timestamp not UTC: got %v", tx.Timestamp.Location())
			}

			// Verify specific fields when wantTx is fully specified.
			if tt.wantTx != nil {
				if tx.ReferenceID != tt.wantTx.ReferenceID {
					t.Errorf("ReferenceID: got %q want %q", tx.ReferenceID, tt.wantTx.ReferenceID)
				}
				if tx.AmountMinorUnits != tt.wantTx.AmountMinorUnits {
					t.Errorf("AmountMinorUnits: got %d want %d", tx.AmountMinorUnits, tt.wantTx.AmountMinorUnits)
				}
				if tt.wantTx.Timestamp != (time.Time{}) && !tx.Timestamp.Equal(tt.wantTx.Timestamp) {
					t.Errorf("Timestamp: got %v want %v", tx.Timestamp, tt.wantTx.Timestamp)
				}
				if tt.wantTx.RawStatus != "" && tx.RawStatus != tt.wantTx.RawStatus {
					t.Errorf("RawStatus: got %q want %q", tx.RawStatus, tt.wantTx.RawStatus)
				}
			}
		})
	}
}

// TestParseRowTimestampAlwaysUTC verifies the core invariant —
// every transaction timestamp is UTC regardless of source timezone.
func TestParseRowTimestampAlwaysUTC(t *testing.T) {
	lagos, _ := time.LoadLocation("Africa/Lagos")
	cfg := testSourceConfig()
	cols := testColumnPositions()

	row := []string{"ONY_001", "5000.00", "NGN", "2026-06-21 09:00:00", "success"}

	tx, rowErr := parseRow(row, 2, cfg, cols, "NGN", 100, lagos)
	if rowErr != nil {
		t.Fatalf("unexpected error: %v", rowErr)
	}

	if tx.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp not UTC: got %v", tx.Timestamp.Location())
	}

	// 09:00 WAT (UTC+1) should be 08:00 UTC
	wantUTC := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	if !tx.Timestamp.Equal(wantUTC) {
		t.Errorf("Timestamp conversion wrong:\n  got  %v\n  want %v", tx.Timestamp, wantUTC)
	}
}

// TestParseRowNilOnError verifies that parseRow never returns both
// a transaction and an error simultaneously.
func TestParseRowNilOnError(t *testing.T) {
	cfg := testSourceConfig()
	cols := testColumnPositions()

	// Force a currency mismatch error.
	row := []string{"ONY_001", "5000.00", "USD", "2026-06-21 09:00:00", "success"}

	tx, rowErr := parseRow(row, 2, cfg, cols, "NGN", 100, time.UTC)

	if rowErr == nil {
		t.Fatal("expected error but got nil")
	}
	if tx != nil {
		t.Errorf("expected nil transaction on error but got %+v", tx)
	}
}

func TestResolveColumns(t *testing.T) {
	// Standard column index simulating a real CSV header row.
	// Mirrors what buildColumnIndex would produce from:
	// "txn_ref", "amount", "currency", "created_at", "status"
	standardIndex := map[string]int{
		"txn_ref":    0,
		"amount":     1,
		"currency":   2,
		"created_at": 3,
		"status":     4,
	}

	// Paystack-style index — mixed casing already normalized by buildColumnIndex.
	paystackIndex := map[string]int{
		"transaction_id":   0,
		"settled_amount":   1,
		"currency":         2,
		"transaction_date": 3,
		"status":           4,
	}

	tests := []struct {
		name     string
		cfg      domain.SourceConfig
		colIndex map[string]int
		want     columnPositions
		wantErr  error
	}{
		// --- Happy path: all fields mapped ---
		{
			name: "all fields mapped — internal ledger style",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					ReferenceID: "txn_ref",
					Amount:      "amount",
					Currency:    "currency",
					Timestamp:   "created_at",
					Status:      "status",
				},
			},
			colIndex: standardIndex,
			want: columnPositions{
				reference: 0,
				amount:    1,
				currency:  2,
				timestamp: 3,
				status:    4,
			},
		},
		{
			name: "all fields mapped — Paystack style",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					ReferenceID: "Transaction ID",
					Amount:      "Settled Amount",
					Currency:    "Currency",
					Timestamp:   "Transaction Date",
					Status:      "Status",
				},
			},
			colIndex: paystackIndex,
			want: columnPositions{
				reference: 0,
				amount:    1,
				currency:  2,
				timestamp: 3,
				status:    4,
			},
		},

		// --- Optional fields not mapped ---
		{
			name: "reference_id not mapped — returns -1",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					ReferenceID: "",
					Amount:      "amount",
					Currency:    "currency",
					Timestamp:   "created_at",
					Status:      "status",
				},
			},
			colIndex: standardIndex,
			want: columnPositions{
				reference: -1,
				amount:    1,
				currency:  2,
				timestamp: 3,
				status:    4,
			},
		},
		{
			name: "currency not mapped — returns -1",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					ReferenceID: "txn_ref",
					Amount:      "amount",
					Currency:    "",
					Timestamp:   "created_at",
					Status:      "status",
				},
			},
			colIndex: standardIndex,
			want: columnPositions{
				reference: 0,
				amount:    1,
				currency:  -1,
				timestamp: 3,
				status:    4,
			},
		},
		{
			name: "status not mapped — returns -1",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					ReferenceID: "txn_ref",
					Amount:      "amount",
					Currency:    "currency",
					Timestamp:   "created_at",
					Status:      "",
				},
			},
			colIndex: standardIndex,
			want: columnPositions{
				reference: 0,
				amount:    1,
				currency:  2,
				timestamp: 3,
				status:    -1,
			},
		},
		{
			name: "only required fields mapped",
			cfg: domain.SourceConfig{
				Fields: domain.FieldMapping{
					Amount:    "amount",
					Timestamp: "created_at",
				},
			},
			colIndex: standardIndex,
			want: columnPositions{
				reference: -1,
				amount:    1,
				currency:  -1,
				timestamp: 3,
				status:    -1,
			},
		},

		// --- Error cases ---
		{
			name: "amount column not found",
			cfg: domain.SourceConfig{
				Name: "test_source",
				Fields: domain.FieldMapping{
					Amount:    "total_amount", // not in index
					Timestamp: "created_at",
				},
			},
			colIndex: standardIndex,
			wantErr:  sancho.ErrAmountColumnNotFound,
		},
		{
			name: "timestamp column not found",
			cfg: domain.SourceConfig{
				Name: "test_source",
				Fields: domain.FieldMapping{
					Amount:    "amount",
					Timestamp: "transaction_time", // not in index
				},
			},
			colIndex: standardIndex,
			wantErr:  sancho.ErrTimestampColumnNotFound,
		},
		{
			name: "both required columns missing — amount checked first",
			cfg: domain.SourceConfig{
				Name: "test_source",
				Fields: domain.FieldMapping{
					Amount:    "missing_amount",
					Timestamp: "missing_timestamp",
				},
			},
			colIndex: standardIndex,
			wantErr:  sancho.ErrAmountColumnNotFound, // amount is checked first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveColumns(tt.cfg, tt.colIndex)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("resolveColumns() expected error %v but got nil", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("resolveColumns()\n  got error  %v\n  want error %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("resolveColumns() unexpected error: %v", err)
				return
			}

			if got.amount != tt.want.amount {
				t.Errorf("amount col: got %d want %d", got.amount, tt.want.amount)
			}
			if got.timestamp != tt.want.timestamp {
				t.Errorf("timestamp col: got %d want %d", got.timestamp, tt.want.timestamp)
			}
			if got.reference != tt.want.reference {
				t.Errorf("reference col: got %d want %d", got.reference, tt.want.reference)
			}
			if got.currency != tt.want.currency {
				t.Errorf("currency col: got %d want %d", got.currency, tt.want.currency)
			}
			if got.status != tt.want.status {
				t.Errorf("status col: got %d want %d", got.status, tt.want.status)
			}
		})
	}
}

// TestResolveColumnsNormalization verifies that field names in the YAML config
// are normalized before lookup — so "Transaction ID" finds "transaction_id" in the index.
func TestResolveColumnsNormalization(t *testing.T) {
	// Index already has normalized keys (as buildColumnIndex would produce).
	colIndex := map[string]int{
		"transaction_id": 0,
		"amount":         1,
		"created_at":     2,
	}

	variants := []struct {
		name      string
		amountVal string
	}{
		{"exact match", "amount"},
		{"title case", "Amount"},
		{"all caps", "AMOUNT"},
		{"with spaces", "  amount  "},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			cfg := domain.SourceConfig{
				Fields: domain.FieldMapping{
					Amount:    v.amountVal,
					Timestamp: "created_at",
				},
			}

			got, err := resolveColumns(cfg, colIndex)
			if err != nil {
				t.Errorf("resolveColumns() unexpected error for %q: %v", v.amountVal, err)
				return
			}
			if got.amount != 1 {
				t.Errorf("amount col for %q: got %d want 1", v.amountVal, got.amount)
			}
		})
	}
}

// writeTempCSV creates a temporary CSV file with the given content
// and returns its path. The caller is responsible for cleanup.
func writeTempCSV(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mogaji_test_*.csv")
	if err != nil {
		t.Fatalf("failed to create temp CSV: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write temp CSV: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestPrepareSource(t *testing.T) {
	// Standard CSV used across most tests.
	standardCSV := "txn_ref,amount,currency,created_at,status\nONY_001,5000.00,NGN,2026-06-21 09:00:00,success\n"

	tests := []struct {
		name    string
		cfg     func(csvPath string) domain.SourceConfig
		csv     string
		wantErr error
		// Optional assertions on successful result.
		wantMultiplier   int64
		wantRefCol       int
		wantAmountCol    int
		wantTimestampCol int
	}{
		// --- Happy path ---
		{
			name: "valid config and CSV",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						ReferenceID: "txn_ref",
						Amount:      "amount",
						Currency:    "currency",
						Timestamp:   "created_at",
						Status:      "status",
					},
				}
			},
			csv:              standardCSV,
			wantMultiplier:   100,
			wantRefCol:       0,
			wantAmountCol:    1,
			wantTimestampCol: 3,
		},
		{
			name: "Lagos timezone loads correctly",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "Africa/Lagos",
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			csv:            standardCSV,
			wantMultiplier: 100,
		},
		{
			name: "decimal places 0 defaults to 2",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:          "test_source",
					File:          csvPath,
					Timezone:      "UTC",
					DecimalPlaces: 0, // should default to 2
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			csv:            standardCSV,
			wantMultiplier: 100, // 10^2
		},
		{
			name: "decimal places 3 — KWD",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:          "test_source",
					File:          csvPath,
					Timezone:      "UTC",
					DecimalPlaces: 3,
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			csv:            standardCSV,
			wantMultiplier: 1000, // 10^3
		},
		{
			name: "normalized header matching — Paystack style",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "paystack",
					File:     csvPath,
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						ReferenceID: "Transaction ID",
						Amount:      "Settled Amount",
						Currency:    "Currency",
						Timestamp:   "Transaction Date",
						Status:      "Status",
					},
				}
			},
			csv:              "Transaction ID,Settled Amount,Currency,Transaction Date,Status\nONY_001,4925.00,NGN,2026-06-21 09:00:03,success\n",
			wantMultiplier:   100,
			wantRefCol:       0,
			wantAmountCol:    1,
			wantTimestampCol: 3,
		},

		// --- Error: invalid timezone ---
		{
			name: "invalid timezone",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "Not/ATimezone",
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			csv:     standardCSV,
			wantErr: sancho.ErrInvalidTimezone,
		},

		// --- Error: file not found ---
		{
			name: "file not found",
			cfg: func(_ string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     "/nonexistent/path/file.csv",
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			csv:     standardCSV,
			wantErr: sancho.ErrFileNotFound,
		},

		// --- Error: amount column not in headers ---
		{
			name: "amount column not found in headers",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						Amount:    "total_amount", // not in CSV
						Timestamp: "created_at",
					},
				}
			},
			csv:     standardCSV,
			wantErr: sancho.ErrAmountColumnNotFound,
		},

		// --- Error: timestamp column not in headers ---
		{
			name: "timestamp column not found in headers",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "transaction_time", // not in CSV
					},
				}
			},
			csv:     standardCSV,
			wantErr: sancho.ErrTimestampColumnNotFound,
		},

		// --- Error: ambiguous headers in CSV ---
		{
			name: "ambiguous CSV headers",
			cfg: func(csvPath string) domain.SourceConfig {
				return domain.SourceConfig{
					Name:     "test_source",
					File:     csvPath,
					Timezone: "UTC",
					Fields: domain.FieldMapping{
						Amount:    "amount",
						Timestamp: "created_at",
					},
				}
			},
			// "Amount" and "amount" normalize to the same key
			csv:     "txn_ref,Amount,amount,created_at,status\nONY_001,5000.00,5000.00,2026-06-21 09:00:00,success\n",
			wantErr: sancho.ErrAmbiguousHeaders,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write temp CSV (ignored for file-not-found test).
			csvPath := writeTempCSV(t, tt.csv)
			cfg := tt.cfg(csvPath)

			loc, _, cols, multiplier, err := prepareSource(cfg)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("prepareSource() expected error %v but got nil", tt.wantErr)
					return
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("prepareSource()\n  got error  %v\n  want error %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("prepareSource() unexpected error: %v", err)
				return
			}

			// Verify location loaded.
			if loc == nil {
				t.Error("prepareSource() returned nil location")
			}

			// Verify multiplier.
			if tt.wantMultiplier != 0 && multiplier != tt.wantMultiplier {
				t.Errorf("multiplier: got %d want %d", multiplier, tt.wantMultiplier)
			}

			// Verify column positions when specified.
			if tt.wantRefCol != 0 && cols.reference != tt.wantRefCol {
				t.Errorf("reference col: got %d want %d", cols.reference, tt.wantRefCol)
			}
			if tt.wantAmountCol != 0 && cols.amount != tt.wantAmountCol {
				t.Errorf("amount col: got %d want %d", cols.amount, tt.wantAmountCol)
			}
			if tt.wantTimestampCol != 0 && cols.timestamp != tt.wantTimestampCol {
				t.Errorf("timestamp col: got %d want %d", cols.timestamp, tt.wantTimestampCol)
			}
		})
	}
}

// TestPrepareSourceDefaultDecimalPlaces verifies that decimal_places: 0
// in config always defaults to 2, never produces a multiplier of 1.
func TestPrepareSourceDefaultDecimalPlaces(t *testing.T) {
	csvPath := writeTempCSV(t, "amount,created_at\n5000.00,2026-06-21 09:00:00\n")

	cfg := domain.SourceConfig{
		Name:          "test",
		File:          csvPath,
		Timezone:      "UTC",
		DecimalPlaces: 0,
		Fields: domain.FieldMapping{
			Amount:    "amount",
			Timestamp: "created_at",
		},
	}

	_, _, _, multiplier, err := prepareSource(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if multiplier == 1 {
		t.Error("multiplier should not be 1 when decimal_places is 0 — default of 2 should apply")
	}
	if multiplier != 100 {
		t.Errorf("multiplier: got %d want 100", multiplier)
	}
}

// baseSourceConfig returns a minimal valid SourceConfig pointing to a given file.
func baseSourceConfig(file string) domain.SourceConfig {
	return domain.SourceConfig{
		Name:     "test_source",
		File:     file,
		Role:     domain.SourceRoleInternal,
		Timezone: "UTC",
		Fields: domain.FieldMapping{
			ReferenceID: "txn_ref",
			Amount:      "amount",
			Currency:    "currency",
			Timestamp:   "created_at",
			Status:      "status",
		},
	}
}

// standardCSVContent is a clean 3-row CSV used across multiple tests.
const standardCSVContent = `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,2500.00,NGN,2026-06-21 09:05:00,success
ONY_003,10000.00,NGN,2026-06-21 09:10:00,success
`

// ── LoadCSV ──────────────────────────────────────────────────────────────────

func TestLoadCSV(t *testing.T) {
	t.Run("valid CSV loads all rows", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 3 {
			t.Errorf("got %d transactions want 3", len(result.Transactions))
		}
		if len(result.Errors) != 0 {
			t.Errorf("got %d errors want 0", len(result.Errors))
		}
	})

	t.Run("transactions have correct values", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tx := result.Transactions[0]
		if tx.ReferenceID != "ONY_001" {
			t.Errorf("ReferenceID: got %q want ONY_001", tx.ReferenceID)
		}
		if tx.AmountMinorUnits != 500000 {
			t.Errorf("AmountMinorUnits: got %d want 500000", tx.AmountMinorUnits)
		}
		if tx.Currency != "NGN" {
			t.Errorf("Currency: got %q want NGN", tx.Currency)
		}
		if tx.Timestamp.Location() != time.UTC {
			t.Errorf("Timestamp not UTC: %v", tx.Timestamp.Location())
		}
		if tx.RawStatus != "success" {
			t.Errorf("RawStatus: got %q want success", tx.RawStatus)
		}
		if tx.SourceRow != 2 {
			t.Errorf("SourceRow: got %d want 2", tx.SourceRow)
		}
	})

	t.Run("meta counts are correct", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Meta.TotalLoaded != 3 {
			t.Errorf("TotalLoaded: got %d want 3", result.Meta.TotalLoaded)
		}
		if result.Meta.TotalAfterDedup != 3 {
			t.Errorf("TotalAfterDedup: got %d want 3", result.Meta.TotalAfterDedup)
		}
		if result.Meta.DuplicatesSkipped != 0 {
			t.Errorf("DuplicatesSkipped: got %d want 0", result.Meta.DuplicatesSkipped)
		}
		if result.Meta.Name != "test_source" {
			t.Errorf("Meta.Name: got %q want test_source", result.Meta.Name)
		}
		if result.Meta.FilePath != path {
			t.Errorf("Meta.FilePath: got %q want %q", result.Meta.FilePath, path)
		}
	})

	t.Run("empty CSV — header only", func(t *testing.T) {
		path := writeTempCSV(t, "txn_ref,amount,currency,created_at,status\n")
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Transactions) != 0 {
			t.Errorf("got %d transactions want 0", len(result.Transactions))
		}
		if result.Meta.TotalLoaded != 0 {
			t.Errorf("TotalLoaded: got %d want 0", result.Meta.TotalLoaded)
		}
	})

	t.Run("duplicate reference — first wins, second goes to errors", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,2500.00,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 2 {
			t.Errorf("got %d transactions want 2", len(result.Transactions))
		}
		if result.Meta.DuplicatesSkipped != 1 {
			t.Errorf("DuplicatesSkipped: got %d want 1", result.Meta.DuplicatesSkipped)
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrDuplicateReference) {
			t.Errorf("error type: got %v want ErrDuplicateReference", result.Errors[0].Err)
		}
		// First occurrence is kept — reference_id ONY_001 at row 2.
		if result.Transactions[0].ReferenceID != "ONY_001" {
			t.Errorf("first transaction should be ONY_001")
		}
		if result.Transactions[0].SourceRow != 2 {
			t.Errorf("first ONY_001 should be from row 2, got row %d", result.Transactions[0].SourceRow)
		}
	})

	t.Run("blank reference IDs not deduplicated", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
,5000.00,NGN,2026-06-21 09:00:00,success
,2500.00,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Both blank-reference rows pass through — dedup only fires on non-empty references.
		if len(result.Transactions) != 2 {
			t.Errorf("got %d transactions want 2 — blank references should not be deduplicated", len(result.Transactions))
		}
		if result.Meta.DuplicatesSkipped != 0 {
			t.Errorf("DuplicatesSkipped: got %d want 0", result.Meta.DuplicatesSkipped)
		}
	})

	t.Run("currency mismatch row skipped", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,5000.00,USD,2026-06-21 09:05:00,success
ONY_003,2500.00,NGN,2026-06-21 09:10:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 2 {
			t.Errorf("got %d transactions want 2", len(result.Transactions))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrCurrencyMismatch) {
			t.Errorf("error type: got %v want ErrCurrencyMismatch", result.Errors[0].Err)
		}
	})

	t.Run("invalid amount row skipped", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,not-a-number,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 1 {
			t.Errorf("got %d transactions want 1", len(result.Transactions))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrInvalidAmount) {
			t.Errorf("error type: got %v want ErrInvalidAmount", result.Errors[0].Err)
		}
	})

	t.Run("invalid timestamp row skipped", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,not-a-date,success
ONY_002,2500.00,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 1 {
			t.Errorf("got %d transactions want 1", len(result.Transactions))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrInvalidTimestamp) {
			t.Errorf("error type: got %v want ErrInvalidTimestamp", result.Errors[0].Err)
		}
	})

	t.Run("multiple bad rows — all collected, good rows loaded", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,bad-amount,NGN,2026-06-21 09:05:00,success
ONY_003,2500.00,USD,2026-06-21 09:10:00,success
ONY_004,2500.00,NGN,bad-date,success
ONY_005,1000.00,NGN,2026-06-21 09:20:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Transactions) != 2 {
			t.Errorf("got %d transactions want 2 (ONY_001 and ONY_005)", len(result.Transactions))
		}
		if len(result.Errors) != 3 {
			t.Errorf("got %d errors want 3", len(result.Errors))
		}
		if result.Meta.TotalLoaded != 5 {
			t.Errorf("TotalLoaded: got %d want 5", result.Meta.TotalLoaded)
		}
	})

	t.Run("row errors carry correct row number", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,bad-amount,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, err := LoadCSV(cfg, "NGN")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		// Header is row 1, first data row is row 2, bad row is row 3.
		if result.Errors[0].RowNumber != 3 {
			t.Errorf("RowNumber: got %d want 3", result.Errors[0].RowNumber)
		}
	})
}

// ── StreamCSV ─────────────────────────────────────────────────────────────────

func TestStreamCSV(t *testing.T) {
	t.Run("valid CSV streams all rows to callback", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		var received []*domain.Transaction
		result, err := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			received = append(received, tx)
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(received) != 3 {
			t.Errorf("callback received %d transactions want 3", len(received))
		}
		if len(result.Errors) != 0 {
			t.Errorf("got %d errors want 0", len(result.Errors))
		}
	})

	t.Run("transactions streamed in order", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		var refs []string
		StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			refs = append(refs, tx.ReferenceID)
			return nil
		})

		want := []string{"ONY_001", "ONY_002", "ONY_003"}
		for i, ref := range refs {
			if ref != want[i] {
				t.Errorf("position %d: got %q want %q", i, ref, want[i])
			}
		}
	})

	t.Run("callback error stops streaming immediately", func(t *testing.T) {
		path := writeTempCSV(t, standardCSVContent)
		cfg := baseSourceConfig(path)

		callCount := 0
		stopErr := fmt.Errorf("stop now")

		_, err := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			callCount++
			if callCount == 2 {
				return stopErr
			}
			return nil
		})

		if err == nil {
			t.Error("expected error when callback returns error but got nil")
		}
		if callCount != 2 {
			t.Errorf("callback called %d times want 2 — should stop after error", callCount)
		}
	})

	t.Run("bad rows skipped — callback never called for them", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,bad-amount,NGN,2026-06-21 09:05:00,success
ONY_003,2500.00,NGN,2026-06-21 09:10:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		var received []*domain.Transaction
		result, err := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			received = append(received, tx)
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(received) != 2 {
			t.Errorf("callback received %d transactions want 2", len(received))
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrInvalidAmount) {
			t.Errorf("error type: got %v want ErrInvalidAmount", result.Errors[0].Err)
		}
	})

	t.Run("duplicate reference — first streamed, second goes to errors", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,2500.00,NGN,2026-06-21 09:05:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		var received []*domain.Transaction
		result, err := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			received = append(received, tx)
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(received) != 2 {
			t.Errorf("callback received %d transactions want 2", len(received))
		}
		if result.Meta.DuplicatesSkipped != 1 {
			t.Errorf("DuplicatesSkipped: got %d want 1", result.Meta.DuplicatesSkipped)
		}
		if len(result.Errors) != 1 {
			t.Fatalf("got %d errors want 1", len(result.Errors))
		}
		if !errors.Is(&result.Errors[0], sancho.ErrDuplicateReference) {
			t.Errorf("error type: got %v want ErrDuplicateReference", result.Errors[0].Err)
		}
	})

	t.Run("meta counts correct", func(t *testing.T) {
		csv := `txn_ref,amount,currency,created_at,status
ONY_001,5000.00,NGN,2026-06-21 09:00:00,success
ONY_002,bad-amount,NGN,2026-06-21 09:05:00,success
ONY_003,5000.00,NGN,2026-06-21 09:10:00,success
ONY_003,5000.00,NGN,2026-06-21 09:10:00,success
`
		path := writeTempCSV(t, csv)
		cfg := baseSourceConfig(path)

		result, _ := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error { return nil })

		// 4 rows loaded, 1 bad amount, 1 duplicate = 2 good
		if result.Meta.TotalLoaded != 4 {
			t.Errorf("TotalLoaded: got %d want 4", result.Meta.TotalLoaded)
		}
		if result.Meta.DuplicatesSkipped != 1 {
			t.Errorf("DuplicatesSkipped: got %d want 1", result.Meta.DuplicatesSkipped)
		}
		if result.Meta.TotalAfterDedup != 1 {
			t.Errorf("TotalAfterDedup: got %d want 1", result.Meta.TotalAfterDedup)
		}
	})

	t.Run("empty CSV — callback never called", func(t *testing.T) {
		path := writeTempCSV(t, "txn_ref,amount,currency,created_at,status\n")
		cfg := baseSourceConfig(path)

		callCount := 0
		result, err := StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
			callCount++
			return nil
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 0 {
			t.Errorf("callback called %d times want 0", callCount)
		}
		if result.Meta.TotalLoaded != 0 {
			t.Errorf("TotalLoaded: got %d want 0", result.Meta.TotalLoaded)
		}
	})
}

// ── Shared invariants ─────────────────────────────────────────────────────────

// TestLoadCSVAndStreamCSVProduceSameTransactions verifies that LoadCSV and
// StreamCSV produce identical transactions for the same input.
// This is the most important consistency invariant between the two functions.
func TestLoadCSVAndStreamCSVProduceSameTransactions(t *testing.T) {
	path := writeTempCSV(t, standardCSVContent)
	cfg := baseSourceConfig(path)

	loadResult, err := LoadCSV(cfg, "NGN")
	if err != nil {
		t.Fatalf("LoadCSV unexpected error: %v", err)
	}

	var streamed []*domain.Transaction
	_, err = StreamCSV(cfg, "NGN", func(tx *domain.Transaction) error {
		streamed = append(streamed, tx)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamCSV unexpected error: %v", err)
	}

	if len(loadResult.Transactions) != len(streamed) {
		t.Fatalf("length mismatch: LoadCSV=%d StreamCSV=%d",
			len(loadResult.Transactions), len(streamed))
	}

	for i, loaded := range loadResult.Transactions {
		s := streamed[i]
		if loaded.ReferenceID != s.ReferenceID {
			t.Errorf("[%d] ReferenceID: LoadCSV=%q StreamCSV=%q", i, loaded.ReferenceID, s.ReferenceID)
		}
		if loaded.AmountMinorUnits != s.AmountMinorUnits {
			t.Errorf("[%d] AmountMinorUnits: LoadCSV=%d StreamCSV=%d", i, loaded.AmountMinorUnits, s.AmountMinorUnits)
		}
		if !loaded.Timestamp.Equal(s.Timestamp) {
			t.Errorf("[%d] Timestamp: LoadCSV=%v StreamCSV=%v", i, loaded.Timestamp, s.Timestamp)
		}
		if loaded.Currency != s.Currency {
			t.Errorf("[%d] Currency: LoadCSV=%q StreamCSV=%q", i, loaded.Currency, s.Currency)
		}
		if loaded.SourceRow != s.SourceRow {
			t.Errorf("[%d] SourceRow: LoadCSV=%d StreamCSV=%d", i, loaded.SourceRow, s.SourceRow)
		}
	}
}
