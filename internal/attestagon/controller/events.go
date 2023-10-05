package controller

import (
	"strings"

	"github.com/cilium/tetragon/api/v1/tetragon"
)

func processEvent(exec *tetragon.ProcessExec) *tetragon.Pod {
	if !strings.Contains(exec.Process.Binary, "dev/fd") {
		return nil
	}

	po := exec.GetProcess().GetPod()
	if po == nil {
		return nil
	}

	return po
}
