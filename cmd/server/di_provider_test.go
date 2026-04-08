package main

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newMockDIC creates a diContainer with a sqlmock-backed db provider.
func newMockDIC(t *testing.T) (*diContainer, func()) {
	t.Helper()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}

	flg := &flags{addr: ":0"}
	dic := &diContainer{flags: flg}
	dic.db = func() (*sql.DB, error) { return db, nil }
	dic.accountRepo = newAccountRepoDIProvider(dic)
	dic.txRepo = newTxRepoDIProvider(dic)
	dic.accountSvc = newAccountSvcDIProvider(dic)
	dic.ledgerSvc = newLedgerSvcDIProvider(dic)
	dic.httpHandler = newHTTPHandlerDIProvider(dic)

	return dic, func() { db.Close() }
}

func TestNewDIContainer_AllProvidersWired(t *testing.T) {
	flg := &flags{databaseURL: "postgres://test:test@localhost/test", addr: ":0"}
	dic, closer := newDIContainer(flg)
	defer closer(func(err error) {})

	if dic.flags != flg {
		t.Error("flags not set")
	}
	if dic.db == nil {
		t.Error("db provider is nil")
	}
	if dic.accountRepo == nil {
		t.Error("accountRepo provider is nil")
	}
	if dic.txRepo == nil {
		t.Error("txRepo provider is nil")
	}
	if dic.accountSvc == nil {
		t.Error("accountSvc provider is nil")
	}
	if dic.ledgerSvc == nil {
		t.Error("ledgerSvc provider is nil")
	}
	if dic.httpHandler == nil {
		t.Error("httpHandler provider is nil")
	}
}

func TestDIProviders_AllResolve(t *testing.T) {
	dic, cleanup := newMockDIC(t)
	defer cleanup()

	if _, err := dic.accountRepo(); err != nil {
		t.Fatalf("accountRepo: %v", err)
	}
	if _, err := dic.txRepo(); err != nil {
		t.Fatalf("txRepo: %v", err)
	}
	if _, err := dic.accountSvc(); err != nil {
		t.Fatalf("accountSvc: %v", err)
	}
	if _, err := dic.ledgerSvc(); err != nil {
		t.Fatalf("ledgerSvc: %v", err)
	}
	if _, err := dic.httpHandler(); err != nil {
		t.Fatalf("httpHandler: %v", err)
	}
}

func TestDIProviders_Singleton(t *testing.T) {
	dic, cleanup := newMockDIC(t)
	defer cleanup()

	r1, _ := dic.accountRepo()
	r2, _ := dic.accountRepo()
	if r1 != r2 {
		t.Error("accountRepo should return same instance")
	}

	t1, _ := dic.txRepo()
	t2, _ := dic.txRepo()
	if t1 != t2 {
		t.Error("txRepo should return same instance")
	}

	s1, _ := dic.accountSvc()
	s2, _ := dic.accountSvc()
	if s1 != s2 {
		t.Error("accountSvc should return same instance")
	}

	l1, _ := dic.ledgerSvc()
	l2, _ := dic.ledgerSvc()
	if l1 != l2 {
		t.Error("ledgerSvc should return same instance")
	}

	// httpHandler returns http.Handler which wraps a HandlerFunc (not comparable
	// with !=), so we just verify it resolves twice without error.
	if _, err := dic.httpHandler(); err != nil {
		t.Fatalf("httpHandler first call: %v", err)
	}
	if _, err := dic.httpHandler(); err != nil {
		t.Fatalf("httpHandler second call: %v", err)
	}
}

func TestDIProviders_DBErrorPropagation(t *testing.T) {
	dbErr := errors.New("db unavailable")

	flg := &flags{addr: ":0"}
	dic := &diContainer{flags: flg}
	dic.db = func() (*sql.DB, error) { return nil, dbErr }
	dic.accountRepo = newAccountRepoDIProvider(dic)
	dic.txRepo = newTxRepoDIProvider(dic)
	dic.accountSvc = newAccountSvcDIProvider(dic)
	dic.ledgerSvc = newLedgerSvcDIProvider(dic)
	dic.httpHandler = newHTTPHandlerDIProvider(dic)

	if _, err := dic.accountRepo(); !errors.Is(err, dbErr) {
		t.Errorf("accountRepo should propagate db error, got %v", err)
	}
	if _, err := dic.txRepo(); !errors.Is(err, dbErr) {
		t.Errorf("txRepo should propagate db error, got %v", err)
	}
	if _, err := dic.accountSvc(); !errors.Is(err, dbErr) {
		t.Errorf("accountSvc should propagate db error, got %v", err)
	}
	if _, err := dic.ledgerSvc(); !errors.Is(err, dbErr) {
		t.Errorf("ledgerSvc should propagate db error, got %v", err)
	}
	if _, err := dic.httpHandler(); !errors.Is(err, dbErr) {
		t.Errorf("httpHandler should propagate db error, got %v", err)
	}
}

func TestHTTPHandler_WithFrontendDir(t *testing.T) {
	dic, cleanup := newMockDIC(t)
	defer cleanup()
	dic.flags.frontendDir = "/tmp/nonexistent-test-dir"

	h, err := dic.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler with frontendDir: %v", err)
	}
	if h == nil {
		t.Error("expected non-nil handler")
	}
}
