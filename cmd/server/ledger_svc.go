package main

import (
	"sync"

	"banking-system/internal/service"
)

func newLedgerSvc(dic *diContainer) (*service.LedgerService, error) {
	db, err := dic.db()
	if err != nil {
		return nil, err
	}
	accountRepo, err := dic.accountRepo()
	if err != nil {
		return nil, err
	}
	txRepo, err := dic.txRepo()
	if err != nil {
		return nil, err
	}
	return service.NewLedgerService(db, accountRepo, txRepo), nil
}

func newLedgerSvcDIProvider(dic *diContainer) func() (*service.LedgerService, error) {
	var svc *service.LedgerService
	var mu sync.Mutex
	return func() (*service.LedgerService, error) {
		mu.Lock()
		defer mu.Unlock()
		var err error
		if svc == nil {
			svc, err = newLedgerSvc(dic)
		}
		return svc, err
	}
}
