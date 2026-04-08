package main

import (
	"sync"

	"banking-system/internal/service"
)

func newAccountSvc(dic *diContainer) (*service.AccountService, error) {
	repo, err := dic.accountRepo()
	if err != nil {
		return nil, err
	}
	return service.NewAccountService(repo), nil
}

func newAccountSvcDIProvider(dic *diContainer) func() (*service.AccountService, error) {
	var svc *service.AccountService
	var mu sync.Mutex
	return func() (*service.AccountService, error) {
		mu.Lock()
		defer mu.Unlock()
		var err error
		if svc == nil {
			svc, err = newAccountSvc(dic)
		}
		return svc, err
	}
}
