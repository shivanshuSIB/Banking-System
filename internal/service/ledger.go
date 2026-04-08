package service

import (
	"context"
	"database/sql"
	"fmt"

	"banking-system/internal/domain"
	"banking-system/internal/repository"

	"github.com/google/uuid"
)

// LedgerService handles all balance-changing operations with double-entry bookkeeping.
// It uses pessimistic locking (SELECT FOR UPDATE) to prevent races under concurrency.
type LedgerService struct {
	db       repository.DB
	accounts repository.AccountRepository
	txRepo   repository.TransactionRepository
}

func NewLedgerService(
	db repository.DB,
	accounts repository.AccountRepository,
	txRepo repository.TransactionRepository,
) *LedgerService {
	return &LedgerService{db: db, accounts: accounts, txRepo: txRepo}
}

// Transfer moves amount from one account to another atomically.
func (s *LedgerService) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.Transaction, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}
	if req.FromAccountID == req.ToAccountID {
		return nil, domain.ErrSameAccount
	}

	// Idempotency check (outside the main tx to keep it simple).
	if req.IdempotencyKey != "" {
		if existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	txRecord := &domain.Transaction{
		ID:            uuid.New(),
		Type:          domain.TypeTransfer,
		Status:        domain.StatusPending,
		Amount:        req.Amount,
		FromAccountID: &req.FromAccountID,
		ToAccountID:   &req.ToAccountID,
		Note:          req.Note,
	}
	if req.IdempotencyKey != "" {
		txRecord.IdempotencyKey = &req.IdempotencyKey
	}

	if err := s.txRepo.Create(ctx, tx, txRecord); err != nil {
		return nil, fmt.Errorf("create transaction record: %w", err)
	}

	// Lock both accounts in a consistent order to avoid deadlocks.
	first, second := req.FromAccountID, req.ToAccountID
	if first.String() > second.String() {
		first, second = second, first
	}

	if _, err := s.accounts.GetByIDForUpdate(ctx, tx, first); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}
	if _, err := s.accounts.GetByIDForUpdate(ctx, tx, second); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	// Fetch source balance to check funds.
	src, err := s.accounts.GetByIDForUpdate(ctx, tx, req.FromAccountID)
	if err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}
	if src.Balance < req.Amount {
		err = domain.ErrInsufficientFunds
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	// Apply balance changes.
	if err := s.accounts.UpdateBalance(ctx, tx, req.FromAccountID, -req.Amount); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}
	if err := s.accounts.UpdateBalance(ctx, tx, req.ToAccountID, req.Amount); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	// Write double-entry journal entries.
	if err := s.writeJournalPair(ctx, tx, txRecord.ID, req.FromAccountID, req.ToAccountID, req.Amount); err != nil {
		return txRecord, err
	}

	if err := s.txRepo.UpdateStatus(ctx, tx, txRecord.ID, domain.StatusSuccess, ""); err != nil {
		return txRecord, err
	}

	if err := tx.Commit(); err != nil {
		return txRecord, fmt.Errorf("commit: %w", err)
	}

	txRecord.Status = domain.StatusSuccess
	return txRecord, nil
}

// Deposit adds external funds to an account.
func (s *LedgerService) Deposit(ctx context.Context, req domain.DepositRequest) (*domain.Transaction, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	if req.IdempotencyKey != "" {
		if existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	txRecord := &domain.Transaction{
		ID:          uuid.New(),
		Type:        domain.TypeDeposit,
		Status:      domain.StatusPending,
		Amount:      req.Amount,
		ToAccountID: &req.AccountID,
		Note:        req.Note,
	}
	if req.IdempotencyKey != "" {
		txRecord.IdempotencyKey = &req.IdempotencyKey
	}

	if err := s.txRepo.Create(ctx, tx, txRecord); err != nil {
		return nil, err
	}

	if _, err := s.accounts.GetByIDForUpdate(ctx, tx, req.AccountID); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	if err := s.accounts.UpdateBalance(ctx, tx, req.AccountID, req.Amount); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	entry := &domain.JournalEntry{
		ID:            uuid.New(),
		TransactionID: txRecord.ID,
		AccountID:     req.AccountID,
		Amount:        req.Amount,
		Direction:     domain.DirectionCredit,
	}
	if err := s.txRepo.CreateJournalEntry(ctx, tx, entry); err != nil {
		return txRecord, err
	}

	if err := s.txRepo.UpdateStatus(ctx, tx, txRecord.ID, domain.StatusSuccess, ""); err != nil {
		return txRecord, err
	}

	if err := tx.Commit(); err != nil {
		return txRecord, fmt.Errorf("commit: %w", err)
	}

	txRecord.Status = domain.StatusSuccess
	return txRecord, nil
}

// Withdraw removes funds from an account.
func (s *LedgerService) Withdraw(ctx context.Context, req domain.WithdrawalRequest) (*domain.Transaction, error) {
	if req.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}

	if req.IdempotencyKey != "" {
		if existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	txRecord := &domain.Transaction{
		ID:            uuid.New(),
		Type:          domain.TypeWithdrawal,
		Status:        domain.StatusPending,
		Amount:        req.Amount,
		FromAccountID: &req.AccountID,
		Note:          req.Note,
	}
	if req.IdempotencyKey != "" {
		txRecord.IdempotencyKey = &req.IdempotencyKey
	}

	if err := s.txRepo.Create(ctx, tx, txRecord); err != nil {
		return nil, err
	}

	acct, err := s.accounts.GetByIDForUpdate(ctx, tx, req.AccountID)
	if err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}
	if acct.Balance < req.Amount {
		err = domain.ErrInsufficientFunds
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	if err := s.accounts.UpdateBalance(ctx, tx, req.AccountID, -req.Amount); err != nil {
		_ = s.failTransaction(ctx, tx, txRecord, err)
		return txRecord, err
	}

	entry := &domain.JournalEntry{
		ID:            uuid.New(),
		TransactionID: txRecord.ID,
		AccountID:     req.AccountID,
		Amount:        -req.Amount,
		Direction:     domain.DirectionDebit,
	}
	if err := s.txRepo.CreateJournalEntry(ctx, tx, entry); err != nil {
		return txRecord, err
	}

	if err := s.txRepo.UpdateStatus(ctx, tx, txRecord.ID, domain.StatusSuccess, ""); err != nil {
		return txRecord, err
	}

	if err := tx.Commit(); err != nil {
		return txRecord, fmt.Errorf("commit: %w", err)
	}

	txRecord.Status = domain.StatusSuccess
	return txRecord, nil
}

// Reverse undoes a completed transaction by creating an inverse one.
// Idempotent: calling it twice with the same original ID has no further effect.
func (s *LedgerService) Reverse(ctx context.Context, req domain.ReversalRequest) (*domain.Transaction, error) {
	// Idempotency check.
	if req.IdempotencyKey != "" {
		if existing, err := s.txRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}

	original, err := s.txRepo.GetByID(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}
	if original.Status != domain.StatusSuccess {
		return nil, domain.ErrCannotReverseType
	}

	// Check if already reversed.
	reversed, err := s.txRepo.IsReversed(ctx, req.TransactionID)
	if err != nil {
		return nil, err
	}
	if reversed {
		return nil, domain.ErrAlreadyReversed
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	reversalRecord := &domain.Transaction{
		ID:            uuid.New(),
		Type:          domain.TypeReversal,
		Status:        domain.StatusPending,
		Amount:        original.Amount,
		FromAccountID: original.FromAccountID,
		ToAccountID:   original.ToAccountID,
		ReversalOf:    &req.TransactionID,
		Note:          req.Note,
	}
	if req.IdempotencyKey != "" {
		reversalRecord.IdempotencyKey = &req.IdempotencyKey
	}

	if err := s.txRepo.Create(ctx, tx, reversalRecord); err != nil {
		return nil, err
	}

	// Invert the original operation.
	switch original.Type {
	case domain.TypeTransfer:
		// Lock accounts in consistent order.
		first, second := *original.FromAccountID, *original.ToAccountID
		if first.String() > second.String() {
			first, second = second, first
		}
		if _, err := s.accounts.GetByIDForUpdate(ctx, tx, first); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if _, err := s.accounts.GetByIDForUpdate(ctx, tx, second); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}

		// Check destination still has enough funds.
		dst, err := s.accounts.GetByIDForUpdate(ctx, tx, *original.ToAccountID)
		if err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if dst.Balance < original.Amount {
			err = domain.ErrInsufficientFunds
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}

		// Swap the direction.
		if err := s.accounts.UpdateBalance(ctx, tx, *original.ToAccountID, -original.Amount); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if err := s.accounts.UpdateBalance(ctx, tx, *original.FromAccountID, original.Amount); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if err := s.writeJournalPair(ctx, tx, reversalRecord.ID, *original.ToAccountID, *original.FromAccountID, original.Amount); err != nil {
			return reversalRecord, err
		}

	case domain.TypeDeposit:
		acct, err := s.accounts.GetByIDForUpdate(ctx, tx, *original.ToAccountID)
		if err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if acct.Balance < original.Amount {
			err = domain.ErrInsufficientFunds
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if err := s.accounts.UpdateBalance(ctx, tx, *original.ToAccountID, -original.Amount); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		entry := &domain.JournalEntry{
			ID: uuid.New(), TransactionID: reversalRecord.ID,
			AccountID: *original.ToAccountID, Amount: -original.Amount, Direction: domain.DirectionDebit,
		}
		if err := s.txRepo.CreateJournalEntry(ctx, tx, entry); err != nil {
			return reversalRecord, err
		}

	case domain.TypeWithdrawal:
		if _, err := s.accounts.GetByIDForUpdate(ctx, tx, *original.FromAccountID); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		if err := s.accounts.UpdateBalance(ctx, tx, *original.FromAccountID, original.Amount); err != nil {
			_ = s.failTransaction(ctx, tx, reversalRecord, err)
			return reversalRecord, err
		}
		entry := &domain.JournalEntry{
			ID: uuid.New(), TransactionID: reversalRecord.ID,
			AccountID: *original.FromAccountID, Amount: original.Amount, Direction: domain.DirectionCredit,
		}
		if err := s.txRepo.CreateJournalEntry(ctx, tx, entry); err != nil {
			return reversalRecord, err
		}

	default:
		return nil, domain.ErrCannotReverseType
	}

	if err := s.txRepo.UpdateStatus(ctx, tx, reversalRecord.ID, domain.StatusSuccess, ""); err != nil {
		return reversalRecord, err
	}

	if err := tx.Commit(); err != nil {
		return reversalRecord, fmt.Errorf("commit: %w", err)
	}

	reversalRecord.Status = domain.StatusSuccess
	return reversalRecord, nil
}

func (s *LedgerService) GetTransaction(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	return s.txRepo.GetByID(ctx, id)
}

func (s *LedgerService) ListTransactions(ctx context.Context) ([]*domain.Transaction, error) {
	return s.txRepo.GetAll(ctx)
}

// -- helpers -----------------------------------------------------------------

// writeJournalPair writes a debit entry for src and a credit entry for dst.
func (s *LedgerService) writeJournalPair(ctx context.Context, tx *sql.Tx, txID, srcID, dstID uuid.UUID, amount int64) error {
	debit := &domain.JournalEntry{
		ID:            uuid.New(),
		TransactionID: txID,
		AccountID:     srcID,
		Amount:        -amount,
		Direction:     domain.DirectionDebit,
	}
	credit := &domain.JournalEntry{
		ID:            uuid.New(),
		TransactionID: txID,
		AccountID:     dstID,
		Amount:        amount,
		Direction:     domain.DirectionCredit,
	}
	if err := s.txRepo.CreateJournalEntry(ctx, tx, debit); err != nil {
		return err
	}
	return s.txRepo.CreateJournalEntry(ctx, tx, credit)
}

// failTransaction marks a transaction as failed and persists a FAILED row for auditing.
//
// The caller still has an open tx that may hold:
//   - an uncommitted INSERT into transactions (PENDING), and
//   - FOR UPDATE locks on accounts.
// If we opened failTx and tried to INSERT/UPSERT the same transaction id while that tx
// is still open, the second session blocks on the first's row lock while the first waits
// inside failTransaction → self-deadlock (seen as pq: deadlock detected on later queries).
//
// So we roll back the main tx first to release locks and drop the uncommitted PENDING row,
// then write the FAILED audit row on a new connection.
func (s *LedgerService) failTransaction(ctx context.Context, tx *sql.Tx, t *domain.Transaction, cause error) error {
	t.Status = domain.StatusFailed
	t.ErrorMessage = cause.Error()

	_ = tx.Rollback() // release account locks and uncommitted transaction row before failTx

	failTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer failTx.Rollback() //nolint:errcheck

	// Upsert the transaction as FAILED (it may not exist yet if Create failed).
	const q = `
		INSERT INTO transactions (id, type, status, amount, from_account_id, to_account_id, reversal_of, idempotency_key, note, error_message)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET status='FAILED', error_message=EXCLUDED.error_message`
	_, execErr := failTx.ExecContext(ctx, q,
		t.ID, t.Type, domain.StatusFailed, t.Amount,
		nullUUID(t.FromAccountID), nullUUID(t.ToAccountID), nullUUID(t.ReversalOf),
		nullableStr(t.IdempotencyKey), t.Note, cause.Error(),
	)
	if execErr != nil {
		return execErr
	}
	return failTx.Commit()
}

func nullUUID(id *uuid.UUID) interface{} {
	if id == nil {
		return nil
	}
	return *id
}

func nullableStr(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}
