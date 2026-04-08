package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"banking-system/internal/domain"

	"github.com/google/uuid"
)

// --- mock AccountRepository -------------------------------------------------

type mockAccountRepo struct {
	getByIDFn       func(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	getAllFn         func(ctx context.Context) ([]*domain.Account, error)
	createFn        func(ctx context.Context, name, currency string) (*domain.Account, error)
	getByIDForUpdFn func(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error)
	updateBalanceFn func(ctx context.Context, tx *sql.Tx, id uuid.UUID, delta int64) error
}

func (m *mockAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockAccountRepo) GetAll(ctx context.Context) ([]*domain.Account, error) {
	return m.getAllFn(ctx)
}
func (m *mockAccountRepo) Create(ctx context.Context, name, currency string) (*domain.Account, error) {
	return m.createFn(ctx, name, currency)
}
func (m *mockAccountRepo) GetByIDForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error) {
	return m.getByIDForUpdFn(ctx, tx, id)
}
func (m *mockAccountRepo) UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, delta int64) error {
	return m.updateBalanceFn(ctx, tx, id, delta)
}

// --- tests ------------------------------------------------------------------

func TestGetAccount_Success(t *testing.T) {
	id := uuid.New()
	want := &domain.Account{ID: id, Name: "Alice", Currency: "USD", Balance: 1000}
	repo := &mockAccountRepo{
		getByIDFn: func(_ context.Context, gotID uuid.UUID) (*domain.Account, error) {
			if gotID != id {
				t.Fatalf("unexpected ID %v", gotID)
			}
			return want, nil
		},
	}
	svc := NewAccountService(repo)
	got, err := svc.GetAccount(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Name != want.Name {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	repo := &mockAccountRepo{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Account, error) {
			return nil, domain.ErrAccountNotFound
		},
	}
	svc := NewAccountService(repo)
	_, err := svc.GetAccount(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestListAccounts(t *testing.T) {
	accounts := []*domain.Account{
		{ID: uuid.New(), Name: "A"},
		{ID: uuid.New(), Name: "B"},
	}
	repo := &mockAccountRepo{
		getAllFn: func(context.Context) ([]*domain.Account, error) {
			return accounts, nil
		},
	}
	svc := NewAccountService(repo)
	got, err := svc.ListAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(got))
	}
}

func TestCreateAccount_Defaults(t *testing.T) {
	var capturedName, capturedCurrency string
	repo := &mockAccountRepo{
		createFn: func(_ context.Context, name, currency string) (*domain.Account, error) {
			capturedName = name
			capturedCurrency = currency
			return &domain.Account{ID: uuid.New(), Name: name, Currency: currency}, nil
		},
	}
	svc := NewAccountService(repo)

	// Empty name and currency should use defaults
	_, err := svc.CreateAccount(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if capturedName != "Unnamed" {
		t.Errorf("expected default name 'Unnamed', got %q", capturedName)
	}
	if capturedCurrency != "INR" {
		t.Errorf("expected default currency 'INR', got %q", capturedCurrency)
	}
}

func TestCreateAccount_CustomValues(t *testing.T) {
	var capturedName, capturedCurrency string
	repo := &mockAccountRepo{
		createFn: func(_ context.Context, name, currency string) (*domain.Account, error) {
			capturedName = name
			capturedCurrency = currency
			return &domain.Account{ID: uuid.New(), Name: name, Currency: currency}, nil
		},
	}
	svc := NewAccountService(repo)

	_, err := svc.CreateAccount(context.Background(), "Bob", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if capturedName != "Bob" {
		t.Errorf("expected 'Bob', got %q", capturedName)
	}
	if capturedCurrency != "USD" {
		t.Errorf("expected 'USD', got %q", capturedCurrency)
	}
}

func TestCreateAccount_RepoError(t *testing.T) {
	repoErr := errors.New("db connection failed")
	repo := &mockAccountRepo{
		createFn: func(context.Context, string, string) (*domain.Account, error) {
			return nil, repoErr
		},
	}
	svc := NewAccountService(repo)
	_, err := svc.CreateAccount(context.Background(), "X", "Y")
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}
