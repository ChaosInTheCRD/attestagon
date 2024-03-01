package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
)

func createRsaKey() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	return priv, &priv.PublicKey, nil
}

func createTestKey() ([]byte, []byte, error) {
	privKey, _, err := createRsaKey()
	if err != nil {
		return nil, nil, err
	}

	keyBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}

	pubPem := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: keyBytes})
	privPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return pubPem, privPem, nil
}

func main() {
	pubPem, privPem, err := createTestKey()
	if err != nil {
		log.Fatal("Error creating RSA key: ", err)
	}
	os.WriteFile("private.pem", privPem, 0644)
	os.WriteFile("public.pem", pubPem, 0644)
}
