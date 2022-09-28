package controller

import (
   "context"
   "os"
   "gopkg.in/yaml.v2"
   corev1 "k8s.io/api/core/v1"
   "google.golang.org/grpc"
   "google.golang.org/grpc/credentials"
   "google.golang.org/grpc/credentials/insecure"
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


func (c *Controller) dial(ctx context.Context) (*grpc.ClientConn, error) {

   var err error
   var conn *grpc.ClientConn

   if c.tetragonGrpcClientConfig.TLSConfig != nil {
      c.log.Info("Connecting to tetragon runtime with TLS enabled")
      conn, err = grpc.DialContext(
         ctx, 
         c.tetragonGrpcClientConfig.TetragonServerAddress, 
         grpc.WithTransportCredentials(credentials.NewTLS(c.tetragonGrpcClientConfig.TLSConfig)), 
         grpc.WithBlock(),
      )
   } else {
      c.log.Info("Connecting to tetragon runtime with TLS disabled")
      conn, err = grpc.DialContext(
         ctx,
         c.tetragonGrpcClientConfig.TetragonServerAddress,
         grpc.WithTransportCredentials(insecure.NewCredentials()),
      )

   } 
   if err != nil {
      return nil, err
   }

   c.log.Info("Connected to tetragon runtime")
   return conn, nil
}


func (c *Controller) ReadyForProcessing(pod *corev1.Pod) bool {
   for i := 0; i < len(c.artifacts); i++ {
      if pod.Status.Phase == "Completed" && pod.Annotations["attestagon.io/artifact"] == c.artifacts[i].Name && c.artifacts[i].Name != "" && pod.Annotations["attestagon.io/attested"] != "true" {
         return true
      } 
   }

   return false
}
