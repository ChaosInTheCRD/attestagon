package signals

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2/klogr"
)

type options struct {
	Exit          func(code int)
	SignalChannel func() chan os.Signal
	Log           logr.Logger
}

// defaultOptions are the default options for Execute which uses os.Exit as the
// exit command, and uses the host os.Signal channel to capture signals. All
// non-test consumers will want to use these options.
func defaultOptions() options {
	return options{
		Log:           klogr.New(),
		Exit:          os.Exit,
		SignalChannel: func() chan os.Signal { return make(chan os.Signal, 2) },
	}
}

// Execute will execute the given function, passing in a context which is
// managed by os signals.
// Upon receiving a SIGINT or SIGTERM, the context will be cancelled and a
// graceful shutdown will be attempted. Upon receiving 3 more SIGINT or SIGTERM
// signals, the process will exit with code 1 immediately.
// If a SIGHUP is received, the context will be cancelled and the function will
// be executed again.
// If at any time the command returns an error, the process will print the
// error and exit with code 1.
// Uses the default options, using the signal source from the host os, and
// exiting with os.Exit.
func Execute(cmdFn func(context.Context) error) error {
	return executeWithOptions(cmdFn, defaultOptions())
}

// executeWithOptions is the same as Execute, but allows for custom options to
// be passed. Only needed for testing.
func executeWithOptions(cmdFn func(context.Context) error, opts options) error {
	log := opts.Log.WithName("signals")
	ch := opts.SignalChannel()
	signal.Notify(ch, append(shutdownSignals, syscall.SIGHUP)...)

	for {
		ctx, cancel := context.WithCancel(context.Background())
		cmdStopped, gofuncStopped := make(chan struct{}), make(chan struct{})
		var sig os.Signal

		go func() {
			defer close(gofuncStopped)
			select {
			case sig = <-ch:
			case <-cmdStopped:
				return
			}

			if sig == syscall.SIGHUP {
				log.Info("received SIGHUP, hot reloading...")
				cancel()
				return
			}

			cancel()
			for i := 0; i < 3; i++ {
				log.Info("received signal, shutting down gracefully...", "signal", sig.String())
				select {
				case <-cmdStopped:
					return
				case sig = <-ch:
					// Don't count non SIGINT/SIGTERM signals as a shutdown signal which
					// counts towards the 3	signals used to force shutdown. This mostly
					// prevents some automated process causing a ungraceful shutdown
					// sending SIGHUPs.
					if sig != os.Interrupt && sig != syscall.SIGTERM {
						i--
					}
				}
			}

			log.Error(errors.New("received signal"), "force closing", "signal", sig)

			opts.Exit(1)
		}()

		err := cmdFn(ctx)
		close(cmdStopped)
		<-gofuncStopped

		if err != nil {
			return err
		}

		// If we receive a SIGHUP signal, then we should continue to restart the
		// command.
		if sig == syscall.SIGHUP {
			continue
		}

		return nil
	}
}
