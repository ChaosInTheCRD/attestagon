package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/cache"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/chaosinthecrd/attestagon/pkg/util"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	Artifacts []Artifact

	tetragonClient tetragonv1.FineGuidanceSensorsClient

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

	// wg is the waitGroup for handling the goroutines running to process pods
	wg sync.WaitGroup

	// eventCache is the cache of tetragon events received from the gRPC stream
	eventCache cache.Cache
}

// Config is the config file for the attestagon controller.
type Config struct {
	Artifacts []Artifact `yaml:"artifacts"`
}

// Artifact is the configuration fields for a pod that generates a particular artifact, and the particular annotation value it should look for, as well as the image repository reference that it should send the attestation to.
type Artifact struct {
	Name string `yaml:"name"`
	Ref  string `yaml:"ref"`
}

// New constructs a new Controller instance.
func New(ctx context.Context, log logr.Logger, opts Options) (*Controller, error) {
	c := &Controller{
		log:          log.WithName("attestagon"),
		cosignConfig: opts.CosignConfig,
	}

	conn, err := util.Dial(ctx, opts.TetragonServerAddress, opts.TLSConfig, log)
	if err != nil {
		c.log.Error(err, "Error on recieving events: ")
	}

	c.tetragonClient = tetragonv1.NewFineGuidanceSensorsClient(conn)

	client, err := kubernetes.NewForConfig(opts.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes client: %w", err)
	}

	c.clientset = client

	config, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load attestagon config: %w", err)
	}

	c.Artifacts = config.Artifacts

	mgr, err := manager.New(runtimeconfig.GetConfigOrDie(), manager.Options{Scheme: scheme.Scheme})
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

	if pod.Status.Phase != corev1.PodPending {
		return reconcile.Result{}, nil
	}

	// Check if it needs to be attestagon'd
	ok, art := c.ReadyForProcessing(pod)
	if ok {
		log.Printf("This Pod needs attested! %s", pod.GetName())
		pod.SetAnnotations(map[string]string{"attestagon.io/attested": "true"})
		err = c.client.Update(ctx, pod)

		// Do the attestagon thing
		containers := []predicate.Container{}
		for _, n := range pod.Spec.Containers {
			containers = append(containers, predicate.Container{Name: n.Name, Image: n.Image})
		}
		for _, n := range pod.Spec.InitContainers {
			containers = append(containers, predicate.Container{Name: n.Name, Image: n.Image})
		}
		c.wg.Add(1)
		go c.ProcessPod(ctx, pod, art, containers)
		if err != nil {
			return reconcile.Result{}, err
		}
		// pod.SetAnnotations(map[string]string{"attestagon.io/attested": "true"})
		// err = c.client.Update(ctx, pod)
	} else {
		log.Printf("Aha! This pod don't need no attesting! Pod %s is in phase %s", pod.GetName(), pod.Status.Phase)
	}

	return reconcile.Result{}, err
}

func (c *Controller) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.log.Info("shutting down server")
		os.Exit(1)
	}()

	err := c.controllerManager.Start(ctx)
	c.wg.Wait()
	wg.Wait()
	return err
}
