# version_settings() enforces a minimum Tilt version
# https://docs.tilt.dev/api.html#api.version_settings
version_settings(constraint='>=0.22.2')

# config.main_path is the absolute path to the Tiltfile being run
# there are many Tilt-specific built-ins for manipulating paths, environment variables, parsing JSON/YAML, and more!
# https://docs.tilt.dev/api.html#api.config.main_path
tiltfile_path = config.main_path

local_resource(
  'attestagon',
  cmd='go build ./cmd/attestagon',
  serve_cmd='./attestagon --config-path hack/test-config.yaml --tetragon-server-address localhost:54321 --signer-kms-ref awskms:///arn:aws:kms:eu-north-1:339150376714:key/e127e81e-844d-44b1-8536-f81574796872 --tls-key-path=key.pem --tls-cert-path=cert.pem',
  deps=['**/*.go', 'go.mod', 'go.sum', 'hack/test-config.yaml', '~/.cosign/cosign.key', 'key.pem', 'cert.pem']
)

# print writes messages to the (Tiltfile) log in the Tilt UI
# the Tiltfile language is Starlark, a simplified Python dialect, which includes many useful built-ins
# config.tilt_subcommand makes it possible to only run logic during `tilt up` or `tilt down`
# https://github.com/bazelbuild/starlark/blob/master/spec.md#print
# https://docs.tilt.dev/api.html#api.config.tilt_subcommand
if config.tilt_subcommand == 'up':
    print("""
    Success!
    """)
