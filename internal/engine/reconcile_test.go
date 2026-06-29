package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/babafemi99/Mogaji/internal/domain"
	"gopkg.in/yaml.v3"
)

// loadTestConfig reads and parses a mapping.yml from a testdata directory.
// File paths in the config are resolved relative to the testdata directory.
func loadTestConfig(t *testing.T, dir string) domain.Config {
	t.Helper()

	mappingPath := filepath.Join(dir, "mapping.yml")
	data, err := os.ReadFile(mappingPath)
	if err != nil {
		t.Fatalf("failed to read mapping file: %v", err)
	}

	var cfg domain.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse mapping file: %v", err)
	}

	// Resolve file paths relative to the testdata directory.
	for i := range cfg.Sources {
		if !filepath.IsAbs(cfg.Sources[i].File) {
			cfg.Sources[i].File = filepath.Join(dir, cfg.Sources[i].File)
		}
	}

	return cfg
}

// ── Simple scenario ───────────────────────────────────────────────────────────
//
// internal.csv: 3 transactions (ONY_001, ONY_002, ONY_003)
// external.csv: 2 transactions (ONY_001, ONY_002)
//
// Expected:
//   - ONY_001 → ExactMatch (fee variance of 7500 kobo)
//   - ONY_002 → ExactMatch (fee variance of 3750 kobo)
//   - ONY_003 → MissingExternal (no external record)

func TestReconcile_Simple(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	e := New(cfg)
	run := e.Run()

	if run.Status != domain.RunStatusComplete {
		t.Fatalf("Status: got %q want COMPLETE — error: %s", run.Status, run.Error)
	}
}

func TestReconcile_Simple_MatchCounts(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	s := run.Summary

	if s.TotalInternal != 3 {
		t.Errorf("TotalInternal: got %d want 3", s.TotalInternal)
	}
	if s.TotalExternal != 2 {
		t.Errorf("TotalExternal: got %d want 2", s.TotalExternal)
	}
	if s.ExactMatches != 2 {
		t.Errorf("ExactMatches: got %d want 2", s.ExactMatches)
	}
	if s.MissingExternal != 1 {
		t.Errorf("MissingExternal: got %d want 1 — ONY_003 has no external record", s.MissingExternal)
	}
	if s.MissingInternal != 0 {
		t.Errorf("MissingInternal: got %d want 0", s.MissingInternal)
	}
	if s.AmbiguousMatches != 0 {
		t.Errorf("AmbiguousMatches: got %d want 0", s.AmbiguousMatches)
	}
}

func TestReconcile_Simple_Variance(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	// ONY_001: 500000 - 492500 = 7500 kobo
	// ONY_002: 250000 - 246250 = 3750 kobo
	// Total variance: 11250 kobo
	if run.Summary.TotalVarianceMinor != 11250 {
		t.Errorf("TotalVarianceMinor: got %d want 11250", run.Summary.TotalVarianceMinor)
	}
}

func TestReconcile_Simple_MatchRate(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	// 2 exact matches out of 3 internal = 66.666...%
	want := 66.66666666666666
	if run.Summary.MatchRatePercent != want {
		t.Errorf("MatchRatePercent: got %.10f want %.10f",
			run.Summary.MatchRatePercent, want)
	}
}

func TestReconcile_Simple_MatchesHaveCorrectOutcomes(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	if len(run.Matches) != 3 {
		t.Fatalf("got %d matches want 3", len(run.Matches))
	}

	// Count outcomes across all matches.
	outcomes := make(map[domain.MatchOutcome]int)
	for _, m := range run.Matches {
		outcomes[m.Outcome]++
	}

	if outcomes[domain.OutcomeExactMatch] != 2 {
		t.Errorf("ExactMatch count: got %d want 2", outcomes[domain.OutcomeExactMatch])
	}
	if outcomes[domain.OutcomeMissingExternal] != 1 {
		t.Errorf("MissingExternal count: got %d want 1", outcomes[domain.OutcomeMissingExternal])
	}
}

func TestReconcile_Simple_MissingExternalHasNoExternal(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	for _, m := range run.Matches {
		if m.Outcome == domain.OutcomeMissingExternal {
			if m.External != nil {
				t.Error("MissingExternal match should have nil External")
			}
			if m.Internal == nil {
				t.Error("MissingExternal match should have non-nil Internal")
			}
			if m.Internal.ReferenceID != "ONY_003" {
				t.Errorf("MissingExternal: got reference %q want ONY_003",
					m.Internal.ReferenceID)
			}
		}
	}
}

func TestReconcile_Simple_ExactMatchesHaveCorrectRule(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	for _, m := range run.Matches {
		if m.Outcome == domain.OutcomeExactMatch {
			if m.Rule != RuleReferenceMatch {
				t.Errorf("ExactMatch rule: got %q want %q", m.Rule, RuleReferenceMatch)
			}
			if m.Confidence != ConfidenceReferenceMatch {
				t.Errorf("ExactMatch confidence: got %f want %f",
					m.Confidence, ConfidenceReferenceMatch)
			}
		}
	}
}

func TestReconcile_Simple_SourceMetaPopulated(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	if len(run.Sources) != 2 {
		t.Fatalf("got %d sources want 2", len(run.Sources))
	}

	for _, src := range run.Sources {
		if src.Name == "" {
			t.Error("source has empty name")
		}
		if src.FilePath == "" {
			t.Error("source has empty file path")
		}
		if src.TotalLoaded == 0 {
			t.Errorf("source %q: TotalLoaded is 0", src.Name)
		}
	}
}

func TestReconcile_Simple_NoParsErrors(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	run := New(cfg).Run()

	if len(run.ParseErrors) != 0 {
		t.Errorf("got %d parse errors want 0: %+v",
			len(run.ParseErrors), run.ParseErrors)
	}
}

// ── Determinism invariant ─────────────────────────────────────────────────────

func TestReconcile_Simple_Deterministic(t *testing.T) {
	dir := filepath.Join("testdata", "simple")
	cfg := loadTestConfig(t, dir)

	// Run the same reconciliation twice.
	run1 := New(cfg).Run()
	run2 := New(cfg).Run()

	// Summary must be identical.
	if run1.Summary.ExactMatches != run2.Summary.ExactMatches {
		t.Errorf("ExactMatches differs between runs: %d vs %d",
			run1.Summary.ExactMatches, run2.Summary.ExactMatches)
	}
	if run1.Summary.MissingExternal != run2.Summary.MissingExternal {
		t.Errorf("MissingExternal differs between runs: %d vs %d",
			run1.Summary.MissingExternal, run2.Summary.MissingExternal)
	}
	if run1.Summary.TotalVarianceMinor != run2.Summary.TotalVarianceMinor {
		t.Errorf("TotalVarianceMinor differs between runs: %d vs %d",
			run1.Summary.TotalVarianceMinor, run2.Summary.TotalVarianceMinor)
	}
	if run1.Summary.MatchRatePercent != run2.Summary.MatchRatePercent {
		t.Errorf("MatchRatePercent differs between runs: %f vs %f",
			run1.Summary.MatchRatePercent, run2.Summary.MatchRatePercent)
	}

	// Match count must be identical.
	if len(run1.Matches) != len(run2.Matches) {
		t.Errorf("match count differs between runs: %d vs %d",
			len(run1.Matches), len(run2.Matches))
	}
}

func TestReconcile_InvalidConfig_RunStatusFailed(t *testing.T) {
	cfg := domain.Config{
		Run: domain.RunConfig{
			ID:       "test-fail",
			Currency: "NGN",
		},
		Sources: []domain.SourceConfig{
			{
				Name:     "internal",
				Role:     domain.SourceRoleInternal,
				File:     "testdata/simple/nonexistent.csv",
				Timezone: "UTC",
				Fields: domain.FieldMapping{
					Amount:    "amount",
					Timestamp: "created_at",
				},
			},
			{
				Name:     "external",
				Role:     domain.SourceRoleExternal,
				File:     "testdata/simple/external.csv",
				Timezone: "UTC",
				Fields: domain.FieldMapping{
					Amount:    "Settled Amount",
					Timestamp: "Transaction Date",
				},
			},
		},
	}

	run := New(cfg).Run()

	if run.Status != domain.RunStatusFailed {
		t.Errorf("Status: got %q want FAILED", run.Status)
	}
	if run.Error == "" {
		t.Error("Error: should be set on failed run")
	}
}
