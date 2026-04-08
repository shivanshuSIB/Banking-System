package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "banking-system/docs"
)

// @title Banking System API
// @version 1.0
// @description A double-entry bookkeeping banking system API supporting accounts, transfers, deposits, withdrawals, and reversals.

// @host localhost:8080
// @BasePath /api
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	flg := getFlags()

	dic, closeDIC := newDIContainer(flg)
	defer closeDIC(LogOnErr)

	h, err := dic.httpHandler()
	if err != nil {
		return err
	}

	return serve(flg.addr, h)
}

func serve(addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("server listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
