package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/internal/ingest"
	"github.com/logrusorgru/aurora/v4"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
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
func Write(run domain.Run, outputPath string) error {
	report := Report{
		Version: "1.0",
		Run:     run,
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
	var buf bytes.Buffer
	s := run.Summary

	// Identity
	fmt.Fprintf(&buf, "  Run ID   : %s\n", aurora.Bold(run.ID))
	fmt.Fprintf(&buf, "  Status   : %s\n", colorStatus(run.Status))
	fmt.Fprintf(&buf, "  Currency : %s\n\n", aurora.Bold(run.Currency))

	// Sources table
	fmt.Fprintln(&buf, aurora.Bold("  Sources"))
	srcTable := tablewriter.NewTable(&buf, tablewriter.WithConfig(tablewriter.Config{
		Row: tw.CellConfig{
			Alignment: tw.CellAlignment{
				PerColumn: []tw.Align{tw.AlignLeft, tw.AlignRight},
			},
		},
	}))
	srcTable.Header([]string{"Side", "Transactions"})
	srcTable.Append([]string{"Internal", fmt.Sprintf("%d", s.TotalInternal)})
	srcTable.Append([]string{"External", fmt.Sprintf("%d", s.TotalExternal)})
	srcTable.Render()

	// Results table
	fmt.Fprintln(&buf, aurora.Bold("\n  Results"))
	resTable := tablewriter.NewTable(&buf, tablewriter.WithConfig(tablewriter.Config{
		Row: tw.CellConfig{
			Alignment: tw.CellAlignment{
				PerColumn: []tw.Align{tw.AlignLeft, tw.AlignRight},
			},
		},
	}))
	resTable.Header([]string{"Outcome", "Count"})
	resTable.Bulk([][]string{
		{fmt.Sprint(aurora.Green("Exact Match")), fmt.Sprintf("%d", s.ExactMatches)},
		{fmt.Sprint(aurora.Yellow("Ambiguous")), fmt.Sprintf("%d", s.AmbiguousMatches)},
		{fmt.Sprint(aurora.Yellow("Missing External")), fmt.Sprintf("%d", s.MissingExternal)},
		{fmt.Sprint(aurora.Yellow("Missing Internal")), fmt.Sprintf("%d", s.MissingInternal)},
		{fmt.Sprint(aurora.Red("Duplicate Internal")), fmt.Sprintf("%d", s.DuplicateInternal)},
		{fmt.Sprint(aurora.Red("Duplicate External")), fmt.Sprintf("%d", s.DuplicateExternal)},
	})
	resTable.Render()

	// Footer stats
	var matchColor func(arg interface{}) aurora.Value
	switch {
	case s.MatchRatePercent >= 95:
		matchColor = aurora.Green
	case s.MatchRatePercent >= 80:
		matchColor = aurora.Yellow
	default:
		matchColor = aurora.Red
	}

	fmt.Fprintf(&buf, "\n  Match rate    : %s%%\n",
		matchColor(fmt.Sprintf("%.2f", s.MatchRatePercent)))
	fmt.Fprintf(&buf, "  Total variance: %s\n",
		aurora.Bold(fmt.Sprintf("₦%.2f", float64(s.TotalVarianceMinor)/100)))

	if len(run.ParseErrors) > 0 {
		fmt.Fprintf(&buf, "  Parse errors  : %s rows skipped during ingestion\n",
			aurora.Yellow(fmt.Sprintf("%d", len(run.ParseErrors))))
	}

	fmt.Fprintln(&buf)
	return buf.String()
}

func colorStatus(status domain.RunStatus) aurora.Value {
	switch status {
	case domain.RunStatusComplete:
		return aurora.Green(string(status))
	case domain.RunStatusFailed:
		return aurora.Red(string(status))
	default:
		return aurora.Yellow(string(status))
	}
}
