package tetragon

import (
	"crypto/tls"
)

type GrpcClientConfig struct { // TLSConfig is the TLS config for connecting to the grpc server
	TLSConfig *tls.Config

	// TetragonServerAddress is the address for the tetragon GRPC server.
	TetragonServerAddress string
}
