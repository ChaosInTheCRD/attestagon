package image

import (
	"encoding/json"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
)

type PodTerminationMessage struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func FindImageDigest(pod *corev1.Pod) (string, error) {
	for _, status := range pod.Status.ContainerStatuses {
		message := []PodTerminationMessage{}
		json.Unmarshal([]byte(status.State.Terminated.Message), &message)

		for i := 0; i < len(message); i++ {
			log.Println("checking message", message[i].Key, "with value ", message[i].Value)
			if message[i].Key == "digest" {
				log.Println("found digest", message[i].Value)
				return message[i].Value, nil
			} else {
				continue
			}
		}
	}

	return "", fmt.Errorf("could not find image digest in completeed pod status terminated messages")
}
