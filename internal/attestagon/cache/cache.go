package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/pkg/util"
	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/go-logr/logr"
)

// Cache is used for collecting tetragon events marked with the attestagon label. This is to ensure that events are not missed while the controller kicks off the attestation process for a pod.
type Cache struct {
	// log is the Controller logger.
	log logr.Logger

	tetragonClient tetragonv1.FineGuidanceSensorsClient

	// Cache is to store responses from the gRPC server of Tetragon
	Cache map[string][]tetragonv1.GetEventsResponse

	// EventTTL time before the event is removed from the cache.
	eventTTL time.Duration

	// mu is the mutex for modifying the cache
	mu sync.Mutex
}

type Options struct {
	TetragonServerAddress string
	TLSConfig             options.TLSConfig
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

// New constructs a new cache instance.
func New(ctx context.Context, log logr.Logger, opts Options) (*Cache, error) {
	c := &Cache{
		log:      log.WithName("cache"),
		Cache:    map[string][]tetragonv1.GetEventsResponse{},
		eventTTL: 10 * time.Second,
	}

	conn, err := util.Dial(ctx, opts.TetragonServerAddress, opts.TLSConfig, log)
	if err != nil {
		c.log.Error(err, "Error on recieving events: ")
	}

	c.tetragonClient = tetragonv1.NewFineGuidanceSensorsClient(conn)

	return c, nil
}

func (c *Cache) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c.log.Info("shutting down server")
		return
	}()

	err := c.Run(ctx)
	wg.Wait()
	return err
}

// Start starts the controller
func (c *Cache) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if c.Cache != nil {
				for art, res := range c.Cache {
					if len(c.Cache[art]) > 0 {
						c.cleanEvents(art, res)
					}
				}
			}
		}
	}()
	stream, err := c.tetragonClient.GetEvents(ctx, &tetragonv1.GetEventsRequest{})
	if err != nil {
		return fmt.Errorf("Failed to call GetEvents")
	}

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
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

			err = c.processEvent(res)
			if err != nil {
				return err
			}
		}
	}

}
