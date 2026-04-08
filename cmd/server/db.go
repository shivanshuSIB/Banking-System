package main

import (
	"context"
	"database/sql"
	"sync"

	"banking-system/internal/migrations"
	pgRepo "banking-system/internal/repository/postgres"
)

func newDBDIProvider(flg *flags) (func() (*sql.DB, error), ErrCloser) {
	var db *sql.DB
	var mu sync.Mutex
	var ec ErrCloser
	closeDB := ec.Nil()

	provider := func() (*sql.DB, error) {
		mu.Lock()
		defer mu.Unlock()
		if db == nil {
			var err error
			db, err = newDB(flg)
			if err != nil {
				return nil, err
			}
			closeDB = func(onErr OnErrFunc) {
				onErr(db.Close())
			}
		}
		return db, nil
	}

	closer := func(onErr OnErrFunc) { closeDB(onErr) }
	return provider, closer
}

func newDB(flg *flags) (*sql.DB, error) {
	db, err := pgRepo.Open(flg.databaseURL)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(context.Background(), migrations.Schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
