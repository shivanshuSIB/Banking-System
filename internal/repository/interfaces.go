package repository

import (
	"context"
	"database/sql"

	"banking-system/internal/domain"

	"github.com/google/uuid"
)

// AccountRepository defines persistence operations for accounts.
type AccountRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetAll(ctx context.Context) ([]*domain.Account, error)
	Create(ctx context.Context, name, currency string) (*domain.Account, error)
	GetByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, delta int64) error
}

// TransactionRepository defines persistence operations for transactions and journal entries.
type TransactionRepository interface {
	Create(ctx context.Context, tx *sql.Tx, t *domain.Transaction) error
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus, errMsg string) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error)
	GetAll(ctx context.Context) ([]*domain.Transaction, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error)
	CreateJournalEntry(ctx context.Context, tx *sql.Tx, e *domain.JournalEntry) error
	IsReversed(ctx context.Context, originalID uuid.UUID) (bool, error)
	GetJournalEntriesByTransaction(ctx context.Context, txID uuid.UUID) ([]*domain.JournalEntry, error)
}

// DB abstracts the database connection so services can begin transactions.
type DB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}
