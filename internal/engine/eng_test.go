package engine

import (
	"testing"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/internal/ingest"
)

// makeSourceMeta is a helper that creates a SourceMeta for testing.
func makeSourceMeta(role domain.SourceRole, totalAfterDedup int) domain.SourceMeta {
	return domain.SourceMeta{
		Name:            "test_source",
		Role:            role,
		TotalAfterDedup: totalAfterDedup,
	}
}

// makeMatch is a helper that creates a Match with a given outcome and variance.
func makeMatch(outcome domain.MatchOutcome, variance int64) domain.Match {
	return domain.Match{
		Outcome:  outcome,
		Variance: variance,
	}
}

func TestComputeSummary_InternalSourcesCounted(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 100),
		makeSourceMeta(domain.SourceRoleInternal, 50),
	}

	summary := computeSummary([]domain.Match{}, sources)

	if summary.TotalInternal != 150 {
		t.Errorf("TotalInternal: got %d want 150", summary.TotalInternal)
	}
	if summary.TotalExternal != 0 {
		t.Errorf("TotalExternal: got %d want 0", summary.TotalExternal)
	}
}

func TestComputeSummary_ExternalSourcesCounted(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleExternal, 80),
		makeSourceMeta(domain.SourceRoleExternal, 20),
	}

	summary := computeSummary([]domain.Match{}, sources)

	if summary.TotalExternal != 100 {
		t.Errorf("TotalExternal: got %d want 100", summary.TotalExternal)
	}
	if summary.TotalInternal != 0 {
		t.Errorf("TotalInternal: got %d want 0", summary.TotalInternal)
	}
}

func TestComputeSummary_MixedSources(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 500),
		makeSourceMeta(domain.SourceRoleExternal, 300),
		makeSourceMeta(domain.SourceRoleExternal, 200),
	}

	summary := computeSummary([]domain.Match{}, sources)

	if summary.TotalInternal != 500 {
		t.Errorf("TotalInternal: got %d want 500", summary.TotalInternal)
	}
	if summary.TotalExternal != 500 {
		t.Errorf("TotalExternal: got %d want 500", summary.TotalExternal)
	}
}

func TestComputeSummary_ExactMatchCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeExactMatch, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.ExactMatches != 2 {
		t.Errorf("ExactMatches: got %d want 2", summary.ExactMatches)
	}
	if summary.TotalMatches != 2 {
		t.Errorf("TotalMatches: got %d want 2", summary.TotalMatches)
	}
}

func TestComputeSummary_AmbiguousMatchCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeAmbiguousMatch, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.AmbiguousMatches != 1 {
		t.Errorf("AmbiguousMatches: got %d want 1", summary.AmbiguousMatches)
	}
}

func TestComputeSummary_DuplicateExternalCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeDuplicateExternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.DuplicateExternal != 1 {
		t.Errorf("DuplicateExternal: got %d want 1", summary.DuplicateExternal)
	}
}

func TestComputeSummary_DuplicateInternalCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeDuplicateInternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.DuplicateInternal != 1 {
		t.Errorf("DuplicateInternal: got %d want 1", summary.DuplicateInternal)
	}
}

func TestComputeSummary_MissingExternalCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeMissingExternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.MissingExternal != 1 {
		t.Errorf("MissingExternal: got %d want 1", summary.MissingExternal)
	}
}

func TestComputeSummary_MissingInternalCounted(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeMissingInternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.MissingInternal != 1 {
		t.Errorf("MissingInternal: got %d want 1", summary.MissingInternal)
	}
}

func TestComputeSummary_TotalMatchesIsAllOutcomes(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeAmbiguousMatch, 0),
		makeMatch(domain.OutcomeDuplicateExternal, 0),
		makeMatch(domain.OutcomeDuplicateInternal, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
		makeMatch(domain.OutcomeMissingInternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.TotalMatches != 6 {
		t.Errorf("TotalMatches: got %d want 6", summary.TotalMatches)
	}

	// Verify TotalMatches equals sum of all outcome counters.
	outcomeSum := summary.ExactMatches +
		summary.AmbiguousMatches +
		summary.DuplicateExternal +
		summary.DuplicateInternal +
		summary.MissingExternal +
		summary.MissingInternal

	if summary.TotalMatches != outcomeSum {
		t.Errorf("TotalMatches %d != sum of outcomes %d", summary.TotalMatches, outcomeSum)
	}
}

func TestComputeSummary_PositiveVarianceAccumulated(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 7500),
		makeMatch(domain.OutcomeExactMatch, 3750),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.TotalVarianceMinor != 11250 {
		t.Errorf("TotalVarianceMinor: got %d want 11250", summary.TotalVarianceMinor)
	}
}

func TestComputeSummary_NegativeVarianceAccumulated(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, -5000),
		makeMatch(domain.OutcomeExactMatch, -3000),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.TotalVarianceMinor != -8000 {
		t.Errorf("TotalVarianceMinor: got %d want -8000", summary.TotalVarianceMinor)
	}
}

func TestComputeSummary_MixedVarianceNetResult(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 10000),
		makeMatch(domain.OutcomeExactMatch, -3000),
		makeMatch(domain.OutcomeExactMatch, 5000),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.TotalVarianceMinor != 12000 {
		t.Errorf("TotalVarianceMinor: got %d want 12000", summary.TotalVarianceMinor)
	}
}

func TestComputeSummary_ZeroVariance(t *testing.T) {
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeExactMatch, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.TotalVarianceMinor != 0 {
		t.Errorf("TotalVarianceMinor: got %d want 0", summary.TotalVarianceMinor)
	}
}

func TestComputeSummary_MatchRate_100Percent(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 3),
	}
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeExactMatch, 0),
	}

	summary := computeSummary(matches, sources)

	if summary.MatchRatePercent != 100.0 {
		t.Errorf("MatchRatePercent: got %.2f want 100.00", summary.MatchRatePercent)
	}
}

func TestComputeSummary_MatchRate_0Percent(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 3),
	}
	matches := []domain.Match{
		makeMatch(domain.OutcomeMissingExternal, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
	}

	summary := computeSummary(matches, sources)

	if summary.MatchRatePercent != 0.0 {
		t.Errorf("MatchRatePercent: got %.2f want 0.00", summary.MatchRatePercent)
	}
}

func TestComputeSummary_MatchRate_Partial(t *testing.T) {
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 3),
	}
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
	}

	summary := computeSummary(matches, sources)

	// 2 exact out of 3 internal = 66.666...%
	want := 66.66666666666666
	if summary.MatchRatePercent != want {
		t.Errorf("MatchRatePercent: got %.10f want %.10f", summary.MatchRatePercent, want)
	}
}

func TestComputeSummary_MatchRate_ZeroInternalNoDivideByZero(t *testing.T) {
	// No internal sources — match rate must stay 0, not panic.
	matches := []domain.Match{
		makeMatch(domain.OutcomeMissingInternal, 0),
	}

	summary := computeSummary(matches, []domain.SourceMeta{})

	if summary.MatchRatePercent != 0.0 {
		t.Errorf("MatchRatePercent: got %.2f want 0.00 — divide by zero guard", summary.MatchRatePercent)
	}
}

func TestComputeSummary_EmptyInputsAllZeros(t *testing.T) {
	summary := computeSummary([]domain.Match{}, []domain.SourceMeta{})

	if summary.TotalInternal != 0 {
		t.Errorf("TotalInternal: got %d want 0", summary.TotalInternal)
	}
	if summary.TotalExternal != 0 {
		t.Errorf("TotalExternal: got %d want 0", summary.TotalExternal)
	}
	if summary.TotalMatches != 0 {
		t.Errorf("TotalMatches: got %d want 0", summary.TotalMatches)
	}
	if summary.ExactMatches != 0 {
		t.Errorf("ExactMatches: got %d want 0", summary.ExactMatches)
	}
	if summary.TotalVarianceMinor != 0 {
		t.Errorf("TotalVarianceMinor: got %d want 0", summary.TotalVarianceMinor)
	}
	if summary.MatchRatePercent != 0.0 {
		t.Errorf("MatchRatePercent: got %.2f want 0.00", summary.MatchRatePercent)
	}
}

func TestComputeSummary_TotalMatchesEqualsOutcomeSum(t *testing.T) {
	// Property: TotalMatches must always equal the sum of all outcome counters.
	sources := []domain.SourceMeta{
		makeSourceMeta(domain.SourceRoleInternal, 10),
		makeSourceMeta(domain.SourceRoleExternal, 8),
	}
	matches := []domain.Match{
		makeMatch(domain.OutcomeExactMatch, 500),
		makeMatch(domain.OutcomeExactMatch, 300),
		makeMatch(domain.OutcomeExactMatch, 0),
		makeMatch(domain.OutcomeAmbiguousMatch, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
		makeMatch(domain.OutcomeMissingExternal, 0),
		makeMatch(domain.OutcomeMissingInternal, 0),
		makeMatch(domain.OutcomeMissingInternal, 0),
		makeMatch(domain.OutcomeDuplicateInternal, 0),
		makeMatch(domain.OutcomeDuplicateExternal, 0),
	}

	summary := computeSummary(matches, sources)

	outcomeSum := summary.ExactMatches +
		summary.AmbiguousMatches +
		summary.DuplicateExternal +
		summary.DuplicateInternal +
		summary.MissingExternal +
		summary.MissingInternal

	if summary.TotalMatches != outcomeSum {
		t.Errorf("TotalMatches %d != sum of all outcomes %d — invariant broken",
			summary.TotalMatches, outcomeSum)
	}
}

// mockFinder is a test implementation of domain.CandidateFinder
// that returns a fixed list of candidates.
type mockFinder struct {
	candidates []*domain.Transaction
}

func (m *mockFinder) FindCandidates(_ *domain.Transaction) []*domain.Transaction {
	return m.candidates
}

// makeRule builds a domain.Rule with a mockFinder.
func makeRule(name string, confidence float64, candidates []*domain.Transaction) domain.Rule {
	return domain.Rule{
		Name:       name,
		Confidence: confidence,
		Finder:     &mockFinder{candidates: candidates},
	}
}

// makeEngineTx creates a minimal internal transaction for testing.
func makeEngineTx(referenceID string, amount int64) *domain.Transaction {
	return &domain.Transaction{
		ReferenceID:      referenceID,
		AmountMinorUnits: amount,
		Currency:         "NGN",
		Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC),
		SourceName:       "internal_ledger",
		SourceRole:       domain.SourceRoleInternal,
	}
}

// makeExternalTx creates a minimal external transaction for testing.
func makeExternalTx(referenceID string, amount int64) *domain.Transaction {
	return &domain.Transaction{
		ReferenceID:      referenceID,
		AmountMinorUnits: amount,
		Currency:         "NGN",
		Timestamp:        time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC),
		SourceName:       "paystack",
		SourceRole:       domain.SourceRoleExternal,
	}
}

// testEngine returns a minimal Engine for testing matchTransaction.
func testEngine() *Engine {
	return New(domain.Config{})
}

func TestMatchTransaction_SingleCandidate_Unclaimed_ExactMatch(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 492500)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	if match.Outcome != domain.OutcomeExactMatch {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeExactMatch)
	}
	if match.Internal != internal {
		t.Error("Internal: wrong transaction pointer")
	}
	if match.External != external {
		t.Error("External: wrong transaction pointer")
	}
	if match.Rule != RuleReferenceMatch {
		t.Errorf("Rule: got %q want %q", match.Rule, RuleReferenceMatch)
	}
	if match.Confidence != ConfidenceReferenceMatch {
		t.Errorf("Confidence: got %f want %f", match.Confidence, ConfidenceReferenceMatch)
	}
}

func TestMatchTransaction_SingleCandidate_Claimed_DuplicateInternal(t *testing.T) {
	internal2 := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 492500)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}

	// Pre-claim the external transaction as if internal1 already matched it.
	claimed := make(map[*domain.Transaction]struct{})
	claimed[external] = struct{}{}

	e := testEngine()
	match := e.matchTransaction(internal2, rules, claimed)

	if match.Outcome != domain.OutcomeDuplicateInternal {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeDuplicateInternal)
	}
	if match.Internal != internal2 {
		t.Error("Internal: wrong transaction pointer")
	}
	if match.External != external {
		t.Error("External: should be set on DuplicateInternal")
	}
}

func TestMatchTransaction_VarianceComputedCorrectly(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 492500)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	// Variance = internal - external = 500000 - 492500 = 7500
	if match.Variance != 7500 {
		t.Errorf("Variance: got %d want 7500", match.Variance)
	}
}

func TestMatchTransaction_NegativeVariance(t *testing.T) {
	// External higher than internal — provider recorded more.
	internal := makeEngineTx("ONY_001", 492500)
	external := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	// Variance = 492500 - 500000 = -7500
	if match.Variance != -7500 {
		t.Errorf("Variance: got %d want -7500", match.Variance)
	}
}

func TestMatchTransaction_ClaimsCandidate(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	e.matchTransaction(internal, rules, claimed)

	// External should now be in claimed map.
	if _, exists := claimed[external]; !exists {
		t.Error("matchTransaction() should have claimed the external transaction")
	}
}

// ── Multiple candidates path ──────────────────────────────────────────────────

func TestMatchTransaction_AllCandidatesClaimed_FallsThrough(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	ext1 := makeExternalTx("ONY_001", 500000)
	ext2 := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{ext1, ext2}),
	}

	// Both candidates already claimed.
	claimed := make(map[*domain.Transaction]struct{})
	claimed[ext1] = struct{}{}
	claimed[ext2] = struct{}{}

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	// Falls through all rules → MissingExternal.
	if match.Outcome != domain.OutcomeMissingExternal {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeMissingExternal)
	}
}

func TestMatchTransaction_OneUnclaimedCandidate_ExactMatch(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	ext1 := makeExternalTx("ONY_001", 500000)
	ext2 := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{ext1, ext2}),
	}

	// ext1 already claimed — only ext2 is available.
	claimed := make(map[*domain.Transaction]struct{})
	claimed[ext1] = struct{}{}

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	if match.Outcome != domain.OutcomeExactMatch {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeExactMatch)
	}
	if match.External != ext2 {
		t.Error("External: should be ext2 — the only unclaimed candidate")
	}
}

func TestMatchTransaction_MultipleUnclaimedCandidates_Ambiguous(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	ext1 := makeExternalTx("", 500000)
	ext2 := makeExternalTx("", 500000)
	ext3 := makeExternalTx("", 500000)

	rules := []domain.Rule{
		makeRule(RuleWeakKeyMatch, ConfidenceWeakKeyMatch, []*domain.Transaction{ext1, ext2, ext3}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	if match.Outcome != domain.OutcomeAmbiguousMatch {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeAmbiguousMatch)
	}
	if len(match.Candidates) != 3 {
		t.Errorf("Candidates: got %d want 3", len(match.Candidates))
	}
}

func TestMatchTransaction_AmbiguousMatch_DoesNotClaimCandidates(t *testing.T) {
	internal := makeEngineTx("", 500000)
	ext1 := makeExternalTx("", 500000)
	ext2 := makeExternalTx("", 500000)

	rules := []domain.Rule{
		makeRule(RuleWeakKeyMatch, ConfidenceWeakKeyMatch, []*domain.Transaction{ext1, ext2}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	e.matchTransaction(internal, rules, claimed)

	// Neither candidate should be claimed after an ambiguous match.
	if _, exists := claimed[ext1]; exists {
		t.Error("ext1 should not be claimed after ambiguous match")
	}
	if _, exists := claimed[ext2]; exists {
		t.Error("ext2 should not be claimed after ambiguous match")
	}
}

func TestMatchTransaction_Pass1Matches_Pass2NotCalled(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 500000)

	pass2Called := false
	pass2Finder := &callTrackingFinder{called: &pass2Called}

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
		{Name: RuleWeakKeyMatch, Confidence: ConfidenceWeakKeyMatch, Finder: pass2Finder},
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	e.matchTransaction(internal, rules, claimed)

	if pass2Called {
		t.Error("PASS 2 should not be called when PASS 1 finds a match")
	}
}

func TestMatchTransaction_Pass1Misses_Pass2Finds(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{}),     // PASS 1 misses
		makeRule(RuleWeakKeyMatch, ConfidenceWeakKeyMatch, []*domain.Transaction{external}), // PASS 2 finds
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	if match.Outcome != domain.OutcomeExactMatch {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeExactMatch)
	}
	if match.Rule != RuleWeakKeyMatch {
		t.Errorf("Rule: got %q want %q — should be PASS 2", match.Rule, RuleWeakKeyMatch)
	}
	if match.Confidence != ConfidenceWeakKeyMatch {
		t.Errorf("Confidence: got %f want %f", match.Confidence, ConfidenceWeakKeyMatch)
	}
}

func TestMatchTransaction_AllRulesMiss_MissingExternal(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{}),
		makeRule(RuleWeakKeyMatch, ConfidenceWeakKeyMatch, []*domain.Transaction{}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()
	match := e.matchTransaction(internal, rules, claimed)

	if match.Outcome != domain.OutcomeMissingExternal {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeMissingExternal)
	}
}

func TestMatchTransaction_EmptyRules_MissingExternal(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)

	e := testEngine()
	match := e.matchTransaction(internal, []domain.Rule{}, make(map[*domain.Transaction]struct{}))

	if match.Outcome != domain.OutcomeMissingExternal {
		t.Errorf("Outcome: got %q want %q", match.Outcome, domain.OutcomeMissingExternal)
	}
}

func TestMatchTransaction_InternalAlwaysSet(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{}),
	}

	e := testEngine()
	match := e.matchTransaction(internal, rules, make(map[*domain.Transaction]struct{}))

	if match.Internal != internal {
		t.Error("Internal should always be set regardless of outcome")
	}
}

func TestMatchTransaction_ExternalNilOnMissingExternal(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{}),
	}

	e := testEngine()
	match := e.matchTransaction(internal, rules, make(map[*domain.Transaction]struct{}))

	if match.External != nil {
		t.Errorf("External should be nil on MissingExternal: got %+v", match.External)
	}
}

func TestMatchTransaction_RuleNameAndConfidenceSet(t *testing.T) {
	internal := makeEngineTx("ONY_001", 500000)
	external := makeExternalTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule("CUSTOM_RULE", 0.95, []*domain.Transaction{external}),
	}

	e := testEngine()
	match := e.matchTransaction(internal, rules, make(map[*domain.Transaction]struct{}))

	if match.Rule != "CUSTOM_RULE" {
		t.Errorf("Rule: got %q want CUSTOM_RULE", match.Rule)
	}
	if match.Confidence != 0.95 {
		t.Errorf("Confidence: got %f want 0.95", match.Confidence)
	}
}

func TestMatchTransaction_ClaimedPreventsDoubleClaim(t *testing.T) {
	external := makeExternalTx("ONY_001", 500000)
	internal1 := makeEngineTx("ONY_001", 500000)
	internal2 := makeEngineTx("ONY_001", 500000)

	rules := []domain.Rule{
		makeRule(RuleReferenceMatch, ConfidenceReferenceMatch, []*domain.Transaction{external}),
	}
	claimed := make(map[*domain.Transaction]struct{})

	e := testEngine()

	// First transaction claims the external.
	match1 := e.matchTransaction(internal1, rules, claimed)
	if match1.Outcome != domain.OutcomeExactMatch {
		t.Fatalf("first match should be ExactMatch, got %q", match1.Outcome)
	}

	// Second transaction finds the same external but it's already claimed.
	match2 := e.matchTransaction(internal2, rules, claimed)
	if match2.Outcome != domain.OutcomeDuplicateInternal {
		t.Errorf("second match should be DuplicateInternal, got %q", match2.Outcome)
	}
}

// callTrackingFinder is a test finder that tracks whether it was called.
type callTrackingFinder struct {
	called *bool
}

func (c *callTrackingFinder) FindCandidates(_ *domain.Transaction) []*domain.Transaction {
	*c.called = true
	return nil
}

func TestToParseError_AllFieldsCopied(t *testing.T) {
	row := ingest.RowError{
		SourceName: "paystack",
		SourceFile: "paystack.csv",
		RowNumber:  42,
		Reason:     "invalid amount",
		RawRow:     []string{"ONY_001", "bad-amount", "NGN", "2026-06-21 09:00:00", "success"},
	}

	got := toParseError(row)

	if got.SourceName != row.SourceName {
		t.Errorf("SourceName: got %q want %q", got.SourceName, row.SourceName)
	}
	if got.SourceFile != row.SourceFile {
		t.Errorf("SourceFile: got %q want %q", got.SourceFile, row.SourceFile)
	}
	if got.RowNumber != row.RowNumber {
		t.Errorf("RowNumber: got %d want %d", got.RowNumber, row.RowNumber)
	}
	if got.Reason != row.Reason {
		t.Errorf("Reason: got %q want %q", got.Reason, row.Reason)
	}
	if len(got.RawRow) != len(row.RawRow) {
		t.Fatalf("RawRow length: got %d want %d", len(got.RawRow), len(row.RawRow))
	}
	for i, v := range row.RawRow {
		if got.RawRow[i] != v {
			t.Errorf("RawRow[%d]: got %q want %q", i, got.RawRow[i], v)
		}
	}
}

func TestToParseError_EmptyFields(t *testing.T) {
	row := ingest.RowError{}
	got := toParseError(row)

	if got.SourceName != "" {
		t.Errorf("SourceName: got %q want empty", got.SourceName)
	}
	if got.SourceFile != "" {
		t.Errorf("SourceFile: got %q want empty", got.SourceFile)
	}
	if got.RowNumber != 0 {
		t.Errorf("RowNumber: got %d want 0", got.RowNumber)
	}
	if got.Reason != "" {
		t.Errorf("Reason: got %q want empty", got.Reason)
	}
	if got.RawRow != nil {
		t.Errorf("RawRow: got %v want nil", got.RawRow)
	}
}

func TestToParseError_NilRawRow(t *testing.T) {
	row := ingest.RowError{
		SourceName: "internal_ledger",
		RowNumber:  5,
		Reason:     "CSV read error",
		RawRow:     nil,
	}

	got := toParseError(row)

	if got.RawRow != nil {
		t.Errorf("RawRow: got %v want nil", got.RawRow)
	}
}
