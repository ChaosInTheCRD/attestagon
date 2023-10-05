package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"
)

func loadConfig(configPath string) (Config, error) {
	c, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, err
	}

	var config Config
	err = yaml.Unmarshal(c, &config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}

func (c *Controller) dial(ctx context.Context, opts Options) (*grpc.ClientConn, error) {
	var err error
	var conn *grpc.ClientConn

	s := opts.TetragonServerAddress
	if s == "" {
		s = "tetragon.kube-system.svc.cluster.local:54321"
	}

	if opts.TLSConfig.CertPath != "" && opts.TLSConfig.KeyPath != "" {
		cer, err := tls.LoadX509KeyPair(opts.TLSConfig.CertPath, opts.TLSConfig.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load x509 key pair for attestagon grpc client: %w", err)
		}
		conn, err = grpc.DialContext(
			ctx,
			s,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cer}})),
			grpc.WithBlock(),
		)
	} else {
		c.log.Info("Connecting to tetragon runtime with TLS disabled")
		conn, err = grpc.DialContext(
			ctx,
			s,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)

	}
	if err != nil {
		return nil, err
	}

	c.log.Info("Connected to tetragon runtime")
	return conn, nil
}
