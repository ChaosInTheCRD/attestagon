package main

import (
	"fmt"
	"os"
        ctrl "sigs.k8s.io/controller-runtime"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/app"
)

func main() {
	cmd := app.NewCommand(ctrl.SetupSignalHandler())
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

}
