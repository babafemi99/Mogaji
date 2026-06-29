package engine

import (
	"fmt"
	"time"

	"github.com/babafemi99/Mogaji/internal/domain"
	"github.com/babafemi99/Mogaji/internal/finder"
	"github.com/babafemi99/Mogaji/internal/ingest"
)

const (
	ConfidenceReferenceMatch = 1.00
	ConfidenceWeakKeyMatch   = 0.85
	ConfidenceAmountWindow   = 0.70
	ConfidenceManualReview   = 0.00

	RuleReferenceMatch = "REFERENCE_MATCH"
	RuleWeakKeyMatch   = "WEAK_KEY_MATCH"
	RuleAmountWindow   = "AMOUNT_WINDOW_MATCH"
)

// Engine is the Mogaji reconciliation engine.
//
// It orchestrates the full reconciliation run:
//  1. Load all external sources into memory and build finder indexes
//  2. Stream all internal sources row by row
//  3. For each internal transaction, run it through the rule chain
//  4. After streaming, flag any unclaimed external transactions as MissingInternal
//  5. Compute the run summary
//
// The engine is single-use — create one per reconciliation run.
type Engine struct {
	cfg domain.Config
}

// New creates a new Engine for the given config.
func New(cfg domain.Config) *Engine {
	return &Engine{cfg: cfg}
}

// Run executes the full reconciliation and returns a completed Run.
// The returned Run contains all matches, all source metadata, and the summary.
// If a fatal error occurs, Run.Status is RunStatusFailed and Run.Error is set.
func (e *Engine) Run() domain.Run {
	run := domain.Run{
		ID:        e.cfg.Run.ID,
		StartedAt: time.Now().UTC(),
		Status:    domain.RunStatusRunning,
		Currency:  e.cfg.Run.Currency,
		Matches:   []domain.Match{},
	}

	matches, sources, parseErrors, err := e.reconcile()
	if err != nil {
		run.Status = domain.RunStatusFailed
		run.Error = err.Error()
		run.CompletedAt = time.Now().UTC()
		return run
	}

	run.Matches = matches
	run.Sources = sources
	run.Summary = computeSummary(matches, sources)
	run.Status = domain.RunStatusComplete
	run.CompletedAt = time.Now().UTC()
	run.ParseErrors = parseErrors

	return run
}

// reconcile is the core logic. Separated from Run() to keep error handling clean.
func (e *Engine) reconcile() ([]domain.Match, []domain.SourceMeta, []domain.ParseError, error) {

	var matches []domain.Match
	var sourceMetas []domain.SourceMeta
	var ingestErrors []ingest.RowError

	// Load all external sources and build finder indexes
	var externalPool []*domain.Transaction

	for _, srcCfg := range e.cfg.Sources {
		if srcCfg.Role != domain.SourceRoleExternal {
			continue
		}

		result, err := ingest.LoadCSV(srcCfg, e.cfg.Run.Currency)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load external source %q: %w", srcCfg.Name, err)
		}

		sourceMetas = append(sourceMetas, result.Meta)
		ingestErrors = append(ingestErrors, result.Errors...)
		externalPool = append(externalPool, result.Transactions...)
	}

	// Build finders from the external pool.
	// Rule chain: 1 → 2 → 3
	rules := []domain.Rule{
		{
			Name:       RuleReferenceMatch,
			Confidence: ConfidenceReferenceMatch,
			Finder:     finder.NewReferenceFinder(externalPool),
		},
		{
			Name:       RuleWeakKeyMatch,
			Confidence: ConfidenceWeakKeyMatch,
			Finder:     finder.NewWeakKeyFinder(externalPool),
		},
		{
			Name:       RuleAmountWindow,
			Confidence: ConfidenceAmountWindow,
			Finder:     finder.NewAmountWindowFinder(externalPool, e.cfg.Run.FeeTolerancePercent, e.cfg.Run.TimeWindowSeconds),
		},
	}

	// claimed tracks which external transactions have already been matched.
	// key: pointer to external Transaction, value: the internal tx that claimed it.
	// Used to detect DuplicateInternal — when two internal txs match the same external tx.
	claimed := make(map[*domain.Transaction]struct{})

	// Stream all internal sources, match each transaction
	for _, srcCfg := range e.cfg.Sources {
		if srcCfg.Role != domain.SourceRoleInternal {
			continue
		}

		streamResult, err := ingest.StreamCSV(srcCfg, e.cfg.Run.Currency, func(tx *domain.Transaction) error {
			match := e.matchTransaction(tx, rules, claimed)
			matches = append(matches, match)
			return nil
		})

		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to stream internal source %q: %w", srcCfg.Name, err)
		}

		sourceMetas = append(sourceMetas, streamResult.Meta)
		ingestErrors = append(ingestErrors, streamResult.Errors...)
	}

	// Flag unclaimed external transactions as MissingInternal
	for _, tx := range externalPool {
		if _, wasClaimed := claimed[tx]; !wasClaimed {
			matches = append(matches, domain.Match{
				External: tx,
				Outcome:  domain.OutcomeMissingInternal,
				Rule:     "",
				// No confidence — absence of a match is not a rule firing.
				// No variance — we have no internal transaction to compare against.
			})
		}
	}

	// Append ingest errors as MissingExternal matches so they appear in the report.
	// These are rows that failed to parse — they couldn't participate in matching.
	var parseErrors []domain.ParseError
	for _, rowErr := range ingestErrors {
		parseErrors = append(parseErrors, toParseError(rowErr))
	}
	return matches, sourceMetas, parseErrors, nil

}

// matchTransaction runs a single internal transaction through the rule chain.
// Returns the best match found, or a MissingExternal match if no rule fires.
func (e *Engine) matchTransaction(tx *domain.Transaction, rules []domain.Rule,
	claimed map[*domain.Transaction]struct{}) domain.Match {
	for _, rule := range rules {
		candidates := rule.Finder.FindCandidates(tx)
		if len(candidates) == 0 {
			continue
		}

		// Single candidate — attempt to claim it.
		if len(candidates) == 1 {
			candidate := candidates[0]

			// Check if this external transaction was already claimed.
			if claimedBy, exists := claimed[candidate]; exists {
				// Two internal transactions matched the same external transaction.
				// Retroactively update the first match to DuplicateInternal.
				// Mark the current tx as DuplicateInternal too.
				_ = claimedBy // used for audit trail in future — (TODO: update prior match:femi)

				return domain.Match{
					Internal:   tx,
					External:   candidate,
					Outcome:    domain.OutcomeDuplicateInternal,
					Rule:       rule.Name,
					Confidence: rule.Confidence,
					Variance:   tx.AmountMinorUnits - candidate.AmountMinorUnits,
					Candidates: candidates,
				}
			}

			// Claim it.
			claimed[candidate] = struct{}{}

			return domain.Match{
				Internal:   tx,
				External:   candidate,
				Outcome:    domain.OutcomeExactMatch,
				Rule:       rule.Name,
				Confidence: rule.Confidence,
				Variance:   tx.AmountMinorUnits - candidate.AmountMinorUnits,
			}
		}

		// Multiple candidates — ambiguous. Cannot confidently pick one.
		// Check if any candidates are already claimed — filter them out first.
		var unclaimed []*domain.Transaction
		for _, c := range candidates {
			if _, exists := claimed[c]; !exists {
				unclaimed = append(unclaimed, c)
			}
		}

		if len(unclaimed) == 0 {
			// All candidates already claimed — treat as missing.
			continue
		}

		if len(unclaimed) == 1 {
			// Only one unclaimed candidate left — claim it.
			claimed[unclaimed[0]] = struct{}{}
			return domain.Match{
				Internal:   tx,
				External:   unclaimed[0],
				Outcome:    domain.OutcomeExactMatch,
				Rule:       rule.Name,
				Confidence: rule.Confidence,
				Variance:   tx.AmountMinorUnits - unclaimed[0].AmountMinorUnits,
			}
		}

		// Multiple unclaimed candidates — genuinely ambiguous.
		// Do not claim any. Surface all candidates for the auditor.
		return domain.Match{
			Internal:   tx,
			Outcome:    domain.OutcomeAmbiguousMatch,
			Rule:       rule.Name,
			Confidence: rule.Confidence,
			Candidates: unclaimed,
		}
	}

	// No rule produced any candidates — transaction is unmatched.
	return domain.Match{
		Internal: tx,
		Outcome:  domain.OutcomeMissingExternal,
	}
}

// computeSummary builds a RunSummary from the completed match slice and source metas.
func computeSummary(matches []domain.Match, sources []domain.SourceMeta) domain.RunSummary {
	var summary domain.RunSummary

	for _, src := range sources {
		if src.Role == domain.SourceRoleInternal {
			summary.TotalInternal += src.TotalAfterDedup
		} else {
			summary.TotalExternal += src.TotalAfterDedup
		}
	}

	for _, m := range matches {
		summary.TotalMatches++

		switch m.Outcome {
		case domain.OutcomeExactMatch:
			summary.ExactMatches++
		case domain.OutcomeAmbiguousMatch:
			summary.AmbiguousMatches++
		case domain.OutcomeDuplicateExternal:
			summary.DuplicateExternal++
		case domain.OutcomeDuplicateInternal:
			summary.DuplicateInternal++
		case domain.OutcomeMissingExternal:
			summary.MissingExternal++
		case domain.OutcomeMissingInternal:
			summary.MissingInternal++
		}

		summary.TotalVarianceMinor += m.Variance
	}

	if summary.TotalInternal > 0 {
		summary.MatchRatePercent = float64(summary.ExactMatches) / float64(summary.TotalInternal) * 100
	}

	return summary
}

func toParseError(e ingest.RowError) domain.ParseError {
	return domain.ParseError{
		SourceName: e.SourceName,
		SourceFile: e.SourceFile,
		RowNumber:  e.RowNumber,
		Reason:     e.Reason,
		RawRow:     e.RawRow,
	}
}
