package domain

import (
	"time"

	"github.com/google/uuid"
)

// TransactionType classifies the nature of a ledger movement.
type TransactionType string

const (
	TypeDeposit    TransactionType = "DEPOSIT"
	TypeWithdrawal TransactionType = "WITHDRAWAL"
	TypeTransfer   TransactionType = "TRANSFER"
	TypeReversal   TransactionType = "REVERSAL"
)

// TransactionStatus tracks whether a transaction succeeded or failed.
type TransactionStatus string

const (
	StatusPending TransactionStatus = "PENDING"
	StatusSuccess TransactionStatus = "SUCCESS"
	StatusFailed  TransactionStatus = "FAILED"
)

// Direction indicates the side of a journal entry.
type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// Account represents a ledger account.
// Balance is stored in the smallest currency unit (e.g. cents for USD).
type Account struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Currency  string    `json:"currency"`
	Balance   int64     `json:"balance"` // cents
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Transaction is the top-level record for any balance-changing operation.
type Transaction struct {
	ID             uuid.UUID         `json:"id"`
	Type           TransactionType   `json:"type"`
	Status         TransactionStatus `json:"status"`
	Amount         int64             `json:"amount"` // cents, always positive
	FromAccountID  *uuid.UUID        `json:"from_account_id,omitempty"`
	ToAccountID    *uuid.UUID        `json:"to_account_id,omitempty"`
	ReversalOf     *uuid.UUID        `json:"reversal_of,omitempty"`
	IdempotencyKey *string           `json:"idempotency_key,omitempty"`
	Note           string            `json:"note,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

// JournalEntry is one side of a double-entry bookkeeping record.
type JournalEntry struct {
	ID            uuid.UUID `json:"id"`
	TransactionID uuid.UUID `json:"transaction_id"`
	AccountID     uuid.UUID `json:"account_id"`
	Amount        int64     `json:"amount"`    // signed: negative=debit, positive=credit
	Direction     Direction `json:"direction"` // DEBIT | CREDIT
	CreatedAt     time.Time `json:"created_at"`
}

// TransferRequest is the input for a transfer operation.
type TransferRequest struct {
	FromAccountID  uuid.UUID `json:"from_account_id"`
	ToAccountID    uuid.UUID `json:"to_account_id"`
	Amount         int64     `json:"amount"` // cents, must be > 0
	Note           string    `json:"note"`
	IdempotencyKey string    `json:"idempotency_key"`
}

// DepositRequest is the input for a deposit (external money entering the system).
type DepositRequest struct {
	AccountID      uuid.UUID `json:"account_id"`
	Amount         int64     `json:"amount"` // cents, must be > 0
	Note           string    `json:"note"`
	IdempotencyKey string    `json:"idempotency_key"`
}

// WithdrawalRequest is the input for a withdrawal.
type WithdrawalRequest struct {
	AccountID      uuid.UUID `json:"account_id"`
	Amount         int64     `json:"amount"` // cents, must be > 0
	Note           string    `json:"note"`
	IdempotencyKey string    `json:"idempotency_key"`
}

// ReversalRequest undoes a completed transaction.
type ReversalRequest struct {
	TransactionID  uuid.UUID `json:"transaction_id"`
	Note           string    `json:"note"`
	IdempotencyKey string    `json:"idempotency_key"`
}
