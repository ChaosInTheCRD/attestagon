package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/cilium/tetragon/api/v1/tetragon"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) GetRuntimeMetadata(ctx context.Context, predicate predicate.Predicate, pod *corev1.Pod) ([]tetragon.GetEventsResponse, error) {
	c.log.Info("Start collecting runtime events")

	stream, err := getEventStream(ctx, c.tetragonClient, pod)
	if err != nil {
		c.log.Error(err, "Failed to get tetragon events: ")
	}

	encoder := json.NewEncoder(os.Stdout)
	events := make([]tetragon.GetEventsResponse, 0)
	var i int
	for start := time.Now(); ; {
		if i%20 == 0 {
			if time.Since(start) > time.Second {
				break
			}
		}
		i++
		fmt.Println("Receiving from stream!")
		res, err := stream.Recv()
		if err != nil {
			c.log.Error(err, "Failed to get grpc stream response: ")

			err := stream.CloseSend()
			if err != nil {
				c.log.Error(err, "Failed to close grpc stream: ")
			}

			break
		}
		fmt.Println("Appending Events!")
		encoder.Encode(res)

		events = append(events, *res)
	}

	return events, nil
}

func getEventStream(ctx context.Context, client tetragon.FineGuidanceSensorsClient, pod *corev1.Pod) (tetragon.FineGuidanceSensors_GetEventsClient, error) {
	// Only set these filters if they are not empty. We currently rely on Protobuf to
	// marshal empty lists as nil for filters to function properly. It doesn't work with
	// stdin mode since it doesn't go over the wire, causing all events to get filtered
	// out because empty allowlist does not match anything.
	filter := &tetragon.Filter{}
	filter.Namespace = []string{pod.Namespace}
	// filter.PodRegex = []string{pod.Name}

	request := tetragon.GetEventsRequest{
		AllowList: []*tetragon.Filter{filter},
	}

	stream, err := client.GetEvents(ctx, &request)
	if err != nil {
		return nil, fmt.Errorf("Failed to call GetEvents")
	}

	return stream, nil
}
