package domain

// CandidateFinder is the interface every matching strategy must implement.
//
// It has one job: given a transaction, return a list of candidate transactions
// from the opposing source pool that could be a match.
//
// A CandidateFinder never makes a match decision — it only returns candidates.
// The engine decides whether a candidate is a valid match, what outcome to
// assign, and what confidence to record.
//
// Implementations must be safe for concurrent use if the engine ever
// parallelises pass execution. For now Mogaji is single-threaded, but
// the contract should be honoured from the start.
type CandidateFinder interface {
	FindCandidates(tx *Transaction) []*Transaction
}

// Rule wraps a CandidateFinder with the policy metadata the engine needs
// to record in match results.
//
// Confidence is a property of the reconciliation policy, not the finder.
// The same ReferenceFinder may be worth 1.0 in a strict bank reconciliation
// and 0.8 in a marketplace payout run where reference IDs are reused.
// Declaring confidence on the Rule — not the finder — keeps that flexibility.
type Rule struct {
	// Name is a short identifier for this rule, recorded on every Match
	// it produces. Used in reports and audit trails.
	// Examples: "REFERENCE_MATCH", "WEAK_KEY_MATCH", "AMOUNT_WINDOW_MATCH"
	Name string

	// Confidence is the rule-level confidence assigned to matches produced
	// by this rule. Range: 0.0 to 1.0.
	// This is deterministic and policy-declared — not a probabilistic score.
	Confidence float64

	// Finder is the CandidateFinder implementation that performs the lookup.
	Finder CandidateFinder
}
