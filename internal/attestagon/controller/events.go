package controller

import (
   corev1 "k8s.io/api/core/v1"
   "github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
   "github.com/cilium/tetragon/api/v1/tetragon"
)

func (c *Controller) GetRuntimeMetadata(predicate predicate.Predicate, pod corev1.Pod) ([]Metadata, error) {

   
  return nil
}

