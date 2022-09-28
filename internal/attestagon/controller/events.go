package controller

import (
	"context"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/cilium/tetragon/api/v1/tetragon"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) GetRuntimeMetadata( ctx context.Context, predicate predicate.Predicate, pod *corev1.Pod) ([]tetragon.GetEventsResponse, error) {

  c.log.Info("Start collecting runtime events")
  conn, err := c.dial(ctx)
  if err != nil {
     c.log.Error(err, "Error on recieving events: ")
  }

  client := tetragon.NewFineGuidanceSensorsClient(conn)

  stream, err := client.GetEvents(ctx, &tetragon.GetEventsRequest{
     AllowList: []*tetragon.Filter{{Namespace: []string{"default"}}},
  })
  if err != nil {
     c.log.Error(err, "Failed to get tetragon events: ")
  }

  streamStart := time.Now()

  events := make([]tetragon.GetEventsResponse, 0) 
  for time.Now() != streamStart.Add(20 * time.Second) {
     res, err := stream.Recv()
     if err != nil {
        c.log.Error(err, "Failed to get grpc stream response: ")

        err := stream.CloseSend()
        if err != nil {
           c.log.Error(err, "Failed to close grpc stream: ")
        }

        break
     }
     
     events = append(events, *res)
  } 

  return events, nil
}
