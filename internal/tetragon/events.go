package tetragon

import (
	"fmt"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/cilium/tetragon/api/v1/tetragon"
)

type TetragonEvent struct {
	PodName      string
	PodNamespace string
	Type         string
	Body         interface{}
}

func ProcessEvent(response *tetragon.GetEventsResponse, predicate predicate.Predicate) (*TetragonEvent, error) {
	switch response.Event.(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		exec := response.GetProcessExec()
		if exec.Process == nil {
			return nil, fmt.Errorf("process field is not set")
		}

		event := &TetragonEvent{
			PodName:      exec.Process.Pod.Name,
			PodNamespace: exec.Process.Pod.Namespace,
			Type:         "ProcessExec",
			Body:         exec.Process.Binary,
		}

		return event, nil
	case *tetragon.GetEventsResponse_ProcessExit:
		return nil, fmt.Errorf("Event not processed: %s", response)
	case *tetragon.GetEventsResponse_ProcessKprobe:
		kprobe := response.GetProcessKprobe()
		if kprobe.Process == nil {
			return nil, fmt.Errorf("process field is not set")
		}
		switch kprobe.FunctionName {
		case "__x64_sys_write":
			// Check that there is a file argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {

				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "SysWrite",
					Body:         kprobe.Args[0].GetFileArg().Path,
				}

				return event, nil
			}
			return nil, fmt.Errorf("Event not processed: %s", response)
		case "__x64_sys_read":
			// Check that there is a file argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "SysRead",
					Body:         kprobe.Args[0].GetFileArg().Path,
				}

				return event, nil
			}
			return nil, fmt.Errorf("Event not processed: %s", response)
		case "fd_install":
			// Check that there is a file argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[1] != nil && kprobe.Args[1].GetFileArg() != nil {
				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "FdInstall",
					Body:         kprobe.Args[0].GetFileArg().Path,
				}

				return event, nil
			}
			return nil, fmt.Errorf("Event not processed: %s", response)
		case "__x64_sys_mount":
			// Check that there is an argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[1] != nil {
				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "SysMount",
					Body:         kprobe.Args[0].GetFileArg().Path,
				}

				return event, nil
			}
			return nil, fmt.Errorf("Event not processed: %s", response)
		case "__x64_sys_setuid":
			// Check that there is an argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "SysSetUID",
					Body:         kprobe.Args[0].GetIntArg(),
				}

				return event, nil
			}
			return nil, fmt.Errorf("Event not processed: %s", response)
		case "tcp_connect":
			// Check that there is an argument to log
			if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
				event := &TetragonEvent{
					PodName:      kprobe.Process.Pod.Name,
					PodNamespace: kprobe.Process.Pod.Namespace,
					Type:         "TCPConnect",
					Body:         kprobe.Args[0].GetSockArg(),
				}

				return event, nil
			}

			return nil, fmt.Errorf("Event not processed: %s", response)
		default:
			return nil, fmt.Errorf("Event not processed: %s", response)
		}
	// case *tetragon.GetEventsResponse_ProcessDns:
	//         // dns := response.GetProcessDns()
	//        return nil, fmt.Errorf("Event not processed: %s", response)
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		// tp := response.GetProcessTracepoint()
		return nil, fmt.Errorf("Event not processed: %s", response)
	}

	return nil, fmt.Errorf("Event not processed: %s", response)
}
