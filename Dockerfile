# Stage 1: Build the application
ARG GO_VERSION=1.26.2
ARG ALPINE_VERSION=3.20

FROM golang:${GO_VERSION} AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends git make && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary.
# The module path is read from go.mod so ldflags do not hardcode a GitHub URL.
ARG VERSION=docker
ARG COMMIT=docker-build
RUN MODULE_PATH="$(go list -m)" && \
    BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -trimpath \
      -ldflags "-s -w \
        -X ${MODULE_PATH}/internal/version.Version=${VERSION} \
        -X ${MODULE_PATH}/internal/version.Commit=${COMMIT} \
        -X ${MODULE_PATH}/internal/version.BuildTime=${BUILD_TIME}" \
      -o /out/nandocodego \
      ./cmd/nandocodego

# Stage 2: Create minimal runtime image
FROM alpine:${ALPINE_VERSION}

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 nandocodego && \
    adduser -D -u 1000 -G nandocodego nandocodego

# Create directories for config, data, cache, and state.
RUN mkdir -p \
      /home/nandocodego/.config/nandocodego \
      /home/nandocodego/.local/share/nandocodego \
      /home/nandocodego/.cache/nandocodego \
      /home/nandocodego/.local/state/nandocodego && \
    chown -R nandocodego:nandocodego /home/nandocodego

# Switch to non-root user
USER nandocodego
WORKDIR /home/nandocodego

# Copy binary from builder
COPY --from=builder /out/nandocodego /usr/local/bin/nandocodego

# Set environment variables for XDG directories and the web listener.
ENV XDG_CONFIG_HOME=/home/nandocodego/.config
ENV XDG_DATA_HOME=/home/nandocodego/.local/share
ENV XDG_CACHE_HOME=/home/nandocodego/.cache
ENV XDG_STATE_HOME=/home/nandocodego/.local/state
ENV NANDOCODEGO_HOST=0.0.0.0
ENV NANDOCODEGO_PORT=8080

# Expose the browser-facing HTTP port for `nandocodego server`.
EXPOSE 8080

# Default command remains help; runtime scripts can call `server`.
ENTRYPOINT ["nandocodego"]
CMD ["--help"]
