package service

import (
	"context"

	"banking-system/internal/domain"
	"banking-system/internal/repository"

	"github.com/google/uuid"
)

// AccountService handles account-level operations.
type AccountService struct {
	accounts repository.AccountRepository
}

func NewAccountService(accounts repository.AccountRepository) *AccountService {
	return &AccountService{accounts: accounts}
}

func (s *AccountService) GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	return s.accounts.GetByID(ctx, id)
}

func (s *AccountService) ListAccounts(ctx context.Context) ([]*domain.Account, error) {
	return s.accounts.GetAll(ctx)
}

func (s *AccountService) CreateAccount(ctx context.Context, name, currency string) (*domain.Account, error) {
	if name == "" {
		name = "Unnamed"
	}
	if currency == "" {
		currency = "INR"
	}
	return s.accounts.Create(ctx, name, currency)
}
