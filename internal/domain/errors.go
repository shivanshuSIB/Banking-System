package domain

import "errors"

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrInvalidAmount       = errors.New("amount must be greater than zero")
	ErrSameAccount         = errors.New("source and destination accounts must differ")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrAlreadyReversed     = errors.New("transaction has already been reversed")
	ErrCannotReverseType   = errors.New("only SUCCESS transactions can be reversed")
	ErrIdempotentReplay    = errors.New("idempotency key already used")
)
