package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/internal/ingest"
)

// Report is the top-level JSON output structure written to disk.
// It wraps the Run with additional metadata useful for consumers
// who don't want to scan the full match list.
type Report struct {
	// Version is the Mogaji report format version.
	// Allows consumers to handle format changes gracefully.
	Version string `json:"version"`

	// Run is the full reconciliation run result.
	Run domain.Run `json:"run"`

	// IngestErrors are all row-level parse errors collected across all sources.
	// These are rows that failed normalization and never participated in matching.
	// Surfaced here so the consumer sees the full picture in one place.
	IngestErrors []ingest.RowError `json:"ingest_errors,omitempty"`
}

// Write serializes the Run and any ingest errors into a JSON report
// and writes it to the given output path.
//
// The output file is created if it does not exist.
// Parent directories are created if they do not exist.
// An existing file at the output path is overwritten.
//
// The JSON is pretty-printed for human readability —
// these reports are read by finance teams and auditors, not just machines.
func Write(run domain.Run, ingestErrors []ingest.RowError, outputPath string) error {
	report := Report{
		Version:      "1.0",
		Run:          run,
		IngestErrors: ingestErrors,
	}

	// Ensure parent directories exist.
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("report: cannot create output directory %q: %w", dir, err)
	}

	// Marshal to pretty-printed JSON.
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("report: failed to marshal run to JSON: %w", err)
	}

	// Write atomically — write to a temp file then rename.
	// Prevents a partially written report if the process crashes mid-write.
	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("report: failed to write temp file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		// Clean up the temp file if rename fails.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("report: failed to rename temp file to %q: %w", outputPath, err)
	}

	return nil
}

// Summary returns a human-readable summary string of a completed run.
// Used for CLI stdout output after the report is written.
func Summary(run domain.Run) string {
	s := run.Summary
	return fmt.Sprintf(`
Mogaji Reconciliation Report
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Run ID       : %s
Status       : %s
Currency     : %s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Internal txs : %d
External txs : %d
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Exact matches     : %d
Ambiguous         : %d
Duplicate internal: %d
Duplicate external: %d
Missing external  : %d
Missing internal  : %d
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Match rate        : %.2f%%
Total variance    : %d minor units
━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`,
		run.ID,
		run.Status,
		run.Currency,
		s.TotalInternal,
		s.TotalExternal,
		s.ExactMatches,
		s.AmbiguousMatches,
		s.DuplicateInternal,
		s.DuplicateExternal,
		s.MissingExternal,
		s.MissingInternal,
		s.MatchRatePercent,
		s.TotalVarianceMinor,
	)
}
