package main

import (
	"net/http"
	"sync"

	"banking-system/internal/handler"
)

func newHTTPHandler(dic *diContainer) (http.Handler, error) {
	accountSvc, err := dic.accountSvc()
	if err != nil {
		return nil, err
	}
	ledgerSvc, err := dic.ledgerSvc()
	if err != nil {
		return nil, err
	}
	if dic.flags.frontendDir != "" {
		return handler.NewWithFrontend(accountSvc, ledgerSvc, dic.flags.frontendDir), nil
	}
	return handler.New(accountSvc, ledgerSvc), nil
}

func newHTTPHandlerDIProvider(dic *diContainer) func() (http.Handler, error) {
	var h http.Handler
	var mu sync.Mutex
	return func() (http.Handler, error) {
		mu.Lock()
		defer mu.Unlock()
		var err error
		if h == nil {
			h, err = newHTTPHandler(dic)
		}
		return h, err
	}
}
