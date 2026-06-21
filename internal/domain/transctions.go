package domain

import "time"

// Transaction is the canonical normalized record in Mogaji.
//
// All raw CSV rows — regardless of provider, schema, or field naming —
// are normalized into this struct before any matching begins.
//
// Invariants that must hold after normalization:
//   - AmountMinorUnits is always a positive int64 (e.g. kobo for NGN, cents for USD)
//   - Timestamp is always UTC
//   - Currency is always uppercase ISO 4217 (e.g. "NGN", "USD")
//   - ReferenceID may be empty — the engine must handle this gracefully
//   - SourceName always matches a SourceMeta.Name declared in the run config
//   - SourceRole always matches the role of that SourceMeta
//   - SourceRow is 1-indexed (matches the CSV row number including header)
type Transaction struct {
	// ReferenceID is the provider-declared transaction identifier.
	// May be empty for certain bank exports or bulk settlement files.
	// Never use this as a map key directly without an empty check.
	ReferenceID string `json:"reference_id"`

	// SourceName is the name of the source this transaction came from.
	// Always matches a SourceMeta.Name declared in the run config.
	// Examples: "paystack", "flutterwave", "moniepoint_pos", "internal_ledger"
	SourceName string `json:"source_name"`

	// SourceRole is the role of the source this transaction came from.
	// Copied from SourceMeta.Role at ingest time for convenient access
	// without needing to look up the SourceMeta on every engine operation.
	SourceRole SourceRole `json:"source_role"`

	// AmountMinorUnits is the transaction amount in the smallest currency unit.
	// Examples: ₦5,000.75 → 500075 kobo | $10.00 → 1000 cents
	// Floating-point is never used. All conversion happens at ingest time.
	AmountMinorUnits int64 `json:"amount_minor_units"`

	// Currency is the ISO 4217 currency code, always uppercase.
	// Mogaji v1 enforces single-currency runs — cross-currency is rejected at ingest.
	Currency string `json:"currency"`

	// Timestamp is the transaction time, always normalized to UTC.
	// The source timezone is declared in the YAML mapping and applied at ingest.
	Timestamp time.Time `json:"timestamp"`

	// RawStatus is the status string exactly as it appeared in the source CSV.
	// Mogaji does not normalize statuses — that is left to the caller.
	RawStatus string `json:"raw_status"`

	// SourceFile is the path of the CSV file this transaction was read from.
	// Preserved for audit trail and error reporting.
	SourceFile string `json:"source_file"`

	// SourceRow is the 1-indexed row number in the source CSV (including header row).
	// Row 1 = header, Row 2 = first data row.
	// Preserved so any match result or error can be traced back to an exact line.
	SourceRow int `json:"source_row"`
}
