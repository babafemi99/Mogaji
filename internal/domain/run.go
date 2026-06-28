package domain

import (
	"time"

	"github.com/babafemi99/Mogaji/sancho"
)

// RunStatus describes the current state of a reconciliation run.
type RunStatus string

const (
	// RunStatusPending means the run has been created but not yet started.
	RunStatusPending RunStatus = "PENDING"

	// RunStatusRunning means the engine is actively processing.
	RunStatusRunning RunStatus = "RUNNING"

	// RunStatusComplete means the engine finished without fatal errors.
	// Individual match outcomes may still contain warnings or ambiguities.
	RunStatusComplete RunStatus = "COMPLETE"

	// RunStatusFailed means the engine encountered a fatal error and stopped.
	// Partial results should not be trusted.
	RunStatusFailed RunStatus = "FAILED"
)

// SourceRole describes which side of the reconciliation a source belongs to.
// A run may have multiple sources on either side.
type SourceRole string

const (
	// SourceRoleInternal means this source is an internal ledger.
	// e.g. your application database export, your accounting system CSV.
	SourceRoleInternal SourceRole = "internal"

	// SourceRoleExternal means this source is an external provider statement.
	// e.g. Paystack settlement CSV, Flutterwave payout report, bank statement.
	SourceRoleExternal SourceRole = "external"
)

func (s SourceRole) String() string {
	return string(s)
}

func (s SourceRole) Validate() error {
	switch s {
	case SourceRoleInternal, SourceRoleExternal:
		return nil
	default:
		return sancho.ErrInvalidSourceRole
	}
}

// SourceMeta describes a single input source in a reconciliation run.
// A run may have any number of sources on either side.
type SourceMeta struct {
	// Name is a human-readable label for this source.
	// Used in match results and reports to identify which source a transaction came from.
	// Examples: "paystack", "flutterwave", "moniepoint_pos", "internal_ledger"
	Name string `json:"name"`

	// Role indicates which side of the reconciliation this source belongs to.
	Role SourceRole `json:"role"`

	// FilePath is the path to the CSV file for this source.
	FilePath string `json:"file_path"`

	// TotalLoaded is the count of raw rows read from the CSV before deduplication.
	TotalLoaded int `json:"total_loaded"`

	// TotalAfterDedup is the count of transactions remaining after duplicate removal.
	TotalAfterDedup int `json:"total_after_dedup"`

	// DuplicatesSkipped is the count of rows dropped during deduplication.
	DuplicatesSkipped int `json:"duplicates_skipped"`
}

// Run is the top-level context for a single reconciliation execution.
//
// Every report, match result, variance, and audit entry belongs to a Run.
// This is the root object that ties everything together.
//
// A Run is created before ingestion begins and updated as the engine progresses.
// The final Run object — with all Matches populated — is what gets serialized
// into the JSON report.
type Run struct {
	// ID is a unique identifier for this reconciliation run.
	// Recommended format: "YYYY-MM-DD-{descriptor}" e.g. "2026-06-21-paystack-daily"
	// Set by the caller via the YAML run config or CLI flag.
	ID string `json:"id"`

	// StartedAt is when the run began. Always UTC.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the run finished. Always UTC.
	// Zero value when run is still in progress or failed before completion.
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Status is the current state of the run.
	Status RunStatus `json:"status"`

	// Currency is the single currency enforced for this run.
	// All transactions across all sources must match this currency
	// or they are rejected at ingest.
	Currency string `json:"currency"`

	// Sources is the ordered list of all input sources for this run.
	// May contain multiple internal and/or multiple external sources.
	// Order matches the order declared in the YAML mapping file.
	Sources []SourceMeta `json:"sources"`

	// Matches holds every match result produced by the engine.
	// This includes exact matches, ambiguous matches, duplicates, and missing entries.
	// The full picture of the reconciliation is here.
	Matches []Match `json:"matches"`

	// Summary is a high-level breakdown of outcomes across all matches.
	// Computed once after the engine finishes — not updated incrementally.
	Summary RunSummary `json:"summary"`

	// Error holds the fatal error message if Status is RunStatusFailed.
	// Empty string otherwise.
	Error string `json:"error,omitempty"`
}

// RunSummary is a pre-computed breakdown of match outcomes for a completed run.
// Exists so consumers of the JSON report can read totals without scanning every match.
type RunSummary struct {
	TotalInternal      int     `json:"total_internal"`
	TotalExternal      int     `json:"total_external"`
	TotalMatches       int     `json:"total_matches"`
	ExactMatches       int     `json:"exact_matches"`
	AmbiguousMatches   int     `json:"ambiguous_matches"`
	DuplicateExternal  int     `json:"duplicate_external"`
	DuplicateInternal  int     `json:"duplicate_internal"`
	MissingExternal    int     `json:"missing_external"`
	MissingInternal    int     `json:"missing_internal"`
	TotalVarianceMinor int64   `json:"total_variance_minor_units"`
	MatchRatePercent   float64 `json:"match_rate_percent"`
}
