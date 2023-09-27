# Attestagon

<p align="center">
  <img src="./logo/attestagon.png" />
</p>

Attestagon is a Kubernetes controller that uses the eBPF observability tool [Tetragon](https://github.com/cilium/tetragon) to generate build [provenance](https://slsa.dev/provenance/v0.1) (*not* SLSA provenance) provenance for artifacts built
inside Kubernetes pods. The provenance attestation contains a formatted collection of Tetragon events that are specific to the pod the artifact was built within, including Process execution events, System call activity and I/O activity including network & file access. This would allow a user of an artifact to
more accurately determine the conditions under which the artifact in question was built, and write policy to ensure that certain events (e.g., network requests, file read/writes, number of shell sessions) are disallowed if they took place during the build.

## Note: Work In Progress
Please note that this project is still a work in progress. Since moving the project to use gRPC to communicate with Tetragon (as opposed to inspecting container logs) it is not in a working state. It is however compilable and can be run to get an idea of what the project is meant to do:
1. Get a Kubernetes cluster. It must be able to run Tetragon, so it's best to check the [requirements](https://github.com/cilium/tetragon#requirements).
2. Create some cosign keys. Attestagon uses these to sign the attestation. At this point, Attestagon only supports static keys, so it is probably best to generate some with `cosign generate-key-pair` (see [here](https://docs.sigstore.dev/cosign/signing_with_self-managed_keys/) for more details).
3. Ensure the credentials for the container image repository that you wish to use is available to your local machine. By default the [Makefile](./Makefile) looks for this in the standard location (`${HOME}/.docker/config.json`). If you want to use this anywhere else then I recommend you modify the Makefile. Alternatively if you have another way to get write access to the repository, then you can do that.
4. The configuration in this repository uses Tekton as the artifact builder, and currently it does not support anything else. To make the controller work, you will need to modify the `--destination` reference in the [tekton task](./hack/task.yaml) to point to a repository that you have access to with the credentials from earlier.
5. Also modify the [test configuration file](./hack/test-config.yaml) to reflect the image reference that you intend to push the attestation to.
6. I think that's it? Now you can execute `make all` to role all the dependencies to your cluster.
7. port-forward the Tetragon pod so attestagon can communicate with it by executing `kubectl port-forward svc/tetragon 54321:54321`
8. Finally, run `go run ./cmd/attestagon --config-path hack/test-config.yaml --tetragon-server-address localhost:54321 --cosign-private-key-path <COSIGN_PRIVATE_KEY_PATH>`
9. And that's it!

The gRPC functionality isn't currently working because the controller doesn't have the gRPC connection established when the events take place, and Tetragon doesn't retrospectively send events. The best way forward would be a cache for the events or to go back to the old way of doing it which involved scraping the pod logs, but some thought needs to go into it on my end (and some more time!). If you think
you might have an answer to this problem and fancy contributing, feel free!


