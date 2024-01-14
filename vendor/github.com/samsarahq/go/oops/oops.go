//go:build !js
// +build !js

package oops

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// filePrefixesToShortCircuit is a set of prefixes of files which, when encountered when serializing
// an oops error, will skip appending the current frame and all subsequent frames in that stack.
// It is useful for truncating long call stacks to make stack traces more concise.
var filePrefixesToShortCircuit atomic.Value // map[string]struct{}{}

func init() {
	// Make sure we don't panic if we invoke this package and haven't called `SetPrefixesToShortCircuit`.
	SetPrefixesToShortCircuit()
}

// SetPrefixesToShortCircuit sets `filePrefixesToShortCircuit` to `prefixes`. While it is thread-safe, it is the
// caller's responsibility to ensure that there are not multiple callers in the same program calling
// `SetPrefixesToShortCircuit` concurrently, as this may cause unexpected behavior where different prefixes are
// filtered from stacktraces based on the set of values passed by the most recent caller of this function.
func SetPrefixesToShortCircuit(prefixes ...string) {
	prefixMap := make(map[string]struct{})
	for _, prefix := range prefixes {
		prefixMap[prefix] = struct{}{}
	}
	filePrefixesToShortCircuit.Store(prefixMap)
}

// GetPrefixesToShortCircuit returns the current set of file prefixes to short-circuit. The order of the returned string
// slice is non-deterministic.
func GetPrefixesToShortCircuit() []string {
	prefixMap := filePrefixesToShortCircuit.Load().(map[string]struct{})
	prefixes := make([]string, 0, len(prefixMap))
	for prefix := range prefixMap {
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

// stack is a comparable []uintptr slice.
type stack struct {
	frames []uintptr
}

// A oopsError annotates a cause error with a stacktrace and an explanatory
// message.
type oopsError struct {
	// inner is the next non-oops error in the chain.
	inner error
	// previous is the previous oopsError, if any. This value is only used to follow stacktraces.
	previous *oopsError
	// stack is the current stacktrace. Might be the same as previous' stacktrace if that
	// is another oopsError.
	stack *stack
	// reason is a short explanatory message indicating what went wrong at this level in the stack.
	reason string
	// index is the index of the stack frame where this oopsError was added.
	index int
}

// Error implements error and outputs a full backtrace.
func (e *oopsError) Error() string {
	var b strings.Builder
	e.writeStackTrace(&b)
	return b.String()
}

// MainStackToString writes the frames of the main goroutine to a string.
// It returns an empty string if the error is not an oopsError.
func MainStackToString(err error) string {
	var e *oopsError
	if ok := As(err, &e); !ok {
		return ""
	}

	var base error
	for err := error(e); err != nil; err = Unwrap(err) {
		base = err
	}
	var b strings.Builder
	b.WriteString(base.Error())

	frames, skipInfo := framesWithSkipInfo(err)
	if frames == nil || len(frames) == 0 {
		return ""
	}
	b.WriteString("\n\n")
	writeSingleFrameTrace(&b, frames[0], skipInfo[0])
	return b.String()
}

// writeSingleFrameTrace writes the stack trace of frames into the string builder.
func writeSingleFrameTrace(b *strings.Builder, frames []Frame, framesSkipped bool) {
	for _, frame := range frames {
		// Print the current function.
		if frame.Reason != "" {
			b.WriteString(frame.Function)
			b.WriteString(": ")
			b.WriteString(frame.Reason)
			b.WriteRune('\n')
		} else {
			b.WriteString(frame.Function)
			b.WriteRune('\n')
		}
		b.WriteRune('\t')
		b.WriteString(frame.File)
		b.WriteRune(':')
		b.WriteString(strconv.Itoa(frame.Line))
		b.WriteRune('\n')
	}
	if framesSkipped {
		b.WriteString("subsequent stack frames truncated")
		b.WriteRune('\n')
	}
}

// Unwrap returns the next error in the error chain.
// If there is no next error, Unwrap returns nil.
func (err *oopsError) Unwrap() error {
	// Unwrap doesn't follow the err.previous chain because that chain is only
	// used for constructing Frames. err.inner is used for following wrapped error types.
	if err.inner != nil {
		return err.inner
	}
	return nil
}

type stackWithReasons struct {
	stack   *stack
	reasons []string
}

// framesWithSkipInfo returns a slice of stack frames, along with whether or not there were frames that were skipped when
// they were appended to each slice. The returned slices are guaranteed to have the same number of elements.
func framesWithSkipInfo(err error) ([][]Frame, []bool) {
	var e *oopsError
	if ok := As(err, &e); !ok {
		return nil, nil
	}

	// Walk the chain of oopsErrors backwards, collecting a set of stacks and
	// reasons.
	stacks := make([]stackWithReasons, 0, 8)
	for ; e != nil; e = e.previous {
		// If the current error's stack is different from the previous, add it to
		// the set of stacks.
		if len(stacks) == 0 || stacks[len(stacks)-1].stack != e.stack {
			stacks = append(stacks, stackWithReasons{
				stack:   e.stack,
				reasons: make([]string, len(e.stack.frames)),
			})
		}
		// Store the reason with its stack frame.
		stacks[len(stacks)-1].reasons[e.index] = e.reason
	}

	parsedStacks := make([][]Frame, 0, len(stacks))
	skippedFramesInStack := make([]bool, 0, len(stacks))

	// Load the prefixes to short circuit so we don't do it within a loop.
	filePrefixesToSkipMap := filePrefixesToShortCircuit.Load().(map[string]struct{})

	// Walk the set of stacks backwards, starting with stack closest to the
	// causal error.
	for i := len(stacks) - 1; i >= 0; i-- {
		frames := stacks[i].stack.frames
		reasons := stacks[i].reasons

		parsedFrames := make([]Frame, 0, 8)

		// Iterate over the stack frames.
		iter := runtime.CallersFrames(frames)
		var framesSkipped bool

		// j tracks the index in the combined frames / reasons array of iter's stack
		// frame. Each frame in frames / reasons array appears at least once in the
		// iterator's frames, but the iterator's frame might have more frames (for
		// example, cgo frames, or inlined frames.)
		j := 0
		for {
			frame, ok := iter.Next()
			if !ok {
				break
			}

			// Advance j and load the reason whenever the current iterator's frame
			// matches. The iterator's frame's PC might differ by 1 because the
			// iterator adjusts for the difference between callsite and return
			// address.
			var reason string
			if j < len(frames) && (frame.PC == frames[j] || frame.PC+1 == frames[j]) {
				reason = reasons[j]
				j++
			}

			file := frame.File
			// Skip appending this and all other frames from this stack if file contains a prefix in the set of prefixes
			// to short-circuit.
			if mapContainsKeyWithPrefix(filePrefixesToSkipMap, file) {
				framesSkipped = true
				break
			}

			i := strings.LastIndex(file, "/src/")
			if i >= 0 {
				file = file[i+len("/src/"):]
			}

			parsedFrames = append(parsedFrames, Frame{
				File:     file,
				Function: frame.Function,
				Line:     frame.Line,
				Reason:   reason,
			})
		}
		parsedStacks = append(parsedStacks, parsedFrames)
		skippedFramesInStack = append(skippedFramesInStack, framesSkipped)
	}
	return parsedStacks, skippedFramesInStack
}

func mapContainsKeyWithPrefix(filePrefixesToSkipMap map[string]struct{}, file string) bool {
	for prefixToSkip := range filePrefixesToSkipMap {
		if strings.HasPrefix(file, prefixToSkip) {
			return true
		}
	}
	return false
}

// Frames extracts all frames from an oops error. If err is not an oops error,
// nil is returned.
func Frames(err error) [][]Frame {
	frames, _ := framesWithSkipInfo(err)
	return frames
}

// SkipFrames skips numFrames from the stack trace and returns a new copy of the error.
// If numFrames is greater than the number of frames in err, SkipFrames will do nothing and return the original err.
func SkipFrames(err error, numFrames int) error {
	var e *oopsError
	if ok := As(err, &e); !ok || numFrames <= 0 {
		return err
	}
	st := e.stack
	if st == nil {
		return err
	}
	numLeftoverFrames := len(st.frames) - int(numFrames)
	if numLeftoverFrames < 0 {
		return err
	}
	frames := make([]uintptr, numLeftoverFrames)
	copy(frames, st.frames[numFrames:])
	return &oopsError{
		inner:    e.inner,
		previous: e.previous,
		stack:    &stack{frames: frames},
		reason:   e.reason,
		index:    e.index,
	}
}

// writeStackTrace unwinds a chain of oopsErrors and prints the stacktrace
// annotated with explanatory messages.
func (e *oopsError) writeStackTrace(b *strings.Builder) {
	var base error
	var fallbackBase error
	for err := error(e); err != nil; err = Unwrap(err) {
		if _, ok := err.(*oopsError); ok {
			// We've found another oops error in the chain, our "base" is no longer valid.
			// This is possible if another error wraps the oops error:
			// e.g. Oops1 -> nonOopsWrappedErr -> Oops2 -> realError
			base = nil
		} else if base == nil {
			// We've found a non-oops error, and this is the first non-oops error we've
			// found in the current chain.
			//
			// e.g. for the wrapped chain Oops1 -> Oops2 -> networkErr -> realError
			// we want to mark the "base" as networkErr (not realErr)
			base = err
		}
		fallbackBase = err
	}
	if base == nil {
		// This code probably shouldn't be necessary as long as oops errors can't
		// be at the end of the chain (I'm paranoid).
		base = fallbackBase
	}

	b.WriteString(base.Error())
	b.WriteString("\n\n")

	frames, skipInfo := framesWithSkipInfo(e)
	for i, stack := range frames {
		// Include a newline between stacks.
		if i > 0 {
			b.WriteRune('\n')
		}
		writeSingleFrameTrace(b, stack, skipInfo[i])
	}
}

// Reason returns the reason chain of the error. Output can be an empty string.
// NOTE: This does not include inner error in the reason message.
func (e *oopsError) Reason() string {
	output := []string{}
	err := e
	for err != nil {
		if err.reason != "" {
			output = append(output, err.reason)
		}
		err = err.previous
	}
	return strings.Join(output, ": ")
}

// isPrefix checks if a is a prefix of b.
func isPrefix(a []uintptr, b []uintptr) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func wrapf(err error, reason string) error {
	inner := err
	var previous *oopsError

	var st *stack
	var index int
	found := false

	// Find the previous error in our input, if any.
	var e *oopsError
	if ok := As(err, &e); ok {
		previous = e

		// If the input error was not an oops error, then we want our new oops error
		// to point to it, even if it goes on to point to other oops errors. Otherwise,
		// we're in the case of wrapping an already oops-wrapped error and we can point to
		// the same next error.
		if _, ok := err.(*oopsError); ok {
			inner = e.inner
		}

		// Figure out where we are in the existing callstack. Since Wrapf isn't
		// guaranteed to be called at every stack frame, we need to search to find
		// the current callsite. We start searching one level past the previous
		// level (and assume that Wrapf is called at most once per stack).
		st = e.stack
		index = e.index + 1

		// To check where we are, match a number of return frames in the stack. We
		// check one level deeper than the level we are annotating, because the
		// frame in the calling function likely doesn't match:
		//
		// - parent() calls Wrapf and return an error with a stacktrace - child()
		// check cause's return value and then calls Wrapf on parent's error -
		// compare() is the frame that gets compared
		//
		// When parent calls Wrapf and captures the stack frame, the program
		// counter in child will point the if statement that checks the parent's
		// return value. When the child then calls Wrapf, its program counter
		// will have advanced to the Wrapf call, and will no longer match the
		// program originally captured by the parent. However, the program counter
		// in compare will still match, and so we compare against that.
		//
		// To paper over small numbers of dupliate frames (eg. when using
		// recursion), we compare not just 1 frame, but several. We compare only
		// some frames (instead of all) to keep the runtime of Wrapf efficient.

		var buffer [8]uintptr
		// 0 is the frame of Callers, 1 is us, 2 is the public wrapper, 3 is its
		// caller (child), 4 is the caller's caller (compare).
		compare := buffer[:runtime.Callers(4, buffer[:])]

		for index+1 < len(st.frames) {
			if isPrefix(compare, st.frames[index+1:]) {
				found = true
				break
			}
			index++
		}

	}

	if !found {
		var buffer [256]uintptr
		// 0 is the frame of Callers, 1 is us, 2 is the public wrapper, 3 is its
		// caller.
		n := runtime.Callers(3, buffer[:])
		frames := make([]uintptr, n)
		copy(frames, buffer[:n])

		index = 0
		st = &stack{frames: frames}
	}

	return &oopsError{
		inner:    inner,
		previous: previous,
		stack:    st,
		reason:   reason,
		index:    index,
	}
}

// Errorf creates a new error with a reason and a stacktrace.
//
// Use Errorf in places where you would otherwise return an error using
// fmt.Errorf or errors.New.
//
// Note that the result of Errorf includes a stacktrace. This means
// that Errorf is not suitable for storing in global variables. For
// such errors, keep using errors.New.
func Errorf(format string, a ...interface{}) error {
	return wrapf(fmt.Errorf(format, a...), "")
}

// Wrapf annotates an error with a reason and a stacktrace. If err is nil,
// Wrapf returns nil.
//
// Use Wrapf in places where you would otherwise return an error directly. If
// the error passed to Wrapf is nil, Wrapf will also return nil. This makes it
// safe to use in one-line return statements.
//
// To check if a wrapped error is a specific error, such as io.EOF, you can
// extract the error passed in to Wrapf using Cause.
func Wrapf(err error, format string, a ...interface{}) error {
	if err == nil {
		return nil
	}

	return wrapf(err, fmt.Sprintf(format, a...))
}

// Cause extracts the cause error of an oops error. If err is not an oops
// error, err itself is returned.
//
// You can use Cause to check if an error is an expected error. For example, if
// you know than EOF error is fine, you can handle it with Cause.
func Cause(err error) error {
	if e, ok := err.(*oopsError); ok {
		return e.inner
	}
	return err
}

// Recover recovers from a panic in a defer. If there is no panic, Recover()
// returns nil. To use, call oops.Recover(recover()) and compare the result to nil.
func Recover(p interface{}) error {
	if p == nil {
		return nil
	}
	if err, ok := p.(error); ok {
		return wrapf(err, "recovered panic")
	}
	return wrapf(fmt.Errorf("recovered panic: %v", p), "")
}
