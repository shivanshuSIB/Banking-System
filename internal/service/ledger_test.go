package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"banking-system/internal/domain"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

// --- mock DB ----------------------------------------------------------------

type mockDB struct {
	beginTxFn func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (m *mockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return m.beginTxFn(ctx, opts)
}

// --- mock TransactionRepository ---------------------------------------------

type mockTxRepo struct {
	createFn              func(ctx context.Context, tx *sql.Tx, t *domain.Transaction) error
	updateStatusFn        func(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus, errMsg string) error
	getByIDFn             func(ctx context.Context, id uuid.UUID) (*domain.Transaction, error)
	getAllFn              func(ctx context.Context) ([]*domain.Transaction, error)
	getByIdempotencyKeyFn func(ctx context.Context, key string) (*domain.Transaction, error)
	createJournalEntryFn  func(ctx context.Context, tx *sql.Tx, e *domain.JournalEntry) error
	isReversedFn          func(ctx context.Context, originalID uuid.UUID) (bool, error)
	getJournalEntriesFn   func(ctx context.Context, txID uuid.UUID) ([]*domain.JournalEntry, error)
}

func (m *mockTxRepo) Create(ctx context.Context, tx *sql.Tx, t *domain.Transaction) error {
	if m.createFn != nil {
		return m.createFn(ctx, tx, t)
	}
	return nil
}
func (m *mockTxRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus, errMsg string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, tx, id, status, errMsg)
	}
	return nil
}
func (m *mockTxRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockTxRepo) GetAll(ctx context.Context) ([]*domain.Transaction, error) {
	return m.getAllFn(ctx)
}
func (m *mockTxRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	if m.getByIdempotencyKeyFn != nil {
		return m.getByIdempotencyKeyFn(ctx, key)
	}
	return nil, nil
}
func (m *mockTxRepo) CreateJournalEntry(ctx context.Context, tx *sql.Tx, e *domain.JournalEntry) error {
	if m.createJournalEntryFn != nil {
		return m.createJournalEntryFn(ctx, tx, e)
	}
	return nil
}
func (m *mockTxRepo) IsReversed(ctx context.Context, originalID uuid.UUID) (bool, error) {
	if m.isReversedFn != nil {
		return m.isReversedFn(ctx, originalID)
	}
	return false, nil
}
func (m *mockTxRepo) GetJournalEntriesByTransaction(ctx context.Context, txID uuid.UUID) ([]*domain.JournalEntry, error) {
	if m.getJournalEntriesFn != nil {
		return m.getJournalEntriesFn(ctx, txID)
	}
	return nil, nil
}

// --- Transfer tests ---------------------------------------------------------

func TestTransfer_InvalidAmount(t *testing.T) {
	svc := NewLedgerService(nil, nil, nil)
	_, err := svc.Transfer(context.Background(), domain.TransferRequest{Amount: 0})
	if !errors.Is(err, domain.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
	_, err = svc.Transfer(context.Background(), domain.TransferRequest{Amount: -100})
	if !errors.Is(err, domain.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount for negative, got %v", err)
	}
}

func TestTransfer_SameAccount(t *testing.T) {
	id := uuid.New()
	svc := NewLedgerService(nil, nil, nil)
	_, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromAccountID: id,
		ToAccountID:   id,
		Amount:        100,
	})
	if !errors.Is(err, domain.ErrSameAccount) {
		t.Errorf("expected ErrSameAccount, got %v", err)
	}
}

func TestTransfer_IdempotencyReplay(t *testing.T) {
	existing := &domain.Transaction{ID: uuid.New(), Status: domain.StatusSuccess}
	txRepo := &mockTxRepo{
		getByIdempotencyKeyFn: func(_ context.Context, key string) (*domain.Transaction, error) {
			if key == "dup-key" {
				return existing, nil
			}
			return nil, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	got, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromAccountID:  uuid.New(),
		ToAccountID:    uuid.New(),
		Amount:         500,
		IdempotencyKey: "dup-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != existing.ID {
		t.Error("expected existing transaction to be returned")
	}
}

// --- Deposit tests ----------------------------------------------------------

func TestDeposit_InvalidAmount(t *testing.T) {
	svc := NewLedgerService(nil, nil, nil)
	_, err := svc.Deposit(context.Background(), domain.DepositRequest{Amount: 0})
	if !errors.Is(err, domain.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestDeposit_IdempotencyReplay(t *testing.T) {
	existing := &domain.Transaction{ID: uuid.New(), Status: domain.StatusSuccess}
	txRepo := &mockTxRepo{
		getByIdempotencyKeyFn: func(_ context.Context, key string) (*domain.Transaction, error) {
			return existing, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	got, err := svc.Deposit(context.Background(), domain.DepositRequest{
		AccountID:      uuid.New(),
		Amount:         1000,
		IdempotencyKey: "dep-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != existing.ID {
		t.Error("expected idempotent replay")
	}
}

// --- Withdraw tests ---------------------------------------------------------

func TestWithdraw_InvalidAmount(t *testing.T) {
	svc := NewLedgerService(nil, nil, nil)
	_, err := svc.Withdraw(context.Background(), domain.WithdrawalRequest{Amount: -5})
	if !errors.Is(err, domain.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestWithdraw_IdempotencyReplay(t *testing.T) {
	existing := &domain.Transaction{ID: uuid.New(), Status: domain.StatusSuccess}
	txRepo := &mockTxRepo{
		getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) {
			return existing, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	got, err := svc.Withdraw(context.Background(), domain.WithdrawalRequest{
		AccountID:      uuid.New(),
		Amount:         200,
		IdempotencyKey: "wd-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != existing.ID {
		t.Error("expected idempotent replay")
	}
}

// --- Reverse tests ----------------------------------------------------------

func TestReverse_IdempotencyReplay(t *testing.T) {
	existing := &domain.Transaction{ID: uuid.New(), Status: domain.StatusSuccess}
	txRepo := &mockTxRepo{
		getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) {
			return existing, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	got, err := svc.Reverse(context.Background(), domain.ReversalRequest{
		TransactionID:  uuid.New(),
		IdempotencyKey: "rev-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != existing.ID {
		t.Error("expected idempotent replay")
	}
}

func TestReverse_TransactionNotFound(t *testing.T) {
	txRepo := &mockTxRepo{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
			return nil, domain.ErrTransactionNotFound
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	_, err := svc.Reverse(context.Background(), domain.ReversalRequest{
		TransactionID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestReverse_NotSuccessStatus(t *testing.T) {
	txRepo := &mockTxRepo{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
			return &domain.Transaction{Status: domain.StatusFailed}, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	_, err := svc.Reverse(context.Background(), domain.ReversalRequest{
		TransactionID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrCannotReverseType) {
		t.Errorf("expected ErrCannotReverseType, got %v", err)
	}
}

func TestReverse_AlreadyReversed(t *testing.T) {
	txRepo := &mockTxRepo{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
			return &domain.Transaction{Status: domain.StatusSuccess}, nil
		},
		isReversedFn: func(context.Context, uuid.UUID) (bool, error) {
			return true, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)

	_, err := svc.Reverse(context.Background(), domain.ReversalRequest{
		TransactionID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrAlreadyReversed) {
		t.Errorf("expected ErrAlreadyReversed, got %v", err)
	}
}

// --- GetTransaction / ListTransactions --------------------------------------

func TestGetTransaction(t *testing.T) {
	id := uuid.New()
	want := &domain.Transaction{ID: id, Type: domain.TypeDeposit}
	txRepo := &mockTxRepo{
		getByIDFn: func(_ context.Context, gotID uuid.UUID) (*domain.Transaction, error) {
			if gotID != id {
				t.Fatalf("unexpected ID")
			}
			return want, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)
	got, err := svc.GetTransaction(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID {
		t.Error("ID mismatch")
	}
}

func TestListTransactions(t *testing.T) {
	txs := []*domain.Transaction{{ID: uuid.New()}, {ID: uuid.New()}}
	txRepo := &mockTxRepo{
		getAllFn: func(context.Context) ([]*domain.Transaction, error) {
			return txs, nil
		},
	}
	svc := NewLedgerService(nil, nil, txRepo)
	got, err := svc.ListTransactions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

// --- helper unit tests ------------------------------------------------------

func TestNullUUID(t *testing.T) {
	if nullUUID(nil) != nil {
		t.Error("expected nil")
	}
	id := uuid.New()
	got := nullUUID(&id)
	if got != id {
		t.Errorf("expected %v, got %v", id, got)
	}
}

func TestNullableStr(t *testing.T) {
	if nullableStr(nil) != nil {
		t.Error("expected nil for nil ptr")
	}
	empty := ""
	if nullableStr(&empty) != nil {
		t.Error("expected nil for empty string")
	}
	val := "hello"
	if nullableStr(&val) != "hello" {
		t.Errorf("expected 'hello', got %v", nullableStr(&val))
	}
}

// --- sqlmock helpers --------------------------------------------------------

func newMockTx(t *testing.T) (*sql.Tx, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectCommit()
	return tx, func() { db.Close() }
}

func newMockTxForRollback(t *testing.T) (*sql.Tx, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectRollback()
	return tx, func() { db.Close() }
}

func mockAccountRepoOK(balance int64) *mockAccountRepo {
	return &mockAccountRepo{
		getByIDForUpdFn: func(_ context.Context, _ *sql.Tx, id uuid.UUID) (*domain.Account, error) {
			return &domain.Account{ID: id, Balance: balance}, nil
		},
		updateBalanceFn: func(context.Context, *sql.Tx, uuid.UUID, int64) error { return nil },
	}
}

// --- Transfer full-flow tests -----------------------------------------------

func TestTransfer_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{},
	)
	result, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromAccountID: uuid.New(), ToAccountID: uuid.New(), Amount: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess || result.Type != domain.TypeTransfer || result.Amount != 1000 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestTransfer_WithIdempotencyKey(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) { return nil, nil }},
	)
	result, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromAccountID: uuid.New(), ToAccountID: uuid.New(), Amount: 500, IdempotencyKey: "xfer-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotencyKey == nil || *result.IdempotencyKey != "xfer-1" {
		t.Error("idempotency key not set on result")
	}
}

func TestTransfer_BeginTxError(t *testing.T) {
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) {
			return nil, fmt.Errorf("connection refused")
		}},
		nil, &mockTxRepo{},
	)
	_, err := svc.Transfer(context.Background(), domain.TransferRequest{
		FromAccountID: uuid.New(), ToAccountID: uuid.New(), Amount: 100,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Deposit full-flow tests ------------------------------------------------

func TestDeposit_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(5000),
		&mockTxRepo{},
	)
	result, err := svc.Deposit(context.Background(), domain.DepositRequest{AccountID: uuid.New(), Amount: 500})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess || result.Type != domain.TypeDeposit {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestDeposit_WithIdempotencyKey(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(5000),
		&mockTxRepo{getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) { return nil, nil }},
	)
	result, err := svc.Deposit(context.Background(), domain.DepositRequest{
		AccountID: uuid.New(), Amount: 500, IdempotencyKey: "dep-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotencyKey == nil || *result.IdempotencyKey != "dep-1" {
		t.Error("idempotency key not set")
	}
}

func TestDeposit_BeginTxError(t *testing.T) {
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, errors.New("db down") }},
		nil, &mockTxRepo{},
	)
	_, err := svc.Deposit(context.Background(), domain.DepositRequest{AccountID: uuid.New(), Amount: 100})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Withdraw full-flow tests -----------------------------------------------

func TestWithdraw_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(5000),
		&mockTxRepo{},
	)
	result, err := svc.Withdraw(context.Background(), domain.WithdrawalRequest{AccountID: uuid.New(), Amount: 300})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess || result.Type != domain.TypeWithdrawal {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestWithdraw_WithIdempotencyKey(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(5000),
		&mockTxRepo{getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) { return nil, nil }},
	)
	result, err := svc.Withdraw(context.Background(), domain.WithdrawalRequest{
		AccountID: uuid.New(), Amount: 100, IdempotencyKey: "wd-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotencyKey == nil || *result.IdempotencyKey != "wd-1" {
		t.Error("idempotency key not set")
	}
}

func TestWithdraw_BeginTxError(t *testing.T) {
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, errors.New("db down") }},
		nil, &mockTxRepo{},
	)
	_, err := svc.Withdraw(context.Background(), domain.WithdrawalRequest{AccountID: uuid.New(), Amount: 100})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Reverse full-flow tests ------------------------------------------------

func TestReverse_Transfer_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	fromID, toID, originalID := uuid.New(), uuid.New(), uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{
					ID: originalID, Type: domain.TypeTransfer, Status: domain.StatusSuccess,
					Amount: 1000, FromAccountID: &fromID, ToAccountID: &toID,
				}, nil
			},
		},
	)
	result, err := svc.Reverse(context.Background(), domain.ReversalRequest{TransactionID: originalID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess || result.Type != domain.TypeReversal {
		t.Errorf("unexpected result: %+v", result)
	}
	if result.ReversalOf == nil || *result.ReversalOf != originalID {
		t.Error("reversal_of not set")
	}
}

func TestReverse_Deposit_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	acctID, originalID := uuid.New(), uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{
					ID: originalID, Type: domain.TypeDeposit, Status: domain.StatusSuccess,
					Amount: 500, ToAccountID: &acctID,
				}, nil
			},
		},
	)
	result, err := svc.Reverse(context.Background(), domain.ReversalRequest{TransactionID: originalID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess {
		t.Errorf("expected SUCCESS, got %s", result.Status)
	}
}

func TestReverse_Withdrawal_Success(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	acctID, originalID := uuid.New(), uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{
					ID: originalID, Type: domain.TypeWithdrawal, Status: domain.StatusSuccess,
					Amount: 300, FromAccountID: &acctID,
				}, nil
			},
		},
	)
	result, err := svc.Reverse(context.Background(), domain.ReversalRequest{TransactionID: originalID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.StatusSuccess {
		t.Errorf("expected SUCCESS, got %s", result.Status)
	}
}

func TestReverse_UnsupportedType(t *testing.T) {
	tx, cleanup := newMockTxForRollback(t)
	defer cleanup()

	originalID := uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		nil,
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{
					ID: originalID, Type: domain.TypeReversal, Status: domain.StatusSuccess, Amount: 100,
				}, nil
			},
		},
	)
	_, err := svc.Reverse(context.Background(), domain.ReversalRequest{TransactionID: originalID})
	if !errors.Is(err, domain.ErrCannotReverseType) {
		t.Errorf("expected ErrCannotReverseType, got %v", err)
	}
}

func TestReverse_BeginTxError(t *testing.T) {
	originalID := uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, errors.New("db down") }},
		nil,
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{ID: originalID, Status: domain.StatusSuccess, Amount: 100}, nil
			},
		},
	)
	_, err := svc.Reverse(context.Background(), domain.ReversalRequest{TransactionID: originalID})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReverse_WithIdempotencyKey(t *testing.T) {
	tx, cleanup := newMockTx(t)
	defer cleanup()

	acctID, originalID := uuid.New(), uuid.New()
	svc := NewLedgerService(
		&mockDB{beginTxFn: func(context.Context, *sql.TxOptions) (*sql.Tx, error) { return tx, nil }},
		mockAccountRepoOK(10000),
		&mockTxRepo{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Transaction, error) {
				return &domain.Transaction{
					ID: originalID, Type: domain.TypeDeposit, Status: domain.StatusSuccess,
					Amount: 500, ToAccountID: &acctID,
				}, nil
			},
			getByIdempotencyKeyFn: func(context.Context, string) (*domain.Transaction, error) { return nil, nil },
		},
	)
	result, err := svc.Reverse(context.Background(), domain.ReversalRequest{
		TransactionID: originalID, IdempotencyKey: "rev-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotencyKey == nil || *result.IdempotencyKey != "rev-1" {
		t.Error("idempotency key not set")
	}
}
