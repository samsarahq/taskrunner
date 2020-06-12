// +build js

package oops

import (
	"errors"
	"fmt"
)

// A Wrapper provides context around another error.
type Wrapper interface {
	// Unwrap returns the next error in the error chain.
	// If there is no next error, Unwrap returns nil.
	Unwrap() error
}

func Frames(err error) [][]Frame {
	return nil
}

var Errorf = fmt.Errorf

func Wrapf(err error, format string, a ...interface{}) error {
	return err
}

func Cause(err error) error {
	return err
}

func Recover(p interface{}) error {
	if p == nil {
		return nil
	}

	return errors.New("recovered panic")
}

func Unwrap(err error) error {
	return err
}

func Is(err, target error) bool {
	panic("Not supported for js builds")
}

func As(err error, target interface{}) bool {
	panic("Not supported for js builds")
}
