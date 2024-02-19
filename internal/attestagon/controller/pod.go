package controller

import (
	"context"
	"fmt"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"
	"github.com/in-toto/in-toto-golang/in_toto"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) ProcessPod(ctx context.Context, pod *corev1.Pod, art *Artifact) error {
	for i := 0; i < len(c.artifacts); i++ {
		fmt.Println("Processing pod", pod.Name)

		// NOTE: I get a sense that we should be taking it now. Technically more events could come but :shrug:
		// Also we have already assembled the predicate while caching. This may make no sense and we might have to revisit.
		predicate := c.eventCache.Store[pod.Name]

		statement := in_toto.Statement{
			StatementHeader: in_toto.StatementHeader{
				Type:          "https://in-toto.io/Statement/v0.1",
				PredicateType: "https://attestagon.io/provenance/v0.1",
				Subject:       []in_toto.Subject{{Name: art.Name}},
			},
			Predicate: predicate,
		}

		digest, err := image.FindImageDigest(pod)
		if err != nil {
			c.log.Error(err, "Failed to get image digest from pod: ")
		}

		imageRef := fmt.Sprintf("%s@%s", art.Ref, digest)
		c.log.Info("Signing and pushing attestation to", imageRef)

		err = image.SignAndPush(ctx, statement, imageRef, c.cosignConfig.PrivateKeyPath)
		if err != nil {
			c.log.Error(err, "Failed to sign and push image: ")
			return fmt.Errorf("error signing and pushing image: %s", err.Error())
		}
	}

	return nil
}
