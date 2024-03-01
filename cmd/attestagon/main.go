package main

import (
	"fmt"
	"os"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	cmd := app.NewCommand(ctrl.SetupSignalHandler())
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
