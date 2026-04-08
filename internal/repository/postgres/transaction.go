package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"banking-system/internal/domain"

	"github.com/google/uuid"
)

// TransactionRepo implements repository.TransactionRepository against PostgreSQL.
type TransactionRepo struct {
	db *sql.DB
}

func NewTransactionRepo(db *sql.DB) *TransactionRepo {
	return &TransactionRepo{db: db}
}

func (r *TransactionRepo) Create(ctx context.Context, tx *sql.Tx, t *domain.Transaction) error {
	const q = `
		INSERT INTO transactions
			(id, type, status, amount, from_account_id, to_account_id, reversal_of, idempotency_key, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
	_, err := tx.ExecContext(ctx, q,
		t.ID, t.Type, t.Status, t.Amount,
		nullUUID(t.FromAccountID), nullUUID(t.ToAccountID), nullUUID(t.ReversalOf),
		nullString(t.IdempotencyKey), t.Note,
	)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	return nil
}

func (r *TransactionRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus, errMsg string) error {
	const q = `UPDATE transactions SET status=$1, error_message=$2 WHERE id=$3`
	_, err := tx.ExecContext(ctx, q, status, nullableStr(errMsg), id)
	return err
}

func (r *TransactionRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	const q = `
		SELECT id, type, status, amount, from_account_id, to_account_id, reversal_of,
		       idempotency_key, note, error_message, created_at
		FROM transactions WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanTransaction(row)
}

func (r *TransactionRepo) GetAll(ctx context.Context) ([]*domain.Transaction, error) {
	const q = `
		SELECT id, type, status, amount, from_account_id, to_account_id, reversal_of,
		       idempotency_key, note, error_message, created_at
		FROM transactions ORDER BY created_at DESC LIMIT 200`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	var txs []*domain.Transaction
	for rows.Next() {
		t, err := scanTransactionRow(rows)
		if err != nil {
			return nil, err
		}
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

func (r *TransactionRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	const q = `
		SELECT id, type, status, amount, from_account_id, to_account_id, reversal_of,
		       idempotency_key, note, error_message, created_at
		FROM transactions WHERE idempotency_key = $1`
	row := r.db.QueryRowContext(ctx, q, key)
	t, err := scanTransaction(row)
	if err == domain.ErrTransactionNotFound {
		return nil, nil // not found is OK here
	}
	return t, err
}

// IsReversed returns true if a SUCCESS reversal transaction already targets originalID.
func (r *TransactionRepo) IsReversed(ctx context.Context, originalID uuid.UUID) (bool, error) {
	const q = `SELECT COUNT(*) FROM transactions WHERE reversal_of=$1 AND status='SUCCESS'`
	var count int
	err := r.db.QueryRowContext(ctx, q, originalID).Scan(&count)
	return count > 0, err
}

func (r *TransactionRepo) CreateJournalEntry(ctx context.Context, tx *sql.Tx, e *domain.JournalEntry) error {
	const q = `
		INSERT INTO journal_entries (id, transaction_id, account_id, amount, direction)
		VALUES ($1,$2,$3,$4,$5)`
	_, err := tx.ExecContext(ctx, q, e.ID, e.TransactionID, e.AccountID, e.Amount, e.Direction)
	if err != nil {
		return fmt.Errorf("insert journal entry: %w", err)
	}
	return nil
}

func (r *TransactionRepo) GetJournalEntriesByTransaction(ctx context.Context, txID uuid.UUID) ([]*domain.JournalEntry, error) {
	const q = `
		SELECT id, transaction_id, account_id, amount, direction, created_at
		FROM journal_entries WHERE transaction_id=$1 ORDER BY created_at`
	rows, err := r.db.QueryContext(ctx, q, txID)
	if err != nil {
		return nil, fmt.Errorf("query journal entries: %w", err)
	}
	defer rows.Close()

	var entries []*domain.JournalEntry
	for rows.Next() {
		e := &domain.JournalEntry{}
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &e.Amount, &e.Direction, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan journal entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// -- helpers -----------------------------------------------------------------

func nullUUID(id *uuid.UUID) interface{} {
	if id == nil {
		return nil
	}
	return *id
}

func nullString(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func scanTransaction(row rowScanner) (*domain.Transaction, error) {
	t := &domain.Transaction{}
	var fromID, toID, reversalOf sql.NullString
	var idempKey, note, errMsg sql.NullString

	err := row.Scan(
		&t.ID, &t.Type, &t.Status, &t.Amount,
		&fromID, &toID, &reversalOf,
		&idempKey, &note, &errMsg, &t.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrTransactionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan transaction: %w", err)
	}

	if fromID.Valid {
		id, _ := uuid.Parse(fromID.String)
		t.FromAccountID = &id
	}
	if toID.Valid {
		id, _ := uuid.Parse(toID.String)
		t.ToAccountID = &id
	}
	if reversalOf.Valid {
		id, _ := uuid.Parse(reversalOf.String)
		t.ReversalOf = &id
	}
	if idempKey.Valid {
		t.IdempotencyKey = &idempKey.String
	}
	t.Note = note.String
	t.ErrorMessage = errMsg.String
	return t, nil
}

func scanTransactionRow(rows *sql.Rows) (*domain.Transaction, error) {
	t := &domain.Transaction{}
	var fromID, toID, reversalOf sql.NullString
	var idempKey, note, errMsg sql.NullString

	err := rows.Scan(
		&t.ID, &t.Type, &t.Status, &t.Amount,
		&fromID, &toID, &reversalOf,
		&idempKey, &note, &errMsg, &t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan transaction row: %w", err)
	}

	if fromID.Valid {
		id, _ := uuid.Parse(fromID.String)
		t.FromAccountID = &id
	}
	if toID.Valid {
		id, _ := uuid.Parse(toID.String)
		t.ToAccountID = &id
	}
	if reversalOf.Valid {
		id, _ := uuid.Parse(reversalOf.String)
		t.ReversalOf = &id
	}
	if idempKey.Valid {
		t.IdempotencyKey = &idempKey.String
	}
	t.Note = note.String
	t.ErrorMessage = errMsg.String
	return t, nil
}
