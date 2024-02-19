package cache

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func (c *EventCache) dial() (*grpc.ClientConn, error) {
	var err error
	var conn *grpc.ClientConn

	if c.clientConfig.TLSConfig != nil {
		c.log.Info("Connecting to tetragon runtime with TLS enabled")
		conn, err = grpc.DialContext(
			c.ctx,
			c.clientConfig.TetragonServerAddress,
			grpc.WithTransportCredentials(credentials.NewTLS(c.clientConfig.TLSConfig)),
			grpc.WithBlock(),
		)
	} else {
		c.log.Info("Connecting to tetragon runtime with TLS disabled")
		conn, err = grpc.DialContext(
			c.ctx,
			c.clientConfig.TetragonServerAddress,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)

	}
	if err != nil {
		return nil, err
	}

	c.log.Info("Connected to tetragon runtime")
	return conn, nil
}
