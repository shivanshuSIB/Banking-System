package main

import (
	"fmt"
	"log"
)

// OnErrFunc is a callback invoked when a close operation fails.
type OnErrFunc func(err error)

// ErrCloser is a function that releases a resource and reports any error to the
// provided OnErrFunc callback.
type ErrCloser func(onErr OnErrFunc)

// Nil returns a no-op ErrCloser.
func (e *ErrCloser) Nil() ErrCloser {
	return func(onErr OnErrFunc) {}
}

// Wrap tags the error with a label before forwarding to the callback.
func (e ErrCloser) Wrap(label string) ErrCloser {
	return func(onErr OnErrFunc) {
		e(func(err error) {
			if err != nil {
				onErr(fmt.Errorf("%s: %w", label, err))
			}
		})
	}
}

// LogOnErr is a ready-made OnErrFunc that logs errors and continues.
func LogOnErr(err error) {
	if err != nil {
		log.Printf("close error: %v", err)
	}
}
