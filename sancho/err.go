package sancho

import (
	"errors"
)

var (
	ErrAmountRequired    = errors.New("amount is required")
	ErrCurrencyRequired  = errors.New("currency is required")
	ErrTimeStampRequired = errors.New("timestamp is required")

	ErrSourceNameRequired     = errors.New("source name is required")
	ErrSourceRoleRequired     = errors.New("source role is required")
	ErrSourceFileRequired     = errors.New("source file is required")
	ErrSourceTimezoneRequired = errors.New("source timezone is required")
	ErrInvalidDecimalPlaces   = errors.New("invalid decimal places check config")
	ErrInvalidSourceRole      = errors.New("source role must be internal or external")
	EmptySourceMetaName       = errors.New("source meta name cannot be empty")

	ErrRunIDRequired          = errors.New("run.id is required")
	ErrRunCurrencyRequired    = errors.New("run.currency is required")
	ErrInternalSourceRequired = errors.New("at least one source with role: internal is required")
	ErrExternalSourceRequired = errors.New("at least one source with role: external is required")
)

var (
	// ErrNegativeAmount is returned when a CSV row contains a negative amount.
	// Mogaji does not support negative amounts — credits and debits are
	// separate rows in the source data.
	ErrNegativeAmount = errors.New("negative amount")

	// ErrInvalidMinorUnits is returned when a minor-units field cannot be
	// parsed as an integer.
	ErrInvalidMinorUnits = errors.New("invalid minor units")

	// ErrInvalidDecimalAmount is returned when a decimal amount field cannot
	// be parsed as a float.
	ErrInvalidDecimalAmount = errors.New("invalid decimal amount")

	// ErrUnrecognisedTimestamp is returned when a timestamp string does not
	// match any supported format.
	ErrUnrecognisedTimestamp = errors.New("unrecognised timestamp format")

	// ErrAmbiguousHeaders is returned when two CSV column headers normalize
	// to the same string.
	ErrAmbiguousHeaders = errors.New("ambiguous CSV headers")

	// ErrColumnNotFound is returned when a required column is missing from
	// the CSV headers.
	ErrColumnNotFound = errors.New("column not found")
)

var (
	ErrCurrencyMismatch       = errors.New("currency mismatch")
	ErrAmountColumnMissing    = errors.New("amount column missing")
	ErrTimestampColumnMissing = errors.New("timestamp column missing")
	ErrInvalidAmount          = errors.New("invalid amount")
	ErrInvalidTimestamp       = errors.New("invalid timestamp")

	ErrAmountColumnNotFound    = errors.New("amount column not found")
	ErrTimestampColumnNotFound = errors.New("timestamp column not found")

	ErrInvalidTimezone = errors.New("invalid timezone")
	ErrFileNotFound    = errors.New("file not found")

	ErrCSVReadError       = errors.New("CSV read error")
	ErrDuplicateReference = errors.New("duplicate reference_id")

	ErrNoSourcesDeclared = errors.New("no sources declared in config")
)
