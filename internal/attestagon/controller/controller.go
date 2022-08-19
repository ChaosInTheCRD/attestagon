package controller

import (
   "log"
   "crypto/tls"
   "github.com/chaosinthecrd/attestagon/internal/tetragon"
   "github.com/chaosinthecrd/attestagon/internal/app/options"
   "k8s.io/client-go/rest"
   kubernetes "github.com/kubernetes/client-go"
   "github.com/go-logr/logr"
)

// Options holds the options needed for the controller
type Options struct {
   // ConfigPath is the path to the attestagon controller configuration file.
   ConfigPath string

   // TLSConfig is the TLS config for the attestagon controller.
   TLSConfig options.TLSConfig

   // CosignConfig is the cosign configuration for the attestagon controller to use for signing the attestation.
   CosignConfig options.CosignConfig

   // TetragonServerAdddress is the address for the tetragon GRPC server.
   TetragonServerAddress string

   // RestConfig is used for interacting with the Kubernetes API server.
   RestConfig *rest.Config
}

// Controller is used for running the attestagon controller. Controller will watch the attestagon logs and generate signed attestations from those logs based on pods that are marked to be attested (using pod annotations).
type Controller struct{
   // log is the Controller logger.
   log logr.Logger

   // artifacts are the artifacts for which attestagon should generate attestations for.
   artifacts []Artifact

   // tetragonGrpcClientConfig is the config used to connect to the tetragon grpc server. 
   tetragonGrpcClientConfig tetragon.GrpcClientConfig

   // cosignConfig is the cosign configuration for the attestagon controller to use for signing the attestation.
   cosignConfig options.CosignConfig

   // clientSet is the Kubernetes clientset used for interacting with the kubernetes api.
   clientset kubernetes.Clientset
}

// Config is the config file for the attestagon controller. 
type Config struct {
  Artifacts  []Artifact `yaml:"artifacts"`
}

// Artifact is the configuration fields for a pod that generates a particular artifact, and the particular annotation value it should look for, as well as the image repository reference that it should send the attestation to.
type Artifact struct {
  Name string `yaml:"name"`
  Ref string `yaml:"ref"`
}


// New constructs a new Controller instance.
func New(log logr.Logger, opts Options) (*Controller, error) {
	c := &Controller{
		log:          log.WithName("attestagon"),
                cosignConfig: opts.CosignConfig,
                tetragonGrpcClientConfig: tetragon.GrpcClientConfig{TLSConfig: opts.TLSConfig},
	}

	// Set sane defaults.

        if opts.TetragonServerAddress == "" {
            c.tetragonGrpcClientConfig.TetragonServerAddress = "tetragon.kube-system.svc.cluster.local:54321"
        }

	if len(d.certFileName) == 0 {
		d.certFileName = "tls.crt"
	}
	if len(d.keyFileName) == 0 {
		d.keyFileName = "tls.key"
	}
	if len(d.caFileName) == 0 {
		d.caFileName = "ca.crt"
	}
	if d.certificateRequestDuration == 0 {
		d.certificateRequestDuration = time.Hour
	}

	var err error
	store, err := storage.NewFilesystem(d.log, opts.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to setup filesystem: %w", err)
	}
	// Used by clients to set the stored file's file-system group before
	// mounting.
	store.FSGroupVolumeAttributeKey = "spiffe.csi.cert-manager.io/fs-group"

	d.store = store
	d.camanager = newCAManager(log, store, opts.RootCAs,
		opts.CertificateFileName, opts.KeyFileName, opts.CAFileName)

	cmclient, err := cmclient.NewForConfig(opts.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build cert-manager client: %w", err)
	}

	mngrLog := d.log.WithName("manager")
	d.driver, err = driver.New(opts.Endpoint, d.log.WithName("driver"), driver.Options{
		DriverName:    opts.DriverName,
		DriverVersion: "v0.2.0",
		NodeID:        opts.NodeID,
		Store:         d.store,
		Manager: manager.NewManagerOrDie(manager.Options{
			Client: cmclient,
			// Use Pod's service account to request CertificateRequests.
			ClientForMetadata:    util.ClientForMetadataTokenRequestEmptyAud(opts.RestConfig),
			MaxRequestsPerVolume: 1,
			MetadataReader:       d.store,
			Clock:                clock.RealClock{},
			Log:                  &mngrLog,
			NodeID:               opts.NodeID,
			GeneratePrivateKey:   generatePrivateKey,
			GenerateRequest:      d.generateRequest,
			SignRequest:          signRequest,
			WriteKeypair:         d.writeKeypair,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to setup csi driver: %w", err)
	}

	return d, nil
}



