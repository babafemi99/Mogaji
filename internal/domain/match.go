package domain

// MatchOutcome describes the classification of a reconciliation result.
// Every transaction pair — or unpaired transaction — produces exactly one outcome.
// These outcomes are the primary signal for auditors and finance teams.
type MatchOutcome string

const (
	// OutcomeExactMatch means the engine found a confident match using a high-signal rule.
	// Example: same ReferenceID, same amount, same currency.
	OutcomeExactMatch MatchOutcome = "EXACT_MATCH"

	// OutcomeAmbiguousMatch means candidates were found but the engine could not
	// confidently select one. Multiple external transactions matched the same
	// internal transaction under a weak rule.
	// These require human review.
	OutcomeAmbiguousMatch MatchOutcome = "AMBIGUOUS_MATCH"

	// OutcomeDuplicateExternal means more than one external transaction was found
	// for a single internal transaction. This is distinct from ambiguity —
	// it suggests the provider recorded the same transaction more than once.
	OutcomeDuplicateExternal MatchOutcome = "DUPLICATE_EXTERNAL"

	// OutcomeDuplicateInternal means more than one internal transaction was found
	// for a single external transaction. Suggests a double-write in the internal ledger.
	OutcomeDuplicateInternal MatchOutcome = "DUPLICATE_INTERNAL"

	// OutcomeMissingExternal means the internal ledger has a record but the
	// external provider statement has no corresponding entry.
	// Classic symptom: transaction was recorded internally but never settled.
	OutcomeMissingExternal MatchOutcome = "MISSING_EXTERNAL"

	// OutcomeMissingInternal means the external provider statement has a record
	// but the internal ledger has no corresponding entry.
	// Classic symptom: partial commit drift — money moved externally, ledger never updated.
	OutcomeMissingInternal MatchOutcome = "MISSING_INTERNAL"
)

// Match is the primary output unit of the Mogaji reconciliation engine.
//
// Every reconciliation run produces a slice of Match results.
// A Match always has at least one of Internal or External set.
// When both are set, the engine found a candidate pair.
// When only one is set, the outcome is Missing* on the other side.
type Match struct {
	// Internal is the transaction from the internal ledger.
	// Nil when OutcomeMissingInternal.
	Internal *Transaction `json:"internal,omitempty"`

	// External is the transaction from the external provider statement.
	// Nil when OutcomeMissingExternal.
	External *Transaction `json:"external,omitempty"`

	// Outcome is the classification of this match result.
	// Always set.
	Outcome MatchOutcome `json:"outcome"`

	// Rule is the name of the CandidateFinder rule that produced this match.
	// Empty string when outcome is Missing* (no rule fired — absence is the signal).
	// Examples: "REFERENCE_MATCH", "WEAK_KEY_MATCH", "AMOUNT_WINDOW_MATCH"
	Rule string `json:"rule,omitempty"`

	// Confidence is the rule-level confidence declared by the reconciliation policy.
	// This is not a probabilistic score — it is a deterministic value assigned
	// to the rule that produced this match.
	// Range: 0.0 (manual review) to 1.0 (exact reference match).
	// Zero when outcome is Missing*.
	Confidence float64 `json:"confidence,omitempty"`

	// Variance is the difference between internal and external amounts in minor units.
	// Computed as: Internal.AmountMinorUnits - External.AmountMinorUnits
	// Zero when amounts are identical.
	// Negative when external is larger (e.g. provider recorded more than internal ledger).
	// Only meaningful when both Internal and External are set.
	Variance int64 `json:"variance,omitempty"`

	// Candidates holds all external transactions the finder returned before
	// the engine selected (or failed to select) one.
	// Populated only when Outcome is AmbiguousMatch, DuplicateExternal, or DuplicateInternal.
	// Useful for audit trails — shows exactly what the engine was choosing between.
	Candidates []*Transaction `json:"candidates,omitempty"`
}
