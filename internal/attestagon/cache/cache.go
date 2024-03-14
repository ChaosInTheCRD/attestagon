package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/tetragon"
	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/go-logr/logr"
	"github.com/in-toto/go-witness/attestation/attestagon"
)

type EventCache struct {
	ctx          context.Context
	log          logr.Logger
	clientConfig *tetragon.GrpcClientConfig
	filter       *tetragonv1.Filter
	Store        map[string]*attestagon.Attestor
}

func New(ctx context.Context, log logr.Logger, tlsConfig options.TLSConfig, tetragonAddr string, podFilter *tetragonv1.Filter) (*EventCache, error) {
	co := &tetragon.GrpcClientConfig{
		TetragonServerAddress: tetragonAddr,
	}

	if tetragonAddr == "" {
		co.TetragonServerAddress = "tetragon.kube-system.svc.cluster.local:54321"
	}

	if tlsConfig.CertPath != "" && tlsConfig.KeyPath != "" {
		cer, err := tls.LoadX509KeyPair(tlsConfig.CertPath, tlsConfig.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load x509 key pair for attestagon grpc client: %w", err)
		}
		co.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cer}}
	}

	return &EventCache{
		ctx:          ctx,
		clientConfig: co,
		filter:       podFilter,
		log:          log,
		Store:        make(map[string]*attestagon.Attestor),
	}, nil
}

func (c *EventCache) Start() error {
	conn, err := c.dial()
	if err != nil {
		return errors.Join(fmt.Errorf("error on recieving events"), err)
	}

	client := tetragonv1.NewFineGuidanceSensorsClient(conn)

	stream, err := getEventStream(c.ctx, client, c.filter)
	if err != nil {
		c.log.Error(err, "Failed to get tetragon events: ")
	}

	errCh := make(chan error)
	done := make(chan struct{})
	go func() {
		err := c.runGarbageCollection()
		if err != nil {
			errCh <- err
		} else {
			done <- struct{}{}
		}
	}()

	for {
		select {
		case err := <-errCh:
			return errors.Join(fmt.Errorf("garbage collector exited with an error"), err)
		case <-done:
			c.log.Info("Garbage collector exited")
			return nil
		default:
			res, err := stream.Recv()
			if err != nil {
				err := stream.CloseSend()
				if err != nil {
					return errors.Join(err, fmt.Errorf("failed to close stream"))
				}
				return errors.Join(err, fmt.Errorf("failed to recieve event"))
			}

			pod := getPodFromEvent(res)
			if pod == nil {
				c.log.Info("No pod name found in event, skipping", "node_name", res.GetNodeName())
				continue
			}

			// NOTE: It'd be good to ensure that only annotated pods get added to the cache

			if c.Store[pod.Name] == nil {
				c.log.Info("Creating new predicate in cache", "pod", pod.Name)
				c.Store[pod.Name] = &attestagon.Attestor{CreatedAt: time.Now(), Pod: attestagon.Pod{Name: pod.Name, Namespace: pod.Namespace}}
			}

			err = c.Store[pod.Name].ProcessEvent(res, c.log)
			if err != nil {
				// we're not gonna fail here for now. There are situations where we fail to process the event but we don't want everything to fall over
				continue
			}

			continue
		}
	}
}

func (c *EventCache) runGarbageCollection() error {
	for {
		// NOTE:
		// We want to execute garbage collection every minute for now
		// We probably want to add something to check to see if the pod is still alive
		// Should be careful to ensure that we don't slam the API
		time.Sleep(1 * time.Minute)
		for k, v := range c.Store {
			if time.Since(v.CreatedAt) > 1*time.Hour {
				c.log.Info("Deleting predicate from cache", "pod", k)
				delete(c.Store, k)
			}
		}
	}
}
