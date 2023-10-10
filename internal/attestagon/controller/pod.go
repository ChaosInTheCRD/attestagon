package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/in-toto/in-toto-golang/in_toto"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) ProcessPod(ctx context.Context, pod *corev1.Pod, artifact Artifact, containers []predicate.Container) error {
	defer c.wg.Done()

	pred := &predicate.Predicate{
		Pod: predicate.Pod{Name: pod.Name, Namespace: pod.Namespace, ContainersRun: containers},
	}

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

		if time.Since(ft) > (1*time.Minute) && !ft.IsZero() {
			c.log.Info("Provenance generation for pod failed. 1 minute has passed since the processing started and the pod has finished", "pod", pod.Name)
			return nil
		}

		finished := true
		for _, n := range pred.Pod.ContainersRun {
			if !n.Pid1Exec || !n.Pid1Exit {
				finished = false
				break
			}
		}
		if finished {
			break
		}

		if len(c.eventCache.Cache[artifact.Name]) > 0 {
			for i, n := range c.eventCache.Cache[artifact.Name] {
				write, err := pred.ProcessEvent(&n)
				if err != nil {
					return err
				}
				if write {
					c.eventCache.DeleteEvent(artifact.Name, i)
				}
			}
		}
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
