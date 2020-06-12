package oops

// This file is for exported types

// A Frame represents a Frame in an oops callstack. The Reason is the manual
// annotation passed to oops.Wrapf.
type Frame struct {
	File     string
	Function string
	Line     int
	Reason   string
}
