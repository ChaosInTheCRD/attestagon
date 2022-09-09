package controller

import (
	"context"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	tetragoninternal "github.com/chaosinthecrd/attestagon/internal/tetragon"
	"github.com/cilium/tetragon/api/v1/tetragon"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) GetRuntimeMetadata(predicate predicate.Predicate, pod corev1.Pod, ctx context.Context) ([]tetragoninternal.TetragonEvent, error) {

  conn, err := c.dial(ctx)
  if err != nil {
          c.log.Error(err, "Error on receiving events: ")
  }

  c.log.Info("Start collecting runtime events")

  client := tetragon.NewFineGuidanceSensorsClient(conn)

  return []tetragoninternal.TetragonEvent{}, nil
}

