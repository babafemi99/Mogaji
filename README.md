# Mogaji

> *The Mogaji is a compound head who does not participate in daily commerce — but intervenes as an authoritative arbiter when financial records conflict, restoring systemic order through independent verification.*

**Mogaji is an open-source financial reconciliation engine written in Go.**

It deterministically compares internal ledgers against external payment provider and bank statements — finding where your money went, why your numbers don't match, and exactly which row caused the problem.

---

## The problem

Modern payment systems distribute money flow across your application, multiple PSPs (Paystack, Flutterwave, Moniepoint), card networks, and bank settlement rails. Each system is correct in isolation. As a whole, they drift.

Three things happen silently at scale:

**1. Partial commit drift** — a transaction completes at the provider but your internal ledger never gets written. Money moved. Your records don't know.

**2. Fee transformation drift** — providers settle net, not gross. Your internal system expected ₦500,000. Paystack settled ₦492,500 after fees. Over thousands of transactions, this becomes material revenue misstatement.

**3. Reconciliation debt** — at scale, finance teams fall back to CSV exports and VLOOKUP. Non-deterministic. Non-reproducible. Fails audits.

Mogaji fixes this without touching your production systems.

---

## How it works

Mogaji is a strictly **offline verification tool**. It never sits in your request path, never processes live payments, and cannot affect transaction correctness. It operates only on exported data.

```
CSV exports (internal ledger + provider statements)
        ↓
  Field mapping via YAML
        ↓
  Normalization (UTC, minor units, canonical struct)
        ↓
  Multi-pass deterministic matching engine
        ↓
  Auditable JSON report with full explanation tree
```

**Matching passes, in order:**

| Pass | Strategy | Confidence |
|------|-----------|------------|
| 1 | Reference ID match | 1.00 |
| 2 | Weak key (amount + currency + time window) | 0.85 |
| 3 | Amount window (amount + currency + ±Δt) | 0.70 |
| 4 | Fee-aware (amount within declared tolerance) | 0.60 |
| 5 | Manual review | 0.00 |

Every match result carries the rule that produced it, its confidence level, the variance in minor units, and a full audit trail back to the source CSV row.

---

## Status

🚧 **Under active development** — not yet ready for production use.

```
internal/
├── domain/
│   ├── transaction.go   ✓ canonical normalized transaction type
│   ├── match.go         ✓ match result and outcome classification
│   ├── run.go           ✓ run context, n-source support, summary
│   └── config.go        ✓ YAML config types and field mapping
└── ingest/
    └── csv.go           ✓ CSV ingestion, normalization, deduplication
```

Coming next: `finder/`, `engine/`, `report/`, CLI entrypoint.

---

## Configuration

Mogaji is configured via a YAML mapping file that describes your sources and how to read them. No code changes required to add a new provider.

```yaml
run:
  id: "2026-06-21-daily"
  currency: "NGN"
  time_window_seconds: 86400
  fee_tolerance_percent: 1.5

sources:
  - name: "internal_ledger"
    role: internal
    file: "ledger.csv"
    timezone: "Africa/Lagos"
    minor_units: true
    fields:
      reference_id: "txn_ref"
      amount: "amount"
      currency: "currency"
      timestamp: "created_at"
      status: "status"

  - name: "paystack"
    role: external
    file: "paystack_statement.csv"
    timezone: "UTC"
    minor_units: false
    decimal_places: 2
    fields:
      reference_id: "Transaction ID"
      amount: "Settled Amount"
      currency: "Currency"
      timestamp: "Transaction Date"
      status: "Status"

  - name: "flutterwave"
    role: external
    file: "flutterwave_report.csv"
    timezone: "UTC"
    minor_units: false
    decimal_places: 2
    fields:
      reference_id: "tx_ref"
      amount: "Amount Settled"
      currency: "Currency"
      timestamp: "Created At"
      status: "Status"
```

CSV column headers are matched with full normalization — casing, spaces, hyphens, and dots are all handled. `"Transaction ID"`, `"transaction_id"`, and `"TRANSACTION-ID"` all resolve to the same column.

---

## Usage

```bash
mogaji reconcile \
  --mapping mapping.yml \
  --out report.json
```

---

## Design principles

- **Deterministic** — given the same inputs, Mogaji always produces the same output. No randomness, no probabilistic matching.
- **Auditable** — every match result carries the rule that fired, the confidence level, and a trace back to the exact source CSV row.
- **Offline** — zero production latency impact. Engine failure cannot affect payment correctness.
- **Honest about uncertainty** — weak matches are classified as weak. Ambiguous candidates are surfaced, not silently resolved.
- **Integer money** — floating-point arithmetic is forbidden. All amounts are `int64` minor units internally.

---

## Contributing

Mogaji is in early development. Issues and PRs welcome once the core engine is stable.

---

## Name

Named after the traditional Yoruba **Mogaji** — a compound head who does not participate in daily commerce but intervenes as an authoritative arbiter when financial records conflict, restoring systemic order through independent verification.