package main

import (
	"os"
	"testing"
)

func TestNewFlags_Defaults(t *testing.T) {
	flg := newFlags()

	if flg.databaseURL != "postgres://banking:banking@localhost:5433/banking?sslmode=disable" {
		t.Errorf("unexpected default databaseURL: %s", flg.databaseURL)
	}
	if flg.addr != ":8080" {
		t.Errorf("unexpected default addr: %s", flg.addr)
	}
	if flg.debug {
		t.Error("debug should default to false")
	}
	if flg.frontendDir != "" {
		t.Errorf("frontendDir should default to empty, got %q", flg.frontendDir)
	}
}

func TestEnvOr_UsesEnv(t *testing.T) {
	const key = "TEST_ENVVAR_BANKING_XYZ"
	os.Setenv(key, "from-env")
	defer os.Unsetenv(key)

	got := envOr(key, "fallback")
	if got != "from-env" {
		t.Errorf("expected 'from-env', got %q", got)
	}
}

func TestEnvOr_UsesFallback(t *testing.T) {
	const key = "TEST_ENVVAR_BANKING_NONEXISTENT"
	os.Unsetenv(key)

	got := envOr(key, "default-val")
	if got != "default-val" {
		t.Errorf("expected 'default-val', got %q", got)
	}
}

func TestEnvOr_EmptyEnvUsesFallback(t *testing.T) {
	const key = "TEST_ENVVAR_BANKING_EMPTY"
	os.Setenv(key, "")
	defer os.Unsetenv(key)

	got := envOr(key, "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback' for empty env, got %q", got)
	}
}
