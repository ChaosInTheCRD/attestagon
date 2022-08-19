package options

import (
	"time"

	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/chaosinthecrd/attestagon/internal/flags"
)

// Options are the CSI Driver flag options.
type Options struct {
   *flags.Flags

   // Attestagon are options specific to the attestagon controller itself.
   Attestagon OptionsAttestagon

   // Tetragon are options specific to the tetragon configuration.
   Tetragon OptionsTetragon
}

// OptionsAttestagon is options specific to attestagon controller itself.
type OptionsAttestagon struct {
   // ConfigPath is the path where the controller can find the config file.
   ConfigPath string

   // TLSConfig is the TLS config for the attestagon controller.
   TLSConfig TLSConfig

   // CosignConfig is the cosign configuration for the attestagon controller to use for signing the attestation.
   CosignConfig CosignConfig
}

// OptionsTetragon is options specific to the way tetragon has been configured.
type OptionsTetragon struct {
   // TetragonServerAdddress is the address for the tetragon GRPC server.
   TetragonServerAddress string
}

type TLSConfig struct {
   // CertPath is the path to the location of the public tls certificate
   CertPath string

   // KeyPath is the path to the location of the tls private key
   KeyPath string
}

type CosignConfig struct {
   // PrivateKeyPath is the path to the location of the cosign private key
   PrivateKeyPath string

   // PublicKeyPath is the path to the location of the cosign public key
   PublicKeyPath string
}


func New() *Options {
	o := new(Options)
	o.Flags = flags.New().
		Add("Attestagon", o.addAttestagonFlags).
		Add("Tetragon", o.addTetragonFlags)

	return o
}

func (o *Options) addAttestagonFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Attestagon.ConfigPath, "config-path", "",
		"The path where the controller can find the config file.")
	fs.StringVar(&o.Attestagon.TLSConfig.CertPath, "tls-cert-path", "",
		"Path to the location of the public tls certificate.")
	fs.StringVar(&o.Attestagon.TLSConfig.CertPath, "tls-key-path", "",
		"Path to the location of the tls private key.")
	fs.StringVar(&o.Attestagon.CosignConfig.PrivateKeyPath, "cosign-private-key-path", "",
		"Path to the location of the cosign private key.")
	fs.StringVar(&o.Attestagon.CosignConfig.PublicKeyPath, "cosign-public-key-path", "",
		"Path to the location of the cosign public key.")
}

func (o *Options) addTetragonFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Tetragon.TetragonNamespace, "tetragon-namespace", "",
		"The namespace where Tetragon is deployed.")
}
