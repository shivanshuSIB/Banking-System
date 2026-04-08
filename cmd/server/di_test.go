package main

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestDIContainer verifies that the DI container wires all providers without
// panicking, given a valid DATABASE_URL. Skip if the database is not reachable.
func TestDIContainer(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://banking:banking@localhost:5432/banking?sslmode=disable"
	}

	// Quick probe: actually connect and ping to verify the DB is available.
	probe, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("cannot open database: %v", err)
	}
	if err := probe.Ping(); err != nil {
		probe.Close()
		t.Skipf("database not reachable: %v", err)
	}
	probe.Close()

	flg := &flags{
		databaseURL: dsn,
		addr:        ":0",
	}

	dic, closer := newDIContainer(flg)
	defer closer(func(err error) {
		if err != nil {
			t.Errorf("close error: %v", err)
		}
	})

	// Resolve every provider — each should succeed if the DB is reachable.
	if _, err := dic.db(); err != nil {
		t.Fatalf("db provider: %v", err)
	}
	if _, err := dic.accountRepo(); err != nil {
		t.Fatalf("accountRepo provider: %v", err)
	}
	if _, err := dic.txRepo(); err != nil {
		t.Fatalf("txRepo provider: %v", err)
	}
	if _, err := dic.accountSvc(); err != nil {
		t.Fatalf("accountSvc provider: %v", err)
	}
	if _, err := dic.ledgerSvc(); err != nil {
		t.Fatalf("ledgerSvc provider: %v", err)
	}
	if _, err := dic.httpHandler(); err != nil {
		t.Fatalf("httpHandler provider: %v", err)
	}
}
