package predicate

import (
	"fmt"

	"github.com/cilium/tetragon/api/v1/tetragon"
)

func (p *Predicate) ProcessEvent(response *tetragon.GetEventsResponse) (bool, error) {
	switch response.Event.(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		exec := response.GetProcessExec()
		if exec.Process == nil {
			return false, fmt.Errorf("process field is not set")
		}

		if exec.Process.Pod.Name == p.Pod.Name && exec.Process.Pod.Namespace == p.Pod.Namespace {
			if p.ProcessesExecuted == nil {
				p.ProcessesExecuted = make(map[string]int)
			}

			p.ProcessesExecuted[exec.Process.Binary] = p.ProcessesExecuted[exec.Process.Binary] + 1

			// Adding command execution to the "CommandsExecuted"
			if p.CommandsExecuted == nil {
				p.CommandsExecuted = make([]CommandsExecuted, 0)
			}

			p.CommandsExecuted = append(p.CommandsExecuted, CommandsExecuted{Command: exec.Process.Binary, Arguments: exec.Process.Arguments})
			return true, nil
		}

		return false, nil
	case *tetragon.GetEventsResponse_ProcessExit:
		return false, nil
	case *tetragon.GetEventsResponse_ProcessKprobe:
		kprobe := response.GetProcessKprobe()
		if kprobe.Process == nil {
			return false, fmt.Errorf("process field is not set")
		}
		if kprobe.Process.Pod.Name == p.Pod.Name && kprobe.Process.Pod.Namespace == p.Pod.Namespace {
			switch kprobe.FunctionName {
			case "__x64_sys_write":
				// Check that there is a file argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
					if p.FilesWritten == nil {
						p.FilesWritten = make(map[string]int)
					}

					p.FilesWritten[kprobe.Args[0].GetFileArg().Path] = p.FilesWritten[kprobe.Args[0].GetFileArg().Path] + 1
					return true, nil
				}
				return false, nil
			case "__x64_sys_read":
				// Check that there is a file argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[0].GetFileArg() != nil {
					if p.FilesRead == nil {
						p.FilesRead = make(map[string]int)
					}

					p.FilesRead[kprobe.Args[0].GetFileArg().Path] = p.FilesRead[kprobe.Args[0].GetFileArg().Path] + 1
					return true, nil
				}
				return false, nil
			case "fd_install":
				// Check that there is a file argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[1] != nil && kprobe.Args[1].GetFileArg() != nil {
					if p.FilesOpened == nil {
						p.FilesOpened = make(map[string]int)
					}

					p.FilesOpened[kprobe.Args[1].GetFileArg().Path] = p.FilesOpened[kprobe.Args[1].GetFileArg().Path] + 1
					return true, nil
				}
				return false, nil
			case "__x64_sys_mount":
				// Check that there is an argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[0] != nil && kprobe.Args[1] != nil {
					if p.FilesystemsMounted == nil {
						p.FilesystemsMounted = make([]FilesystemMounted, 0)
					}

					p.FilesystemsMounted = append(p.FilesystemsMounted, FilesystemMounted{Source: kprobe.Args[0].GetStringArg(), Destination: kprobe.Args[1].GetStringArg()})
					return true, nil
				}
				return false, nil
			case "__x64_sys_setuid":
				// Check that there is an argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
					if p.UIDSet == nil {
						p.UIDSet = make(map[int]int)
					}

					p.UIDSet[int(kprobe.Args[0].GetIntArg())] = p.UIDSet[int(kprobe.Args[0].GetIntArg())] + 1
					return true, nil
				}
				return false, nil
			case "tcp_connect":
				// Check that there is an argument to log
				if len(kprobe.Args) > 0 && kprobe.Args[0] != nil {
					if p.TCPConnections == nil {
						p.TCPConnections = make([]TCPConnection, 0)
					}

					sa := kprobe.Args[0].GetSockArg()
					p.TCPConnections = append(p.TCPConnections, TCPConnection{SocketAddress: sa.Saddr, SocketPort: int(sa.Sport), DestinationAddress: sa.Daddr, DestinationPort: int(sa.Dport)})
					return true, nil
				}
				return false, nil
			default:
				return false, nil
			}
		}
		return false, nil
	// case *tetragon.GetEventsResponse_ProcessDns:
	// 	// dns := response.GetProcessDns()
	// 	return nil
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		// tp := response.GetProcessTracepoint()
		return true, nil
	}

	return true, fmt.Errorf("unknown event type")
}
