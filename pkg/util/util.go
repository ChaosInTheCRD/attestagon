package util

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/app/options"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func Dial(ctx context.Context, address string, tlsConfig options.TLSConfig, log logr.Logger) (*grpc.ClientConn, error) {
	var err error
	var conn *grpc.ClientConn
	var cer tls.Certificate

	if address == "" {
		address = "tetragon.kube-system.svc.cluster.local:54321"
	}

	if tlsConfig.CertPath != "" && tlsConfig.KeyPath != "" {
		cer, err = tls.LoadX509KeyPair(tlsConfig.CertPath, tlsConfig.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load x509 key pair for attestagon grpc client: %w", err)
		}

		log.Info("Connecting to tetragon runtime with TLS enabled")
		conn, err = grpc.DialContext(
			ctx,
			address,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cer}})),
			grpc.WithBlock(),
		)
	} else {
		log.Info("Connecting to tetragon runtime with TLS disabled")
		conn, err = grpc.DialContext(
			ctx,
			address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}
	if err != nil {
		return nil, err
	}

	log.Info("Connected to tetragon runtime")
	return conn, nil
}
