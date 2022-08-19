package main

import (
	"fmt"
	"os"

	"github.com/chaosinthecrd/internal/attestagon/app"
)

func main() {
	cmd := app.NewCommand(
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

}
