package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cert-manager/csi-driver-spiffe/internal/csi/app/options"
	"github.com/cert-manager/csi-driver-spiffe/internal/csi/driver"
	"github.com/cert-manager/csi-driver-spiffe/internal/csi/rootca"
)

const (
	helpOutput = "A controller for generating runtime attestations for Pods running in Kubernetes clusters based on Tetragon events."
)

// NewCommand returns an new command instance of the CSI driver component of csi-driver-spiffe.
func NewCommand(ctx context.Context) *cobra.Command {
	opts := options.New()

	cmd := &cobra.Command{
		Use:   "csi-driver-spiffe",
		Short: helpOutput,
		Long:  helpOutput,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return opts.Complete()
		},

		RunE: func(cmd *cobra.Command, args []string) error {
			log := opts.Logr.WithName("main")

			var rootCA rootca.Interface
			if len(opts.Volume.SourceCABundleFile) > 0 {
				log.Info("using CA root bundle", "filepath", opts.Volume.SourceCABundleFile)

				var err error
				rootCA, err = rootca.NewFile(ctx, opts.Logr, opts.Volume.SourceCABundleFile)
				if err != nil {
					return fmt.Errorf("failed to build root CA: %w", err)
				}
			} else {
				log.Info("propagating root CA bundle disabled")
			}

			driver, err := driver.New(opts.Logr, driver.Options{
				DriverName: opts.DriverName,
				NodeID:     opts.Driver.NodeID,
				Endpoint:   opts.Driver.Endpoint,
				DataRoot:   opts.Driver.DataRoot,

				RestConfig:                 opts.RestConfig,
				TrustDomain:                opts.CertManager.TrustDomain,
				CertificateRequestDuration: opts.CertManager.CertificateRequestDuration,
				IssuerRef:                  opts.CertManager.IssuerRef,

				CertificateFileName: opts.Volume.CertificateFileName,
				KeyFileName:         opts.Volume.KeyFileName,

				CAFileName: opts.Volume.CAFileName,
				RootCAs:    rootCA,
			})
			if err != nil {
				return err
			}

			log.Info("starting SPIFFE CSI driver...")

			return driver.Run(ctx)
		},
	}

	opts.Prepare(cmd)

	return cmd
}
