package oops

// This file is for exported types

// Frame represents a Frame in an oops callstack.
type Frame struct {
	File     string
	Function string
	Line     int
	// Reason is the manual annotation passed to oops.Wrapf.
	Reason string
}
