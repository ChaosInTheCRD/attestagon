package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/cache"
	tetragonconfig "github.com/chaosinthecrd/attestagon/internal/tetragon"
	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	runtimeconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Options holds the options needed for the controller
type Options struct {
	// ConfigPath is the path to the attestagon controller configuration file.
	ConfigPath string

	// TLSConfig is the TLS config for the attestagon controller.
	TLSConfig options.TLSConfig

	// SignerConfig is the signer configuration for the attestagon controller to use for signing the attestation.
	SignerConfig options.SignerConfig

	// TetragonServerAdddress is the address for the tetragon GRPC server.
	TetragonServerAddress string

	// RestConfig is used for interacting with the Kubernetes API server.
	RestConfig *rest.Config
}

// Controller is used for running the attestagon controller. Controller will watch the attestagon logs and generate signed attestations from those logs based on pods that are marked to be attested (using pod annotations).
type Controller struct {
	// ctx is the context for the controller.
	ctx context.Context

	// log is the Controller logger.
	log logr.Logger

	// artifacts are the artifacts for which attestagon should generate attestations for.
	artifacts []Artifact

	// tetragonGrpcClientConfig is the config used to connect to the tetragon grpc server.
	tetragonGrpcClientConfig tetragonconfig.GrpcClientConfig

	// signerConfig is the signer configuration for the attestagon controller to use for signing the attestation.
	signerConfig options.SignerConfig

	// clientSet is the Kubernetes clientset used for interacting with the kubernetes api.
	clientset *kubernetes.Clientset

	// controllerManager is the controller-runtime manager used to run the controller.
	controllerManager manager.Manager

	// cache is the controller-runtime controller cache.
	cache runtimeclient.Reader

	// client is the controller-runtime controller client.
	client runtimeclient.Client

	// eventCache is the cache of tetragon events
	eventCache cache.EventCache

	// mutex is the mutex to ensure that only one process function is executed per pod
	mutex map[string]*sync.Mutex
}

// Config is the config file for the attestagon controller.
type Config struct {
	Artifacts []Artifact `yaml:"artifacts"`
	PodFilter PodFilter  `yaml:"podFilter"`
}

// PodFilter are the filters applied to the tetragon events that are monitored by the attestagon controller.
type PodFilter struct {
	Namespaces []string `yaml:"namespaces"`
	Regex      []string `yaml:"regex"`
}

// Artifact is the configuration fields for a pod that generates a particular artifact, and the particular annotation value it should look for, as well as the image repository reference that it should send the attestation to.
type Artifact struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref"`
}

// New constructs a new Controller instance.
func New(log logr.Logger, opts Options) (*Controller, error) {
	ctx := context.Background()

	config, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load attestagon config: %w", err)
	}

	filters := &tetragonv1.Filter{
		Namespace:   config.PodFilter.Namespaces,
		BinaryRegex: config.PodFilter.Regex,
	}

	ec, err := cache.New(ctx, log.WithName("attestagon-cache"), opts.TLSConfig, opts.TetragonServerAddress, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to create event cache: %w", err)
	}

	c := &Controller{
		ctx:          ctx,
		log:          log.WithName("attestagon"),
		signerConfig: opts.SignerConfig,
		artifacts:    config.Artifacts,
		eventCache:   *ec,
	}

	// Set sane defaults.
	client, err := kubernetes.NewForConfig(opts.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes client: %w", err)
	}

	c.clientset = client

	mgr, err := manager.New(runtimeconfig.GetConfigOrDie(), manager.Options{Scheme: scheme.Scheme, Metrics: metricsserver.Options{BindAddress: "0"}})
	if err != nil {
		return nil, err
	}

	c.controllerManager = mgr

	c.cache = c.controllerManager.GetCache()
	c.client = c.controllerManager.GetClient()
	err = builder.ControllerManagedBy(c.controllerManager).For(&corev1.Pod{}).Complete(c)
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
	if art := c.ReadyForProcessing(pod); art != nil {
		pod.SetAnnotations(map[string]string{"attestagon.io/attested": "true"})
		err = c.client.Update(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		err = c.ProcessPod(ctx, pod, art)
		if err != nil {
			c.log.Error(err, "Failed to process pod")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, err
}

func (c *Controller) Run() error {
	log.SetLogger(zap.New())
	var cancel context.CancelFunc
	c.ctx, cancel = context.WithCancel(c.ctx)
	defer cancel()

	errChan := make(chan error, 2)

	go func() {
		if err := c.eventCache.Start(); err != nil {
			errChan <- fmt.Errorf("eventCache error: %w", err)
		}
	}()

	go func() {
		if err := c.controllerManager.Start(c.ctx); err != nil {
			errChan <- fmt.Errorf("controllerManager error: %w", err)
		}
	}()

	// Wait for an error from any goroutine or both to complete
	select {
	case err := <-errChan:
		// On error, cancel context to shutdown the other goroutine gracefully
		cancel()
		return err
	case <-c.ctx.Done():
		// If the context is done, check for errors from both goroutines
		close(errChan) // Close the channel to avoid leaks
		for e := range errChan {
			if e != nil {
				return e // Return the first error encountered
			}
		}
	}

	return nil
}
