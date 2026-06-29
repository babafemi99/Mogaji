package domain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/babafemi99/Mogaji/sancho"
)

// Config is the validated, parsed representation of the YAML mapping file.
//
// It is the single source of truth for how a reconciliation run behaves —
// which files to ingest, how to map fields, what currency to enforce,
// and what matching thresholds to apply.
//
// Config is constructed once at startup from the YAML file and passed
// into the engine. It is never mutated after construction.
type Config struct {
	// Run holds the top-level run identity and policy settings.
	Run RunConfig `yaml:"run"`

	// Sources is the ordered list of all input sources for this run.
	// Must contain at least one internal and one external source.
	Sources []SourceConfig `yaml:"sources"`
}

func (c Config) Validate() error {
	if err := c.Run.Validate(); err != nil {
		return err
	}

	if len(c.Sources) == 0 {
		return sancho.ErrNoSourcesDeclared
	}

	var hasInternal, hasExternal bool
	for i, src := range c.Sources {
		if err := src.Validate(); err != nil {
			return fmt.Errorf("failed to validate source[%d]: %w", i, err)
		}
		if src.Role == SourceRoleInternal {
			hasInternal = true
		}
		if src.Role == SourceRoleExternal {
			hasExternal = true
		}
	}

	if !hasInternal {
		return sancho.ErrInternalSourceRequired
	}
	if !hasExternal {
		return sancho.ErrExternalSourceRequired
	}

	return nil
}

// RunConfig holds the identity and policy settings for a reconciliation run.
type RunConfig struct {
	// ID is a unique identifier for this run.
	// Recommended format: "YYYY-MM-DD-{descriptor}"
	// Example: "2026-06-21-paystack-daily"
	ID string `yaml:"id"`

	// Currency is the ISO 4217 currency code enforced for this entire run.
	// All transactions across all sources must match this currency.
	// Example: "NGN", "USD", "KES"
	Currency string `yaml:"currency"`

	// TimeWindowSeconds is the maximum allowed time difference between
	// internal and external timestamps for a PASS 2 time-bound match.
	// Default: 86400 (24 hours).
	TimeWindowSeconds int64 `yaml:"time_window_seconds"`

	// FeeTolerancePercent is the maximum allowed percentage difference
	// between internal and external amounts for a PASS 3 fee-aware match.
	// Example: 1.5 means amounts within 1.5% of each other are candidates.
	// Default: 0 (disabled — fee-aware matching must be explicitly enabled).
	FeeTolerancePercent float64 `yaml:"fee_tolerance_percent"`
}

func (c RunConfig) Validate() error {
	if c.ID == "" {
		return sancho.ErrRunIDRequired
	}
	if c.Currency == "" {
		return sancho.ErrRunCurrencyRequired
	}
	return nil
}

// SourceConfig describes a single input source and how to read it.
// Maps directly to one entry under `sources:` in the YAML file.
type SourceConfig struct {
	// Name is the human-readable label for this source.
	// Must be unique within a run.
	// Used in match results and the report to identify transaction origins.
	// Examples: "paystack", "flutterwave", "moniepoint_pos", "internal_ledger"
	Name string `yaml:"name"`

	// Role declares which side of the reconciliation this source belongs to.
	// Must be "internal" or "external".
	Role SourceRole `yaml:"role"`

	// File is the path to the CSV file for this source.
	File string `yaml:"file"`

	// Timezone is the IANA timezone name for timestamps in this source.
	// All timestamps are converted to UTC at ingest using this value.
	// Examples: "Africa/Lagos", "UTC", "Africa/Nairobi", "America/New_York"
	Timezone string `yaml:"timezone"`

	// MinorUnits declares whether the amount field in this CSV is already
	// in minor units (e.g. kobo) or in major units (e.g. naira).
	// true  → amounts are already minor units, no conversion applied
	// false → amounts are in major units, Mogaji multiplies by 10^DecimalPlaces
	MinorUnits bool `yaml:"minor_units"`

	// DecimalPlaces is the number of decimal places in the major unit amount.
	// Only used when MinorUnits is false.
	// Example: 2 for NGN (kobo), 2 for USD (cents), 0 for JPY (no subdivision)
	// Default: 2
	DecimalPlaces int `yaml:"decimal_places"`

	// Fields declares the mapping from Mogaji's canonical field names
	// to the actual column headers in this source's CSV file.
	Fields FieldMapping `yaml:"fields"`
}

func (s SourceConfig) Validate() error {

	if err := s.Fields.Validate(); err != nil {
		return err
	}

	if s.Name == "" {
		return sancho.ErrSourceNameRequired
	}

	if err := s.Role.Validate(); err != nil {
		return err
	}

	if err := validateFilePath(s.File); err != nil {
		return err
	}

	if s.Timezone == "" {
		return sancho.ErrSourceTimezoneRequired
	}

	if !s.MinorUnits && s.DecimalPlaces < 0 {
		return sancho.ErrInvalidDecimalPlaces
	}

	if err := s.Fields.Validate(); err != nil {
		return err
	}

	return nil
}

// FieldMapping maps Mogaji's canonical field names to the actual CSV column headers
// for a given source. Every source has its own mapping because providers
// name their columns differently.
//
// Example: Paystack calls it "transaction_id", Flutterwave calls it "tx_ref",
// your internal ledger calls it "txn_ref" — all map to ReferenceID.
type FieldMapping struct {
	// ReferenceID is the column header for the transaction identifier.
	// May be empty string if the source does not provide a reference ID.
	// When empty, the engine falls back to weak-key matching for all
	// transactions from this source.
	ReferenceID string `yaml:"reference_id"`

	// Amount is the column header for the transaction amount.
	// Required. No default.
	Amount string `yaml:"amount"`

	// Currency is the column header for the currency code.
	// Optional — if empty, the run-level currency is assumed for all rows.
	Currency string `yaml:"currency"`

	// Timestamp is the column header for the transaction datetime.
	// Required. No default.
	Timestamp string `yaml:"timestamp"`

	// Status is the column header for the transaction status.
	// Optional. If empty, RawStatus will be empty string on all transactions
	// from this source.
	Status string `yaml:"status"`
}

func (f FieldMapping) Validate() error {
	if f.Amount == "" {
		return sancho.ErrAmountRequired
	}

	if f.Timestamp == "" {
		return sancho.ErrTimeStampRequired
	}

	return nil
}

func validateFilePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file_path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file %q does not exist", path)
		}
		return fmt.Errorf("cannot access file %q: %w", path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%q is a directory, expected a CSV file", path)
	}

	if !strings.EqualFold(filepath.Ext(path), ".csv") {
		return fmt.Errorf("%q must be a .csv file", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open %q: %w", path, err)
	}
	defer f.Close()

	return nil
}
