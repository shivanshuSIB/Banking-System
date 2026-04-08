package domain

import (
	"errors"
	"testing"
)

func TestDomainErrors(t *testing.T) {
	tests := []struct {
		err  error
		text string
	}{
		{ErrAccountNotFound, "account not found"},
		{ErrInsufficientFunds, "insufficient funds"},
		{ErrInvalidAmount, "amount must be greater than zero"},
		{ErrSameAccount, "source and destination accounts must differ"},
		{ErrTransactionNotFound, "transaction not found"},
		{ErrAlreadyReversed, "transaction has already been reversed"},
		{ErrCannotReverseType, "only SUCCESS transactions can be reversed"},
		{ErrIdempotentReplay, "idempotency key already used"},
	}
	for _, tt := range tests {
		if tt.err.Error() != tt.text {
			t.Errorf("error %q: got %q", tt.text, tt.err.Error())
		}
	}
}

func TestDomainErrorsAreDistinct(t *testing.T) {
	errs := []error{
		ErrAccountNotFound, ErrInsufficientFunds, ErrInvalidAmount,
		ErrSameAccount, ErrTransactionNotFound, ErrAlreadyReversed,
		ErrCannotReverseType, ErrIdempotentReplay,
	}
	for i := 0; i < len(errs); i++ {
		for j := i + 1; j < len(errs); j++ {
			if errors.Is(errs[i], errs[j]) {
				t.Errorf("%v should not match %v", errs[i], errs[j])
			}
		}
	}
}
