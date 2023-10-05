package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/go-logr/logr"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

	// ctx is the context
	ctx context.Context

	// artifacts are the artifacts for which attestagon should generate attestations for.
	artifacts []Artifact

	tetragonClient tetragon.FineGuidanceSensorsClient

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
func New(log logr.Logger, opts Options) (*Controller, error) {
	c := &Controller{
		log:          log.WithName("attestagon"),
		cosignConfig: opts.CosignConfig,
		ctx:          context.Background(),
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
	conn, err := c.dial(c.ctx, opts)
	if err != nil {
		c.log.Error(err, "Error on recieving events: ")
	}

	c.tetragonClient = tetragon.NewFineGuidanceSensorsClient(conn)

	return c, nil
}

// Start starts the controller
func (c *Controller) Start() error {
	ctx := context.Background()
	stream, err := c.tetragonClient.GetEvents(ctx, &tetragon.GetEventsRequest{})
	if err != nil {
		return fmt.Errorf("Failed to call GetEvents")
	}

	var wg sync.WaitGroup
	c.log.Info("Entering Control Loop")
	for {
		select {
		case <-ctx.Done():
			// The context is over, stop processing results
			return ctx.Err()
		default:
			res, err := stream.Recv()
			if err != nil {
				c.log.Error(err, "Failed to get grpc stream response: ")
				serr := stream.CloseSend()
				if serr != nil {
					c.log.Error(err, "Failed to close grpc stream: ")
					return err
				}
				return err
			}

			exec := res.GetProcessExec()
			if exec == nil {
				continue
			}

			pod := processEvent(exec)
			if pod == nil {
				continue
			}

			po, err := c.clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, v1.GetOptions{})
			if err != nil {
				c.log.Error(err, "failed to get pod from kubernetes api")
				return err
			}

			for _, n := range c.artifacts {
				if n.Name == po.Labels["attestagon.io/artifact"] && po.Annotations["attestagion.io/attested"] != "true" {
					c.log.Info("pod found as candidate for attestation", "pod", pod.Name)
					containers := []predicate.Container{}
					for _, n := range po.Spec.Containers {
						containers = append(containers, predicate.Container{Name: n.Name, Image: n.Image})
					}
					for _, n := range po.Spec.InitContainers {
						containers = append(containers, predicate.Container{Name: n.Name, Image: n.Image})
					}
					wg.Add(1)
					go c.ProcessPod(ctx, pod, n, containers, &wg)

					con := []predicate.Container{}
					for _, n := range po.Spec.Containers {
						con = append(con, predicate.Container{Name: n.Name, Image: n.Image})
					}

					c.log.Info("sent container names to goroutine", "pod", pod.Name, "containers", c)
				}
			}

		}
	}

}

func (c *Controller) Run() error {
	err := c.Start()
	return err
}
