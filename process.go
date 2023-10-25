package taskrunner

import (
	"os"
	"os/signal"
	"syscall"
)

// OnInterruptSignal invokes fn when an interrupt signal is received.
// We loosely define the interrupt signal as SIGINT, SIGTERM, and SIGHUP.
func onInterruptSignal(fn func()) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-signalChan
		fn()
	}()
}
