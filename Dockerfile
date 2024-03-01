# NOTE: Saving time at the moment by just building in a custom Dockerfile. Getting build errors using the main Dockerfile.
#
# Builder stage
FROM golang:latest AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the current directory contents into the container at /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o attestagon ./cmd/atttestagon

# Final stage
FROM ubuntu:24.04

# Copy the built binary from the builder stage to /usr/bin/dev-runner
COPY --from=builder /app/attestagon /entrypoint

# Set the stop signal
STOPSIGNAL SIGQUIT

# Copy your config file to the specified location
# COPY ./config.toml /etc/gitlab-runner/config.toml

# Set the entrypoint to the Go binary
ENTRYPOINT ["/entrypoint"]
