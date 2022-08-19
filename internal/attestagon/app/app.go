package app

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
        "github.com/chaosinthecrd/attestagon/internal/attestagon/controller"
)

const (
	helpOutput = "A controller for generating runtime attestations for Pods running in Kubernetes clusters based on Tetragon events."
)

// NewCommand returns an new command instance of the CSI driver component of csi-driver-spiffe.
func NewCommand(ctx context.Context) *cobra.Command {
	opts := options.New()

	cmd := &cobra.Command{
		Use:   "attestagon",
		Short: helpOutput,
		Long:  helpOutput,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return opts.Complete()
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			log := opts.Logr.WithName("main")

			controller, err := controller.New(opts.Logr, controller.Options{
				ConfigPath:     opts.Attestagon.ConfigPath,
                                TLSConfig:      opts.Attestagon.TLSConfig,
                                CosignConfig:   opts.Attestagon.CosignConfig,
                                
                                TetragonServerAddress: opts.Tetragon.TetragonServerAddress,

				RestConfig:                 opts.RestConfig,
			})
			if err != nil {
				return err
			}

			log.Info("starting attestagon controller...")

			return controller.Run(ctx)
		},
	}

	opts.Prepare(cmd)

	return cmd
}
