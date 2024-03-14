package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"
	_ "github.com/in-toto/go-witness/signer/kms/aws"
	_ "github.com/in-toto/go-witness/signer/kms/gcp"
	"github.com/in-toto/in-toto-golang/in_toto"
	"github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/common"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) ProcessPod(ctx context.Context, pod *corev1.Pod, art *Artifact) error {
	for i := 0; i < len(c.artifacts); i++ {
		fmt.Println("Processing pod", pod.Name)

		// NOTE: I get a sense that we should be taking it now. Technically more events could come but :shrug:
		// Also we have already assembled the predicate while caching. This may make no sense and we might have to revisit.
		predicate := c.eventCache.Store[pod.Name]

		digest, err := image.FindImageDigest(pod)
		if err != nil {
			c.log.Error(err, "Failed to get image digest from pod: ")
		}

		dig := strings.Split(digest, ":")
		if len(dig) != 2 {
			return errors.New("invalid digest")
		}

		statement := in_toto.Statement{
			StatementHeader: in_toto.StatementHeader{
				Type:          "https://in-toto.io/Statement/v0.1",
				PredicateType: "https://attestagon.io/provenance/v0.1",
				Subject:       []in_toto.Subject{{Name: art.Name, Digest: common.DigestSet{dig[0]: dig[1]}}},
			},
			Predicate: predicate,
		}

		statementMarshaled, err := json.Marshal(statement)
		if err != nil {
			return errors.Join(err, fmt.Errorf("failed to marshal statement to json"))
		}

		os.WriteFile(fmt.Sprintf("./%s-statement.json", art.Name), statementMarshaled, 0644)

		imageRef := fmt.Sprintf("%s@%s", art.Ref, digest)
		c.log.Info("Signing and pushing attestation", "reference", imageRef)

		if c.signerConfig.KMSRef != "" {
			c.log.Info("KMS signing not implemented yet for non-witness mode")
		} else if c.signerConfig.PrivateKeyPath != "" {
			err = image.SignAndPush(ctx, statement, imageRef, c.signerConfig.PrivateKeyPath)
			if err != nil {
				c.log.Error(err, "Failed to sign and push image: ")
				return fmt.Errorf("error signing and pushing image: %s", err.Error())
			} else {
				return errors.New("no signer configuration provided")
			}
		}

		c.log.Info("Deleting pod from cache", "pod_name", pod.Name)
		delete(c.eventCache.Store, pod.Name)
	}

	return nil
}
