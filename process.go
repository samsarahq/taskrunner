package taskrunner

import (
	"os"
	"os/signal"
	"syscall"
)

// OnInterruptSignal invokes fn when SIGNINT/SIGTERM is received.
func onInterruptSignal(fn func()) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fn()
	}()
}
