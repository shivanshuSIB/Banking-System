package main

import (
	"database/sql"
	"net/http"

	"banking-system/internal/repository"
	"banking-system/internal/service"
)

// diContainer holds every application dependency as a lazy provider.
type diContainer struct {
	flags *flags

	db          func() (*sql.DB, error)
	accountRepo func() (repository.AccountRepository, error)
	txRepo      func() (repository.TransactionRepository, error)
	accountSvc  func() (*service.AccountService, error)
	ledgerSvc   func() (*service.LedgerService, error)
	httpHandler func() (http.Handler, error)
}

func newDIContainer(flg *flags) (*diContainer, ErrCloser) {
	dic := &diContainer{
		flags: flg,
	}

	var closeDB ErrCloser

	dic.db, closeDB = newDBDIProvider(flg)
	dic.accountRepo = newAccountRepoDIProvider(dic)
	dic.txRepo = newTxRepoDIProvider(dic)
	dic.accountSvc = newAccountSvcDIProvider(dic)
	dic.ledgerSvc = newLedgerSvcDIProvider(dic)
	dic.httpHandler = newHTTPHandlerDIProvider(dic)

	cl := func(onErr OnErrFunc) {
		closeDB.Wrap("database")(onErr)
	}
	return dic, cl
}
