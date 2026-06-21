package ingest

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
)

// RowError records a single row that failed to parse during ingestion.
// Rows with errors are skipped — they never enter the transaction pool.
// All errors are collected and surfaced in the final report.
type RowError struct {
	// SourceName is the name of the source this row came from.
	SourceName string `json:"source_name"`

	// SourceFile is the file path of the CSV.
	SourceFile string `json:"source_file"`

	// RowNumber is the 1-indexed row number in the CSV (including header).
	RowNumber int `json:"row_number"`

	// Reason describes why this row failed to parse.
	Reason string `json:"reason"`

	// RawRow is the raw CSV values for this row, preserved for debugging.
	RawRow []string `json:"raw_row"`
}

// IngestResult is the output of ingesting a single source CSV.
type IngestResult struct {
	// Transactions is the list of successfully parsed and normalized transactions.
	Transactions []*domain.Transaction

	// Meta is the populated SourceMeta for this source, including dedup counts.
	Meta domain.SourceMeta

	// Errors is the list of rows that failed to parse.
	// Never nil — empty slice when all rows parsed successfully.
	Errors []RowError
}

// LoadCSV reads a CSV file for the given SourceConfig, normalizes every row
// into a canonical Transaction, deduplicates by ReferenceID where possible,
// and returns an IngestResult.
//
// Bad rows are skipped and recorded in IngestResult.Errors.
// The run currency is enforced — rows with a mismatched currency are skipped.
// Duplicate rows (same ReferenceID) are skipped after the first occurrence.
func LoadCSV(cfg domain.SourceConfig, runCurrency string) (IngestResult, error) {
	result := IngestResult{
		Errors: []RowError{},
	}

	// Load the timezone declared for this source.
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return result, fmt.Errorf("source %q: invalid timezone %q: %w", cfg.Name, cfg.Timezone, err)
	}

	// Open the CSV file.
	f, err := os.Open(cfg.File)
	if err != nil {
		return result, fmt.Errorf("source %q: cannot open file %q: %w", cfg.Name, cfg.File, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	// Read the header row to build a normalized column index.
	headers, err := reader.Read()
	if err != nil {
		return result, fmt.Errorf("source %q: cannot read header row: %w", cfg.Name, err)
	}

	colIndex, err := buildColumnIndex(headers)
	if err != nil {
		return result, fmt.Errorf("source %q: %w", cfg.Name, err)
	}

	// Resolve required column positions from the normalized field mapping.
	amountCol, ok := colIndex[normalizeHeader(cfg.Fields.Amount)]
	if !ok {
		return result, fmt.Errorf("source %q: amount column %q (normalized: %q) not found in CSV headers", cfg.Name, cfg.Fields.Amount, normalizeHeader(cfg.Fields.Amount))
	}

	timestampCol, ok := colIndex[normalizeHeader(cfg.Fields.Timestamp)]
	if !ok {
		return result, fmt.Errorf("source %q: timestamp column %q (normalized: %q) not found in CSV headers", cfg.Name, cfg.Fields.Timestamp, normalizeHeader(cfg.Fields.Timestamp))
	}

	// Optional columns — -1 if not mapped or not found in headers.
	referenceCol := optionalCol(colIndex, cfg.Fields.ReferenceID)
	currencyCol := optionalCol(colIndex, cfg.Fields.Currency)
	statusCol := optionalCol(colIndex, cfg.Fields.Status)

	// Precompute the minor unit multiplier for amount conversion.
	// Only used when MinorUnits is false.
	decimalPlaces := cfg.DecimalPlaces
	if decimalPlaces == 0 {
		decimalPlaces = 2 // safe default for NGN, USD, KES
	}
	multiplier := int64(math.Pow10(decimalPlaces))

	// seen tracks ReferenceIDs already ingested from this source for deduplication.
	// Only populated when a row has a non-empty ReferenceID.
	seen := make(map[string]int) // ReferenceID → first SourceRow

	rowNumber := 1 // header was row 1
	totalLoaded := 0
	duplicatesSkipped := 0

	for {
		rowNumber++
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     fmt.Sprintf("CSV read error: %s", err.Error()),
				RawRow:     nil,
			})
			continue
		}

		totalLoaded++

		// --- ReferenceID ---
		referenceID := ""
		if referenceCol >= 0 && referenceCol < len(row) {
			referenceID = strings.TrimSpace(row[referenceCol])
		}

		// --- Deduplication ---
		// Only deduplicate when we have a non-empty ReferenceID.
		// Blank-reference rows pass through — the engine handles them via weak-key matching.
		if referenceID != "" {
			if firstRow, exists := seen[referenceID]; exists {
				result.Errors = append(result.Errors, RowError{
					SourceName: cfg.Name,
					SourceFile: cfg.File,
					RowNumber:  rowNumber,
					Reason:     fmt.Sprintf("duplicate reference_id %q — first seen at row %d, skipping", referenceID, firstRow),
					RawRow:     row,
				})
				duplicatesSkipped++
				continue
			}
			seen[referenceID] = rowNumber
		}

		// --- Currency ---
		currency := runCurrency // default to run-level currency
		if currencyCol >= 0 && currencyCol < len(row) {
			declared := strings.TrimSpace(strings.ToUpper(row[currencyCol]))
			if declared != "" {
				currency = declared
			}
		}

		// Enforce run currency.
		if currency != runCurrency {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     fmt.Sprintf("currency mismatch: row has %q but run enforces %q", currency, runCurrency),
				RawRow:     row,
			})
			continue
		}

		// --- Amount ---
		if amountCol >= len(row) {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     "row has fewer columns than expected — amount column missing",
				RawRow:     row,
			})
			continue
		}

		amountMinorUnits, err := parseAmount(strings.TrimSpace(row[amountCol]), cfg.MinorUnits, multiplier)
		if err != nil {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     fmt.Sprintf("invalid amount %q: %s", row[amountCol], err.Error()),
				RawRow:     row,
			})
			continue
		}

		// --- Timestamp ---
		if timestampCol >= len(row) {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     "row has fewer columns than expected — timestamp column missing",
				RawRow:     row,
			})
			continue
		}

		timestamp, err := parseTimestamp(strings.TrimSpace(row[timestampCol]), loc)
		if err != nil {
			result.Errors = append(result.Errors, RowError{
				SourceName: cfg.Name,
				SourceFile: cfg.File,
				RowNumber:  rowNumber,
				Reason:     fmt.Sprintf("invalid timestamp %q: %s", row[timestampCol], err.Error()),
				RawRow:     row,
			})
			continue
		}

		// --- Status ---
		rawStatus := ""
		if statusCol >= 0 && statusCol < len(row) {
			rawStatus = strings.TrimSpace(row[statusCol])
		}

		// --- Build canonical Transaction ---
		tx := &domain.Transaction{
			ReferenceID:      referenceID,
			SourceName:       cfg.Name,
			SourceRole:       cfg.Role,
			AmountMinorUnits: amountMinorUnits,
			Currency:         currency,
			Timestamp:        timestamp,
			RawStatus:        rawStatus,
			SourceFile:       cfg.File,
			SourceRow:        rowNumber,
		}

		result.Transactions = append(result.Transactions, tx)
	}

	result.Meta = domain.SourceMeta{
		Name:              cfg.Name,
		Role:              cfg.Role,
		FilePath:          cfg.File,
		TotalLoaded:       totalLoaded,
		TotalAfterDedup:   len(result.Transactions),
		DuplicatesSkipped: duplicatesSkipped,
	}

	return result, nil
}

// optionalCol looks up a field mapping value in the column index.
// Returns -1 if the field is empty (not mapped) or not found in the headers.
// Callers must check for -1 before using the returned index.
func optionalCol(colIndex map[string]int, field string) int {
	if field == "" {
		return -1
	}
	idx, ok := colIndex[normalizeHeader(field)]
	if !ok {
		return -1
	}
	return idx
}

// buildColumnIndex maps normalized header names to their column index.
// Normalization makes matching resilient to casing, spacing, and separator
// inconsistencies common in Nigerian PSP exports and bank statements.
//
// Returns an error if two headers normalize to the same string — this would
// cause a silent wrong-column read and must be caught early.
func buildColumnIndex(headers []string) (map[string]int, error) {
	idx := make(map[string]int, len(headers))
	seen := make(map[string]string) // normalized → original, for error messages

	for i, h := range headers {
		norm := normalizeHeader(h)
		if norm == "" {
			continue // skip blank headers
		}
		if original, exists := seen[norm]; exists {
			return nil, fmt.Errorf(
				"ambiguous CSV headers: %q and %q both normalize to %q — rename one column",
				original, h, norm,
			)
		}
		seen[norm] = h
		idx[norm] = i
	}

	return idx, nil
}

// normalizeHeader converts a CSV column header into a canonical lookup key.
//
// Rules applied in order:
//  1. Trim leading/trailing whitespace
//  2. Lowercase everything
//  3. Replace spaces, hyphens, dots, slashes with underscores
//  4. Strip any remaining non-alphanumeric, non-underscore characters
//  5. Collapse consecutive underscores into one
//  6. Trim leading/trailing underscores
//
// Examples:
//
//	"Transaction ID"  → "transaction_id"
//	"transaction-id"  → "transaction_id"
//	"TRANSACTION ID"  → "transaction_id"
//	"settled_amount"  → "settled_amount"
//	"Settled Amount"  → "settled_amount"
//	"Date/Time"       → "date_time"
//	"amount (NGN)"    → "amount_ngn"
func normalizeHeader(h string) string {
	h = strings.TrimSpace(h)
	h = strings.ToLower(h)

	// Replace common separators with underscore.
	h = strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		"/", "_",
	).Replace(h)

	// Strip characters that are not alphanumeric or underscore.
	var b strings.Builder
	for _, r := range h {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	h = b.String()

	// Collapse consecutive underscores.
	for strings.Contains(h, "__") {
		h = strings.ReplaceAll(h, "__", "_")
	}

	// Trim leading/trailing underscores that may result from stripping.
	h = strings.Trim(h, "_")

	return h
}

// parseAmount converts a raw string amount into minor units (e.g. kobo, cents).
// If minorUnits is true, the value is parsed as an integer directly.
// If minorUnits is false, the value is parsed as a decimal and multiplied
// by the multiplier (10^decimalPlaces) to produce minor units.
func parseAmount(raw string, minorUnits bool, multiplier int64) (int64, error) {
	// Strip common formatting characters.
	raw = strings.ReplaceAll(raw, ",", "")
	raw = strings.ReplaceAll(raw, " ", "")

	if minorUnits {
		val, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("expected integer minor units, got %q", raw)
		}
		if val < 0 {
			return 0, fmt.Errorf("negative amounts are not supported, got %d", val)
		}
		return val, nil
	}

	// Parse as float, then convert to minor units via multiplier.
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("expected decimal amount, got %q", raw)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative amounts are not supported, got %f", val)
	}

	// Round to nearest integer after scaling to avoid floating-point drift.
	return int64(math.Round(val * float64(multiplier))), nil
}

// parseTimestamp attempts to parse a datetime string using a list of common formats.
// The parsed time is returned in UTC regardless of the source timezone.
// The source location is applied during parsing so the conversion is correct.
func parseTimestamp(raw string, loc *time.Location) (time.Time, error) {
	// Common formats seen in Nigerian PSP exports, bank statements, and internal systems.
	formats := []string{
		"2006-01-02T15:04:05Z07:00", // ISO 8601 with timezone
		"2006-01-02T15:04:05",       // ISO 8601 no timezone (source tz applied)
		"2006-01-02 15:04:05",       // common DB export format
		"2006-01-02 15:04:05 -0700", // with numeric offset
		"02/01/2006 15:04:05",       // DD/MM/YYYY common in Nigerian bank exports
		"01/02/2006 15:04:05",       // MM/DD/YYYY
		"2006-01-02",                // date only
		"02/01/2006",                // date only DD/MM/YYYY
	}

	for _, format := range formats {
		t, err := time.ParseInLocation(format, raw, loc)
		if err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognised datetime format %q — supported formats: ISO 8601, YYYY-MM-DD HH:MM:SS, DD/MM/YYYY HH:MM:SS", raw)
}
