package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/in-toto/in-toto-golang/in_toto"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) ProcessPod(ctx context.Context, pod *tetragon.Pod, artifact Artifact, containers []predicate.Container, wg *sync.WaitGroup) error {
	defer wg.Done()
	stream, err := c.tetragonClient.GetEvents(ctx, &tetragon.GetEventsRequest{})
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get events from tetragon client"))
	}

	c.log.Info("created stream for pod", "pod", pod.Name)

	time.Sleep(100 * time.Second)

	// we don't want two functions to write to the predicate at the same time and cause a problem
	mu := sync.Mutex{}

	events := []tetragon.GetEventsResponse{}
	// we want to start the collection process as early as possible. We want to asynchronously find out what containers we need to see finish before accepting that the pod has completed.
	for len(containers) == 0 {
		if ctx.Err() != nil {
			return nil
		}
		res, err := stream.Recv()
		if err != nil {
			c.log.Error(err, "Failed to get grpc stream response: ")
			serr := stream.CloseSend()
			if serr != nil {
				c.log.Error(err, "Failed to close grpc stream: ")
				return err
			}
			return err
		}
		events = append(events, *res)
	}

	pred := &predicate.Predicate{
		Pod:           predicate.Pod{Name: pod.Name, Namespace: pod.Namespace},
		ContainersRun: containers,
	}

	go pred.ProcessEvents(&events, &mu)

	for ctx.Err() == nil {
		// I really dont like this. It's very janky but for now we need to just accept a synchronization problem between Kubernetes API Server and Tetragon Events API
		pod, err := c.clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, v1.GetOptions{})
		if err != nil {
			return err
		}
		var ft time.Time
		if pod.Status.Phase != "Running" && ft.IsZero() {
			ft = time.Now()
		}
		if time.Since(ft) > (1 * time.Minute) {
			c.log.Info("Provenance generation for pod %s failed. 1 minute has passed since the processing started and the pod has finished")
			return nil
		}

		finished := true
		for _, n := range pred.ContainersRun {
			if !n.Pid1Exec || !n.Pid1Exit {
				finished = false
				break
			}
		}
		if finished {
			break
		}

		res, err := stream.Recv()
		if err != nil {
			c.log.Error(err, "Failed to get grpc stream response: ")
			serr := stream.CloseSend()
			if serr != nil {
				c.log.Error(err, "Failed to close grpc stream: ")
				return err
			}
			return err
		}

		pred.ProcessEvent(res, &mu)
	}

	statement := in_toto.Statement{
		StatementHeader: in_toto.StatementHeader{
			Type:          "https://in-toto.io/Statement/v0.1",
			PredicateType: "https://attestagon.io/provenance/v0.1",
			Subject:       []in_toto.Subject{{Name: artifact.Name}},
		},
		Predicate: pred,
	}

	po, err := c.clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, v1.GetOptions{})
	if err != nil {
		return err
	}
	digest, err := image.FindImageDigest(po)
	if err != nil {
		c.log.Error(err, "Failed to get image digest from pod: ")
	}

	imageRef := fmt.Sprintf("%s@%s", artifact.Ref, digest)
	c.log.Info("Signing and pushing attestation to", imageRef)

	err = image.SignAndPush(ctx, statement, imageRef, c.cosignConfig.PrivateKeyPath)
	if err != nil {
		c.log.Error(err, "Failed to sign and push image: ")
		return fmt.Errorf("Error signing and pushing image: %s", err.Error())
	}

	return nil
}
