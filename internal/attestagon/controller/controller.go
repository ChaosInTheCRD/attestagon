package controller

import (
	"crypto/tls"
	"fmt"
        "context"

        corev1 "k8s.io/api/core/v1"
        "k8s.io/apimachinery/pkg/api/errors"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/chaosinthecrd/attestagon/internal/tetragon"
	"github.com/go-logr/logr"
        "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
        "sigs.k8s.io/controller-runtime/pkg/manager"
        "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"k8s.io/client-go/kubernetes/scheme"
        runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
        runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

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
type Controller struct {
   // log is the Controller logger.
   log logr.Logger

   // artifacts are the artifacts for which attestagon should generate attestations for.
   artifacts []Artifact

   // tetragonGrpcClientConfig is the config used to connect to the tetragon grpc server. 
   tetragonGrpcClientConfig tetragon.GrpcClientConfig

   // cosignConfig is the cosign configuration for the attestagon controller to use for signing the attestation.
   cosignConfig options.CosignConfig

   // clientSet is the Kubernetes clientset used for interacting with the kubernetes api.
   clientset *kubernetes.Clientset

   // controllerManager is the controller-runtime manager used to run the controller.
   controllerManager manager.Manager

   // cache is the controller-runtime controller cache.
   cache runtimeclient.Reader

   // client is the controller-runtime controller client.
   client runtimeclient.Client
}

type myController struct {
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
	}

	// Set sane defaults.

        if opts.TLSConfig.CertPath != "" && opts.TLSConfig.KeyPath != "" {
            cer, err := tls.LoadX509KeyPair(opts.TLSConfig.CertPath, opts.TLSConfig.KeyPath)
            if err != nil {
               return nil, fmt.Errorf("failed to load x509 key pair for attestagon grpc client: %w", err)
            }
            c.tetragonGrpcClientConfig.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cer}}
        }

        if opts.TetragonServerAddress == "" {
            c.tetragonGrpcClientConfig.TetragonServerAddress = "tetragon.kube-system.svc.cluster.local:54321"
        }


	client, err := kubernetes.NewForConfig(opts.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes client: %w", err)
	}

        c.clientset = client

        config, err := loadConfig(opts.ConfigPath)
        if err != nil {
           return nil, fmt.Errorf("failed to load attestagon config: %w", err)
        }

        c.artifacts = config.Artifacts

        mgr, err := manager.New(runtimeconfig.GetConfigOrDie(), manager.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}

        c.controllerManager = mgr

         err = builder.ControllerManagedBy(c.controllerManager).For(&corev1.Pod{}).Complete(&Controller{cache: c.controllerManager.GetCache(), client: c.controllerManager.GetClient()})
         if err != nil {
             return nil, err
         }

        
	return c, nil
}

func (c *Controller) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Observe the state of the world
	pod := new(corev1.Pod)
	err := c.cache.Get(ctx, request.NamespacedName, pod)
	if errors.IsNotFound(err) {
		// pod has been deleted
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

        // Check if it needs to be attestagon'd
        if c.ReadyForProcessing(pod) {
           // Do the attestagon thing
           err = c.ProcessPod(ctx, pod)
           if err != nil {
              return reconcile.Result{}, nil
           }
        }

        // TODO: Try and update the object as attestagon'd
	err = c.client.Update(ctx, pod)
	return reconcile.Result{}, err
}

func (c *Controller) Run() {
   c.controllerManager.Start(signals.SetupSignalHandler())
   return
}

