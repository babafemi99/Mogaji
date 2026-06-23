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
	SourceName string   `json:"source_name"`
	SourceFile string   `json:"source_file"`
	RowNumber  int      `json:"row_number"`
	Reason     string   `json:"reason"`
	RawRow     []string `json:"raw_row"`
}

// IngestResult is the output of a LoadCSV call.
// All transactions are held in memory — use for the side being indexed.
type IngestResult struct {
	Transactions []*domain.Transaction
	Meta         domain.SourceMeta
	Errors       []RowError
}

// StreamResult is the output of a StreamCSV call.
// No transactions are held — they were handed off to the callback one by one.
type StreamResult struct {
	Meta   domain.SourceMeta
	Errors []RowError
}

// columnPositions holds resolved column indices for a source.
// Built once from the column index, reused for every row.
type columnPositions struct {
	amount    int
	timestamp int
	reference int // -1 if not mapped
	currency  int // -1 if not mapped
	status    int // -1 if not mapped
}

// LoadCSV reads an entire CSV into memory as []*Transaction.
// Use this for the side being indexed (typically the smaller/external source).
// For large files, use StreamCSV instead.
func LoadCSV(cfg domain.SourceConfig, runCurrency string) (IngestResult, error) {
	result := IngestResult{Errors: []RowError{}}

	loc, colIndex, cols, multiplier, err := prepareSource(cfg)
	if err != nil {
		return result, err
	}

	f, err := os.Open(cfg.File)
	if err != nil {
		return result, fmt.Errorf("source %q: cannot open file %q: %w", cfg.Name, cfg.File, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	if _, err := reader.Read(); err != nil { // skip header — already parsed in prepareSource
		return result, fmt.Errorf("source %q: cannot skip header row: %w", cfg.Name, err)
	}
	_ = colIndex // used inside prepareSource

	seen := make(map[string]int)
	rowNumber := 1
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
				SourceName: cfg.Name, SourceFile: cfg.File,
				RowNumber: rowNumber,
				Reason:    fmt.Sprintf("CSV read error: %s", err.Error()),
			})
			continue
		}
		totalLoaded++

		tx, rowErr := parseRow(row, rowNumber, cfg, cols, runCurrency, multiplier, loc)
		if rowErr != nil {
			result.Errors = append(result.Errors, *rowErr)
			continue
		}

		if tx.ReferenceID != "" {
			if firstRow, exists := seen[tx.ReferenceID]; exists {
				result.Errors = append(result.Errors, RowError{
					SourceName: cfg.Name, SourceFile: cfg.File,
					RowNumber: rowNumber,
					Reason:    fmt.Sprintf("duplicate reference_id %q — first seen at row %d, skipping", tx.ReferenceID, firstRow),
					RawRow:    row,
				})
				duplicatesSkipped++
				continue
			}
			seen[tx.ReferenceID] = rowNumber
		}

		result.Transactions = append(result.Transactions, tx)
	}

	result.Meta = domain.SourceMeta{
		Name: cfg.Name, Role: cfg.Role, FilePath: cfg.File,
		TotalLoaded:       totalLoaded,
		TotalAfterDedup:   len(result.Transactions),
		DuplicatesSkipped: duplicatesSkipped,
	}

	return result, nil
}

// StreamCSV reads a CSV file row by row, passing each valid Transaction
// immediately to fn. Never holds more than one transaction in memory at a time.
//
// Use this for the larger side of a reconciliation run (typically internal ledger).
// If fn returns an error, streaming stops immediately.
// Row-level parse errors are collected and do not stop streaming.
func StreamCSV(cfg domain.SourceConfig, runCurrency string, fn func(*domain.Transaction) error) (StreamResult, error) {
	result := StreamResult{Errors: []RowError{}}

	loc, _, cols, multiplier, err := prepareSource(cfg)
	if err != nil {
		return result, err
	}

	f, err := os.Open(cfg.File)
	if err != nil {
		return result, fmt.Errorf("source %q: cannot open file %q: %w", cfg.Name, cfg.File, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	if _, err := reader.Read(); err != nil { // skip header
		return result, fmt.Errorf("source %q: cannot skip header row: %w", cfg.Name, err)
	}

	seen := make(map[string]int)
	rowNumber := 1
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
				SourceName: cfg.Name, SourceFile: cfg.File,
				RowNumber: rowNumber,
				Reason:    fmt.Sprintf("CSV read error: %s", err.Error()),
			})
			continue
		}
		totalLoaded++

		tx, rowErr := parseRow(row, rowNumber, cfg, cols, runCurrency, multiplier, loc)
		if rowErr != nil {
			result.Errors = append(result.Errors, *rowErr)
			continue
		}

		if tx.ReferenceID != "" {
			if firstRow, exists := seen[tx.ReferenceID]; exists {
				result.Errors = append(result.Errors, RowError{
					SourceName: cfg.Name, SourceFile: cfg.File,
					RowNumber: rowNumber,
					Reason:    fmt.Sprintf("duplicate reference_id %q — first seen at row %d, skipping", tx.ReferenceID, firstRow),
					RawRow:    row,
				})
				duplicatesSkipped++
				continue
			}
			seen[tx.ReferenceID] = rowNumber
		}

		// Hand off immediately — caller owns the transaction from here.
		if err := fn(tx); err != nil {
			return result, fmt.Errorf("source %q: row %d: callback error: %w", cfg.Name, rowNumber, err)
		}
	}

	result.Meta = domain.SourceMeta{
		Name: cfg.Name, Role: cfg.Role, FilePath: cfg.File,
		TotalLoaded:       totalLoaded,
		TotalAfterDedup:   totalLoaded - duplicatesSkipped - len(result.Errors),
		DuplicatesSkipped: duplicatesSkipped,
	}

	return result, nil
}

// prepareSource opens the CSV, reads the header, builds the column index,
// resolves column positions, loads the timezone, and computes the amount multiplier.
// Shared setup between LoadCSV and StreamCSV.
func prepareSource(cfg domain.SourceConfig) (*time.Location, map[string]int, columnPositions, int64, error) {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, nil, columnPositions{}, 0, fmt.Errorf("source %q: invalid timezone %q: %w", cfg.Name, cfg.Timezone, err)
	}

	f, err := os.Open(cfg.File)
	if err != nil {
		return nil, nil, columnPositions{}, 0, fmt.Errorf("source %q: cannot open file %q: %w", cfg.Name, cfg.File, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, nil, columnPositions{}, 0, fmt.Errorf("source %q: cannot read header row: %w", cfg.Name, err)
	}

	colIndex, err := buildColumnIndex(headers)
	if err != nil {
		return nil, nil, columnPositions{}, 0, fmt.Errorf("source %q: %w", cfg.Name, err)
	}

	cols, err := resolveColumns(cfg, colIndex)
	if err != nil {
		return nil, nil, columnPositions{}, 0, fmt.Errorf("source %q: %w", cfg.Name, err)
	}

	decimalPlaces := cfg.DecimalPlaces
	if decimalPlaces == 0 {
		decimalPlaces = 2
	}
	multiplier := int64(math.Pow10(decimalPlaces))

	return loc, colIndex, cols, multiplier, nil
}

// resolveColumns resolves all column positions from the config and column index.
func resolveColumns(cfg domain.SourceConfig, colIndex map[string]int) (columnPositions, error) {
	amountCol, ok := colIndex[normalizeHeader(cfg.Fields.Amount)]
	if !ok {
		return columnPositions{}, fmt.Errorf("amount column %q (normalized: %q) not found in CSV headers", cfg.Fields.Amount, normalizeHeader(cfg.Fields.Amount))
	}

	timestampCol, ok := colIndex[normalizeHeader(cfg.Fields.Timestamp)]
	if !ok {
		return columnPositions{}, fmt.Errorf("timestamp column %q (normalized: %q) not found in CSV headers", cfg.Fields.Timestamp, normalizeHeader(cfg.Fields.Timestamp))
	}

	return columnPositions{
		amount:    amountCol,
		timestamp: timestampCol,
		reference: optionalCol(colIndex, cfg.Fields.ReferenceID),
		currency:  optionalCol(colIndex, cfg.Fields.Currency),
		status:    optionalCol(colIndex, cfg.Fields.Status),
	}, nil
}

// parseRow parses a single CSV row into a canonical Transaction.
// Returns a RowError if the row cannot be parsed.
func parseRow(
	row []string,
	rowNumber int,
	cfg domain.SourceConfig,
	cols columnPositions,
	runCurrency string,
	multiplier int64,
	loc *time.Location,
) (*domain.Transaction, *RowError) {
	rowErr := func(reason string) *RowError {
		return &RowError{
			SourceName: cfg.Name, SourceFile: cfg.File,
			RowNumber: rowNumber, Reason: reason, RawRow: row,
		}
	}

	referenceID := ""
	if cols.reference >= 0 && cols.reference < len(row) {
		referenceID = strings.TrimSpace(row[cols.reference])
	}

	currency := runCurrency
	if cols.currency >= 0 && cols.currency < len(row) {
		if declared := strings.TrimSpace(strings.ToUpper(row[cols.currency])); declared != "" {
			currency = declared
		}
	}
	if currency != runCurrency {
		return nil, rowErr(fmt.Sprintf("currency mismatch: row has %q but run enforces %q", currency, runCurrency))
	}

	if cols.amount >= len(row) {
		return nil, rowErr("row has fewer columns than expected — amount column missing")
	}
	amountMinorUnits, err := parseAmount(strings.TrimSpace(row[cols.amount]), cfg.MinorUnits, multiplier)
	if err != nil {
		return nil, rowErr(fmt.Sprintf("invalid amount %q: %s", row[cols.amount], err.Error()))
	}

	if cols.timestamp >= len(row) {
		return nil, rowErr("row has fewer columns than expected — timestamp column missing")
	}
	timestamp, err := parseTimestamp(strings.TrimSpace(row[cols.timestamp]), loc)
	if err != nil {
		return nil, rowErr(fmt.Sprintf("invalid timestamp %q: %s", row[cols.timestamp], err.Error()))
	}

	rawStatus := ""
	if cols.status >= 0 && cols.status < len(row) {
		rawStatus = strings.TrimSpace(row[cols.status])
	}

	return &domain.Transaction{
		ReferenceID:      referenceID,
		SourceName:       cfg.Name,
		SourceRole:       cfg.Role,
		AmountMinorUnits: amountMinorUnits,
		Currency:         currency,
		Timestamp:        timestamp,
		RawStatus:        rawStatus,
		SourceFile:       cfg.File,
		SourceRow:        rowNumber,
	}, nil
}

// optionalCol looks up a field mapping value in the column index.
// Returns -1 if the field is empty or not found in headers.
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
// Returns an error if two headers normalize to the same string.
func buildColumnIndex(headers []string) (map[string]int, error) {
	idx := make(map[string]int, len(headers))
	seen := make(map[string]string)

	for i, h := range headers {
		norm := normalizeHeader(h)
		if norm == "" {
			continue
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
// Examples:
//
//	"Transaction ID"  → "transaction_id"
//	"transaction-id"  → "transaction_id"
//	"TRANSACTION ID"  → "transaction_id"
//	"Date/Time"       → "date_time"
//	"amount (NGN)"    → "amount_ngn"
func normalizeHeader(h string) string {
	h = strings.TrimSpace(h)
	h = strings.ToLower(h)
	h = strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_").Replace(h)

	var b strings.Builder
	for _, r := range h {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	h = b.String()

	for strings.Contains(h, "__") {
		h = strings.ReplaceAll(h, "__", "_")
	}

	return strings.Trim(h, "_")
}

// parseAmount converts a raw string amount into minor units.
func parseAmount(raw string, minorUnits bool, multiplier int64) (int64, error) {
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

	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("expected decimal amount, got %q", raw)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative amounts are not supported, got %f", val)
	}

	return int64(math.Round(val * float64(multiplier))), nil
}

// parseTimestamp parses a datetime string and returns it in UTC.
// The source timezone is applied during parsing so the conversion is correct.
func parseTimestamp(raw string, loc *time.Location) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700",
		"02/01/2006 15:04:05",
		"01/02/2006 15:04:05",
		"2006-01-02",
		"02/01/2006",
	}

	for _, format := range formats {
		t, err := time.ParseInLocation(format, raw, loc)
		if err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognised datetime format %q — supported formats: ISO 8601, YYYY-MM-DD HH:MM:SS, DD/MM/YYYY HH:MM:SS", raw)
}
