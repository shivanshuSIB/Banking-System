package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"banking-system/internal/domain"

	"github.com/google/uuid"
)

// AccountRepo implements repository.AccountRepository against PostgreSQL.
type AccountRepo struct {
	db *sql.DB
}

func NewAccountRepo(db *sql.DB) *AccountRepo {
	return &AccountRepo{db: db}
}

func (r *AccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	const q = `
		SELECT id, name, currency, balance, created_at, updated_at
		FROM accounts WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, id)
	return scanAccount(row)
}

func (r *AccountRepo) GetAll(ctx context.Context) ([]*domain.Account, error) {
	const q = `
		SELECT id, name, currency, balance, created_at, updated_at
		FROM accounts ORDER BY created_at`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (r *AccountRepo) Create(ctx context.Context, name, currency string) (*domain.Account, error) {
	const q = `
		INSERT INTO accounts (name, currency)
		VALUES ($1, $2)
		RETURNING id, name, currency, balance, created_at, updated_at`
	row := r.db.QueryRowContext(ctx, q, name, currency)
	return scanAccount(row)
}

// GetByIDForUpdate locks the row with SELECT FOR UPDATE inside an existing transaction.
func (r *AccountRepo) GetByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error) {
	const q = `
		SELECT id, name, currency, balance, created_at, updated_at
		FROM accounts WHERE id = $1 FOR UPDATE`
	row := tx.QueryRowContext(ctx, q, id)
	return scanAccount(row)
}

// UpdateBalance applies a signed delta (positive = add, negative = subtract) to an account's balance.
// The DB CHECK constraint (balance >= 0) will reject overdrafts.
func (r *AccountRepo) UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, delta int64) error {
	const q = `UPDATE accounts SET balance = balance + $1, updated_at = NOW() WHERE id = $2`
	res, err := tx.ExecContext(ctx, q, delta, id)
	if err != nil {
		return fmt.Errorf("update balance: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrAccountNotFound
	}
	return nil
}

// -- helpers -----------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccount(row rowScanner) (*domain.Account, error) {
	a := &domain.Account{}
	err := row.Scan(&a.ID, &a.Name, &a.Currency, &a.Balance, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, domain.ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan account: %w", err)
	}
	return a, nil
}

func scanAccountRow(rows *sql.Rows) (*domain.Account, error) {
	a := &domain.Account{}
	err := rows.Scan(&a.ID, &a.Name, &a.Currency, &a.Balance, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan account row: %w", err)
	}
	return a, nil
}
