BINDIR ?= $(CURDIR)/bin
ARCH   ?= amd64

COSIGNKEYPATH ?= ${HOME}/.cosign/cosign.key

REPOCREDSPATH ?= ${HOME}/.docker/config.json

help:  ## display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: help build all clean

build: ## build version-checker
	mkdir -p $(BINDIR)
	CGO_ENABLED=0 go build -o ./$(BINDIR)/attestagon-controller ./cmd/attestagon

# Not really used at this point as Github Actions handling the builds
image: ## build docker image
	ko build ./cmd/attestagon --local

tetragon:
	kubectl apply -f deploy-tetragon -n kube-system
	echo "Waiting for CRDs..."
	sleep 10
attestagon:
	kubectl apply -f deploy -n kube-system

tekton: ## Deploy Tekton for testing purposes
	kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml

kyverno: ## Deploy Kyverno for testing purposes
	kubectl create -f https://raw.githubusercontent.com/kyverno/kyverno/main/config/install.yaml

setup-poc: ## Setup all the stuff related to the POC
	## We need to give tekton and attestagon access to the repo
	kubectl create secret generic repo-creds --from-file=config.json=$(REPOCREDSPATH) --namespace tekton-pipelines --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic repo-creds --from-file=config.json=$(REPOCREDSPATH) --namespace kube-system --dry-run=client -o yaml | kubectl apply -f -
	kubectl create secret generic cosign-creds --from-file=cosign.key=$(COSIGNKEYPATH) --namespace kube-system --dry-run=client -o yaml | kubectl apply -f -
	## Setting up tekton task and kyverno policy for testing
	kubectl apply -f ./hack/task.yaml -n tekton-pipelines
	# kubectl apply -f ./hack/kyverno-policy.yaml -n kyverno
clean: ## clean up created files
	rm -rf \
		$(BINDIR)

all: tetragon attestagon tekton setup-poc  ## runs each specified target
