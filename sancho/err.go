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
