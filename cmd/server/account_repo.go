package main

import (
	"sync"

	"banking-system/internal/repository"
	pgRepo "banking-system/internal/repository/postgres"
)

func newAccountRepo(dic *diContainer) (repository.AccountRepository, error) {
	db, err := dic.db()
	if err != nil {
		return nil, err
	}
	return pgRepo.NewAccountRepo(db), nil
}

func newAccountRepoDIProvider(dic *diContainer) func() (repository.AccountRepository, error) {
	var repo repository.AccountRepository
	var mu sync.Mutex
	return func() (repository.AccountRepository, error) {
		mu.Lock()
		defer mu.Unlock()
		var err error
		if repo == nil {
			repo, err = newAccountRepo(dic)
		}
		return repo, err
	}
}
