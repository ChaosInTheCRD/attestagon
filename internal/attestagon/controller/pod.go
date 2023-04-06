package controller

import (
	"context"
	"fmt"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"
	"github.com/chaosinthecrd/attestagon/internal/attestagon/predicate"
	"github.com/in-toto/in-toto-golang/in_toto"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) ProcessPod(ctx context.Context, pod *corev1.Pod) error {
	for i := 0; i < len(c.Artifacts); i++ {
		if pod.Annotations["attestagon.io/artifact"] == c.Artifacts[i].Name && c.Artifacts[i].Name != "" && pod.Status.Phase == "Succeeded" && pod.Annotations["attestagon.io/attested"] != "true" {
			fmt.Println("Processing pod", pod.Name)

			var predicate predicate.Predicate
			predicate.Pod.Name = pod.Name
			predicate.Pod.Namespace = pod.Namespace

			metadata, err := c.GetRuntimeMetadata(ctx, predicate, pod)
			if err != nil {
				c.log.Error(err, "Failed to get tetragon runtime metadata: ")
			}

			for i := range metadata {
				err := predicate.ProcessEvent(&metadata[i])
				if err != nil {
					c.log.Error(err, "DO THIS LATER")
				}
			}

			statement := in_toto.Statement{
				StatementHeader: in_toto.StatementHeader{
					Type:          "https://in-toto.io/Statement/v0.1",
					PredicateType: "https://attestagon.io/provenance/v0.1",
					Subject:       []in_toto.Subject{{Name: c.Artifacts[i].Name}},
				},
				Predicate: predicate,
			}

			digest, err := image.FindImageDigest(pod)
			if err != nil {
				c.log.Error(err, "Failed to get image digest from pod: ")
			}

			imageRef := fmt.Sprintf("%s@%s", c.Artifacts[i].Ref, digest)
			c.log.Info("Signing and pushing attestation to", imageRef)

			err = image.SignAndPush(ctx, statement, imageRef, c.cosignConfig.PrivateKeyPath)
			if err != nil {
				c.log.Error(err, "Failed to sign and push image: ")
				return fmt.Errorf("Error signing and pushing image: %s", err.Error())
			}
		}
		continue
	}

	return nil
}
