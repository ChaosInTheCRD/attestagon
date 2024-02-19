package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/chaosinthecrd/attestagon/internal/tetragon"
	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/go-logr/logr"
)

type EventCache struct {
	ctx          context.Context
	log          logr.Logger
	clientConfig *tetragon.GrpcClientConfig
	filter       *tetragonv1.Filter
	Store        map[string]*predicate.Predicate
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
		Store:        make(map[string]*predicate.Predicate),
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

	for {
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

		if c.Store[pod.Name] == nil {
			c.log.Info("Creating new predicate in cache for pod %s", pod.Name)
			c.Store[pod.Name] = &predicate.Predicate{Pod: predicate.Pod{Name: pod.Name, Namespace: pod.Namespace}}
		}

		err = c.Store[pod.Name].ProcessEvent(res)
		if err != nil {
			// we're not gonna fail here for now. There are situations where we fail to process the event but we don't want everything to fall over
			c.log.Error(err, "Failed to process event")
		}

		continue
	}
}
