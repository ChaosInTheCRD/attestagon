package cache

import (
	"fmt"
	tetragon "github.com/cilium/tetragon/api/v1/tetragon"
	"time"
)

func (c *Cache) DeleteEvent(artifact string, element int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Cache[artifact][element] = c.Cache[artifact][len(c.Cache)-1]
	c.Cache[artifact][len(c.Cache)-1] = tetragon.GetEventsResponse{}
	c.Cache[artifact] = c.Cache[artifact][:len(c.Cache)-1]
}

// TODO do we need an error return statement? How to handle that?
func (c *Cache) cleanEvents(artifact string, responses []tetragon.GetEventsResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, response := range responses {
		switch response.Event.(type) {
		case *tetragon.GetEventsResponse_ProcessExec:
			exec := response.GetProcessExec()
			if exec.Process == nil {
				continue
			}
			fmt.Println(exec)

			st := time.Unix(int64(exec.GetProcess().GetStartTime().GetSeconds()), int64(exec.GetProcess().GetStartTime().GetNanos()))
			if time.Since(st) > c.eventTTL {
				c.log.Info("found event that has met ttl", "startTime", st)
				c.DeleteEvent(artifact, i)
			}

			continue
		case *tetragon.GetEventsResponse_ProcessExit:
			exit := response.GetProcessExit()
			if exit.Process == nil {
				continue
			}
			fmt.Println(exit)

			st := time.Unix(int64(exit.GetProcess().GetStartTime().GetSeconds()), int64(exit.GetProcess().GetStartTime().GetNanos()))
			if time.Since(st) > c.eventTTL {
				c.log.Info("found event that has met ttl", "startTime", st)
				c.DeleteEvent(artifact, i)
			}

			continue
		case *tetragon.GetEventsResponse_ProcessKprobe:
			kprobe := response.GetProcessKprobe()
			if kprobe.Process == nil {
				continue
			}
			fmt.Println(kprobe)

			st := time.Unix(int64(kprobe.GetProcess().GetStartTime().GetSeconds()), int64(kprobe.GetProcess().GetStartTime().GetNanos()))
			if time.Since(st) > c.eventTTL {
				c.log.Info("found event that has met ttl", "startTime", st)
				c.DeleteEvent(artifact, i)
			}

			continue
		// case *tetragon.GetEventsResponse_ProcessDns:
		// 	// dns := response.GetProcessDns()
		// 	return nil
		case *tetragon.GetEventsResponse_ProcessTracepoint:
			// tp := response.GetProcessTracepoint()

			continue
		default:
			continue

		}
	}
}

func (c *Cache) processEvent(response *tetragon.GetEventsResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch response.Event.(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		exec := response.GetProcessExec()
		if exec.Process == nil {
			return fmt.Errorf("process field is not set")
		}

		// TODO Make sure it checks the config for a matching artifact
		po := exec.GetProcess().GetPod()
		if po != nil {
			for i, n := range po.Labels {
				if n == "attestagion.io/artifact" {
					c.Cache[po.Labels[i+1]] = append(c.Cache[po.Labels[i+1]], *response)
				}
			}
		}

		return nil
	case *tetragon.GetEventsResponse_ProcessExit:
		exit := response.GetProcessExit()
		if exit.Process == nil {
			return fmt.Errorf("process field is not set")
		}

		po := exit.GetProcess().GetPod()
		if po != nil {
			for i, n := range po.Labels {
				if n == "attestagion.io/artifact" {
					c.Cache[po.Labels[i+1]] = append(c.Cache[po.Labels[i+1]], *response)
				}
			}
		}

		return nil
	case *tetragon.GetEventsResponse_ProcessKprobe:
		kprobe := response.GetProcessKprobe()
		if kprobe.Process == nil {
			return fmt.Errorf("process field is not set")
		}
		po := kprobe.GetProcess().GetPod()
		if po != nil {
			for i, n := range po.Labels {
				if n == "attestagion.io/artifact" {
					c.Cache[po.Labels[i+1]] = append(c.Cache[po.Labels[i+1]], *response)
				}
			}
		}
		return nil
	// case *tetragon.GetEventsResponse_ProcessDns:
	// 	// dns := response.GetProcessDns()
	// 	return nil
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		// tp := response.GetProcessTracepoint()
		return nil

	default:
		return fmt.Errorf("unknown event type")

	}
}
