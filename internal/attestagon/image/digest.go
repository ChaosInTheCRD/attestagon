package image

import (
   "fmt"
   "encoding/json"
   corev1 "k8s.io/api/core/v1"
)

type PodTerminationMessage struct {
  Key string `json:"key"`
  Value string `json:"value"`
}

func FindImageDigest(pod *corev1.Pod) (string, error) {
  for _, status := range pod.Status.ContainerStatuses {
    message := []PodTerminationMessage{}
    json.Unmarshal([]byte(status.State.Terminated.Message), &message)

      for i := 0; i < len(message); i++ {
         if message[i].Key == "digest" {
            return message[i].Value, nil
         } else {
            continue
         }
      }
  }

  return "", fmt.Errorf("Could not find image digest in completeed pod status terminated messages") 
}
