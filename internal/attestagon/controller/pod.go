package controller

import (
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chaosinthecrd/attestagon/internal/attestagon/image"
	witness "github.com/in-toto/go-witness"
	"github.com/in-toto/go-witness/archivista"
	"github.com/in-toto/go-witness/attestation"
	"github.com/in-toto/go-witness/attestation/environment"
	"github.com/in-toto/go-witness/attestation/material"
	"github.com/in-toto/go-witness/attestation/product"
	"github.com/in-toto/go-witness/cryptoutil"
	"github.com/in-toto/go-witness/dsse"
	"github.com/in-toto/go-witness/intoto"
	"github.com/in-toto/go-witness/log"
	"github.com/in-toto/go-witness/signer/kms"
	_ "github.com/in-toto/go-witness/signer/kms/aws"
	_ "github.com/in-toto/go-witness/signer/kms/gcp"
	"github.com/in-toto/in-toto-golang/in_toto"
	"github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/common"
	"github.com/sirupsen/logrus"
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

		predicate.Materials = make(map[string]cryptoutil.DigestSet)
		predicate.Materials[art.Name] = cryptoutil.DigestSet{cryptoutil.DigestValue{crypto.SHA256, false}: dig[1]}

		if c.witness {
			c.log.Info("Using witness to create attestation collection")
			opts := []witness.RunOption{
				witness.RunWithAttestors([]attestation.Attestor{environment.New(), product.New(), material.New()}),
			}

			l := logrus.New()
			log.SetLogger(l)

			res, err := witness.Run("attestagon", nil, opts...)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to run witness: "))
			}

			c.log.Info("Finished witness run")

			res.Collection.Attestations = append(res.Collection.Attestations, attestation.CollectionAttestation{
				Type:        "https://attestagon.io/provenance/v0.1",
				Attestation: predicate,
				StartTime:   time.Now(),
				EndTime:     time.Now(),
			})

			data, err := json.Marshal(&res.Collection)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to marshal collection to json"), err)
			}

			hash, err := cryptoutil.HashFromString(dig[0])
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to hash from string"), err)
			}

			stmt, err := intoto.NewStatement(attestation.CollectionType, data, map[string]cryptoutil.DigestSet{art.Name: {cryptoutil.DigestValue{hash, false}: dig[1]}})
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to create in-toto statement"), err)
			}

			stmtJson, err := json.Marshal(&stmt)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to marshal in-toto statement"), err)
			}

			var signer cryptoutil.Signer
			if c.signerConfig.KMSRef != "" {
				kms := kms.KMSSignerProvider{
					Reference: c.signerConfig.KMSRef,
					HashFunc:  crypto.SHA256,
					Options:   kms.ProviderOptions(),
				}

				signer, err = kms.Signer(ctx)
				if err != nil {
					return errors.Join(err, fmt.Errorf("failed to create signer"))
				}
			} else if c.signerConfig.PrivateKeyPath != "" {
				f, err := os.ReadFile(c.signerConfig.PrivateKeyPath)
				if err != nil {
					return errors.Join(err, fmt.Errorf("failed to read private key"))
				}

				signer, err = cryptoutil.NewSignerFromReader(bytes.NewReader(f))
				if err != nil {
					return errors.Join(err, fmt.Errorf("failed to create signer"))
				}
			} else {
				return errors.New("no signer configuration provided")
			}

			res.SignedEnvelope, err = dsse.Sign(intoto.PayloadType, bytes.NewReader(stmtJson), dsse.SignWithSigners(signer))
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to sign statement"))
			}

			signedBytes, err := json.Marshal(&res.SignedEnvelope)
			if err != nil {
				return fmt.Errorf("failed to marshal envelope: %w", err)
			}

			c.log.Info("Writing signed statement to file")
			os.WriteFile(fmt.Sprintf("./%s-statement-signed.json", art.Name), signedBytes, 0644)

			archivistaClient := archivista.New("https://archivista.testifysec.io")
			if gitoid, err := archivistaClient.Store(ctx, res.SignedEnvelope); err != nil {
				return fmt.Errorf("failed to store artifact in archivista: %w", err)
			} else {
				log.Infof("Stored in archivista as %v\n", gitoid)
			}

		} else {
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

		}

		c.log.Info("Deleting pod from cache", "pod_name", pod.Name)
		delete(c.eventCache.Store, pod.Name)
	}

	return nil
}
