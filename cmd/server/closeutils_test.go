package main

import (
	"errors"
	"testing"
)

func TestErrCloser_Nil(t *testing.T) {
	var ec ErrCloser
	nilCloser := ec.Nil()

	// Should not panic or call onErr.
	called := false
	nilCloser(func(err error) {
		called = true
	})
	if called {
		t.Error("nil closer should not invoke onErr")
	}
}

func TestErrCloser_Wrap_WithError(t *testing.T) {
	inner := ErrCloser(func(onErr OnErrFunc) {
		onErr(errors.New("boom"))
	})

	wrapped := inner.Wrap("database")

	var got error
	wrapped(func(err error) {
		got = err
	})

	if got == nil {
		t.Fatal("expected error")
	}
	if got.Error() != "database: boom" {
		t.Errorf("expected 'database: boom', got %q", got.Error())
	}
}

func TestErrCloser_Wrap_NoError(t *testing.T) {
	inner := ErrCloser(func(onErr OnErrFunc) {
		onErr(nil)
	})

	wrapped := inner.Wrap("database")

	called := false
	wrapped(func(err error) {
		if err != nil {
			called = true
		}
	})
	if called {
		t.Error("onErr should not be called with non-nil error when inner has no error")
	}
}

func TestLogOnErr_NilError(t *testing.T) {
	// Should not panic with nil error.
	LogOnErr(nil)
}

func TestLogOnErr_NonNilError(t *testing.T) {
	// Should not panic with a real error (it logs, which we can't easily capture, but it shouldn't crash).
	LogOnErr(errors.New("test error"))
}
