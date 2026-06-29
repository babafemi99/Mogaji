# Mogaji


> *The Mogaji is a compound head who does not participate in daily commerce — but intervenes as an authoritative arbiter when financial records conflict, restoring systemic order through independent verification.*

**Mogaji is an open-source financial reconciliation engine written in Go.**

It deterministically compares internal ledgers against external payment provider and bank statements — finding where your money went, why your numbers don't match, and exactly which row caused the problem.

---

<p align="center">
  <img src="assets/logo.png" alt="Graph Asset" width="200">
</p>

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
| 4 | Manual review | 0.00 |

Every match result carries the rule that produced it, its confidence level, the variance in minor units, and a full audit trail back to the source CSV row.

---

## Quick start

### 1. Install

**From source (requires Go 1.22+):**

```bash
git clone https://github.com/babafemi99/Mogaji
cd Mogaji
go build -o mogaji ./cmd/mogaji
```

**Verify the installation:**

```bash
./mogaji --help
./mogaji version
```

### 2. Try the example

The `examples/` folder contains ready-to-run sample data:

```bash
./mogaji reconcile \
  --mapping examples/paystack-simple/mapping.yml \
  --out report.json \
  --verbose
```

This runs a reconciliation against a sample Paystack settlement with 4 internal transactions and 4 external transactions. You will see:

- 3 exact matches (with fee variance)
- 1 missing external (partial commit drift)
- 1 missing internal (provider has it, your ledger doesn't)

### 3. Run against your own data

Export your data as CSV files and create a mapping file:

```bash
./mogaji reconcile \
  --mapping your-mapping.yml \
  --out report.json \
  --verbose
```

---

## Configuration

Mogaji is configured via a YAML mapping file. No code changes are required to add a new provider — just describe your CSV columns.

```yaml
run:
  id: "2026-06-21-daily"
  currency: "NGN"
  time_window_seconds: 86400      # ±24h for time-bound matching
  fee_tolerance_percent: 1.5      # for fee-aware matching

sources:
  - name: "internal_ledger"
    role: internal
    file: "ledger.csv"            # path relative to this mapping file
    timezone: "Africa/Lagos"
    minor_units: false            # amounts are in naira, not kobo
    decimal_places: 2
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
```

**Multiple external sources are supported.** Add as many sources as you need — Paystack, Flutterwave, Moniepoint, bank statements — in a single run.

CSV column headers are matched with full normalization. `"Transaction ID"`, `"transaction_id"`, and `"TRANSACTION-ID"` all resolve to the same column.

---

## Output

Mogaji produces a JSON report with a full audit trail:

```json
{
  "version": "1.0",
  "run": {
    "id": "2026-06-21-daily",
    "status": "COMPLETE",
    "currency": "NGN",
    "summary": {
      "total_internal": 3,
      "total_external": 2,
      "exact_matches": 2,
      "missing_external": 1,
      "missing_internal": 0,
      "total_variance_minor_units": 11250,
      "match_rate_percent": 66.67
    },
    "matches": [
      {
        "internal": { "reference_id": "ONY_001", "amount_minor_units": 500000 },
        "external": { "reference_id": "ONY_001", "amount_minor_units": 492500 },
        "outcome": "EXACT_MATCH",
        "rule": "REFERENCE_MATCH",
        "confidence": 1,
        "variance": 7500
      }
    ]
  }
}
```

Every match includes the rule that fired, the confidence level, the variance in minor units, and a trace back to the exact source CSV row number.

---

## Examples

The `examples/` folder contains ready-to-run scenarios:

| Example | What it demonstrates |
|---------|---------------------|
| `examples/paystack-simple/` | Basic Paystack reconciliation with fee variance, missing external, and missing internal |

More examples coming — Flutterwave, Moniepoint, multi-source runs.

---

## Design principles

- **Deterministic** — same inputs always produce the same output. No randomness, no probabilistic matching.
- **Auditable** — every match carries the rule that fired, the confidence level, and a trace to the exact source CSV row.
- **Offline** — zero production latency impact. Engine failure cannot affect payment correctness.
- **Honest about uncertainty** — weak matches are classified as weak. Ambiguous candidates are surfaced, not silently resolved.
- **Integer money** — floating-point is forbidden. All amounts are `int64` minor units internally.
- **Memory efficient** — external sources are indexed, internal sources are streamed row by row. Peak memory is proportional to the smaller side.

---

## Status

🚧 **Under active development** — not yet ready for production use.

---

## Contributing

Mogaji is in early development. Issues and PRs are welcome.

```bash
# Run the test suite
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

---

## Name

Named after the traditional Yoruba **Mogaji** — a compound head who does not participate in daily commerce but intervenes as an authoritative arbiter when financial records conflict, restoring systemic order through independent verification.
