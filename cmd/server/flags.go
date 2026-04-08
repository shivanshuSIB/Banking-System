package main

import (
	"flag"
	"os"
)

type flags struct {
	databaseURL string
	addr        string
	debug       bool
	frontendDir string
}

func newFlags() *flags {
	return &flags{
		databaseURL: "postgres://banking:banking@localhost:5433/banking?sslmode=disable",
		addr:        ":8080",
		debug:       false,
	}
}

func getFlags() *flags {
	flg := newFlags()
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.StringVar(&flg.databaseURL, "db", envOr("DATABASE_URL", flg.databaseURL), "PostgreSQL DSN")
	fs.StringVar(&flg.addr, "addr", envOr("ADDR", flg.addr), "HTTP listen address")
	fs.BoolVar(&flg.debug, "debug", false, "Enable debug logging")
	fs.StringVar(&flg.frontendDir, "frontend", envOr("FRONTEND_DIR", "frontend"), "Path to frontend static files directory")
	_ = fs.Parse(os.Args[1:])
	return flg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
