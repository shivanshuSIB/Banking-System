package main

import (
	"sync"

	"banking-system/internal/repository"
	pgRepo "banking-system/internal/repository/postgres"
)

func newTxRepo(dic *diContainer) (repository.TransactionRepository, error) {
	db, err := dic.db()
	if err != nil {
		return nil, err
	}
	return pgRepo.NewTransactionRepo(db), nil
}

func newTxRepoDIProvider(dic *diContainer) func() (repository.TransactionRepository, error) {
	var repo repository.TransactionRepository
	var mu sync.Mutex
	return func() (repository.TransactionRepository, error) {
		mu.Lock()
		defer mu.Unlock()
		var err error
		if repo == nil {
			repo, err = newTxRepo(dic)
		}
		return repo, err
	}
}
