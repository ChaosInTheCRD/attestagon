package cache

import (
	"context"
	"fmt"

	"github.com/cilium/tetragon/api/v1/tetragon"
)

func getEventStream(ctx context.Context, client tetragon.FineGuidanceSensorsClient, filter *tetragon.Filter) (tetragon.FineGuidanceSensors_GetEventsClient, error) {
	// Only set these filters if they are not empty. We currently rely on Protobuf to
	// marshal empty lists as nil for filters to function properly. It doesn't work with
	// stdin mode since it doesn't go over the wire, causing all events to get filtered
	// out because empty allowlist does not match anything.
	// filter.PodRegex = []string{pod.Name}

	request := tetragon.GetEventsRequest{
		AllowList: []*tetragon.Filter{filter},
	}

	stream, err := client.GetEvents(ctx, &request)
	if err != nil {
		return nil, fmt.Errorf("failed to call GetEvents")
	}

	return stream, nil
}

func getPodFromEvent(event *tetragon.GetEventsResponse) *tetragon.Pod {
	switch event.Event.(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		return event.GetProcessExec().Process.Pod
	case *tetragon.GetEventsResponse_ProcessExit:
		return event.GetProcessExit().Process.Pod
	case *tetragon.GetEventsResponse_ProcessKprobe:
		return event.GetProcessKprobe().Process.Pod
	default:
		return nil
	}
}
