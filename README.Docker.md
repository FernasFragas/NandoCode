# Docker Usage Guide

This document explains how to build and run `nandocodego` in a Docker container.

## Quick Start

### Build the Docker Image

```bash
make docker-build
```

This creates a multi-stage Docker image that:
- Uses the Go version configured in `Dockerfile` for building
- Creates a minimal Alpine Linux runtime image (~20MB)
- Runs as a non-root user for security
- Includes CA certificates for HTTPS

### Run the Application

#### Interactive Mode

Run a command interactively:

```bash
# Run with default command (--help)
make docker-run

# Run the doctor command
make docker-run ARGS="doctor"

# Run with specific version check
make docker-run ARGS="--version"
```

#### Background Mode

Run as a daemon (useful for long-running processes in future phases):

```bash
make docker-run-daemon ARGS="doctor"
```

#### Get a Shell

Get a shell inside the container for debugging:

```bash
make docker-shell
```

### Stop and Clean Up

```bash
# Stop running containers
make docker-stop

# Remove Docker images and containers
make docker-clean
```

## Using Docker Compose

For more complex setups, use Docker Compose. The included `docker-compose.yml` provides:

- Automatic volume mounts for all XDG directories
- Environment variable configuration
- Port mapping (8080:8080 for future web interface)
- Support for connecting to host-based Ollama
- Interactive TTY for CLI commands

**Quick Start:**

```bash
# Using make targets
make docker-compose-up        # Build and start
make docker-compose-logs      # Follow logs
make docker-compose-down      # Stop and remove

# Or use docker compose directly
docker compose up             # Build and run
docker compose up -d          # Run in background
docker compose logs -f        # View logs
docker compose down           # Stop
```

**Customizing Commands:**

Edit `docker-compose.yml` to change the default command:

```yaml
# Example: Run doctor command on startup
command: ["doctor"]

# Example: Run with debug logging
environment:
  - NANDOCODEGO_DEBUG=1
```

## Volume Mounts

The Docker setup follows XDG Base Directory specification and automatically mounts these directories from your host:

- `~/.config/nandocodego` → Container config directory (`XDG_CONFIG_HOME`)
- `~/.local/share/nandocodego` → Container data directory (`XDG_DATA_HOME`)
- `~/.cache/nandocodego` → Container cache directory (`XDG_CACHE_HOME`)
- `~/.local/state/nandocodego` → Container state directory (`XDG_STATE_HOME`)

This ensures your configuration, data, cache, and state persist across container runs and follow standard Linux directory conventions.

## Connecting to Ollama

### Option 1: Ollama on Host Machine

If Ollama is running on your host machine, you need to make it accessible from the container.

**On Linux:**
```bash
# Use host network mode
docker run --network host nandocodego:latest
```

**On macOS/Windows:**
```bash
# Use host.docker.internal
docker run --add-host host.docker.internal:host-gateway nandocodego:latest
```

Then pass `--ollama-url http://host.docker.internal:11434` to `nandocodego`.

### Option 2: Ollama in Docker

Run Ollama in a separate container and link them:

```yaml
# docker-compose.yml
version: '3.8'

services:
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama

  nandocodego:
    build: .
    depends_on:
      - ollama
    command: ["--model", "qwen3", "--ollama-url", "http://ollama:11434"]

volumes:
  ollama-data:
```

## Environment Variables

Configure the application using environment variables:

**XDG Base Directory Specification:**
- `XDG_CONFIG_HOME` - Config directory location (default: `/home/nandocodego/.config`)
- `XDG_DATA_HOME` - Data directory location (default: `/home/nandocodego/.local/share`)
- `XDG_CACHE_HOME` - Cache directory location (default: `/home/nandocodego/.cache`)
- `XDG_STATE_HOME` - State directory location (default: `/home/nandocodego/.local/state`)

**Application-Specific:**
- `NANDOCODEGO_DEBUG` - Enable debug logging (set to `1`)
- `NANDOCODEGO_HOST` - Host address for web server (future phases)
- `NANDOCODEGO_PORT` - Port for web server (future phases)
- `OLLAMA_API_KEY` - Optional direct Ollama Cloud credential for cloud-only model selection in `--print` and server mode

The current CLI does not read an Ollama endpoint from environment variables. Use the `--ollama-url` flag for custom endpoints.

Example:

```bash
docker run -e NANDOCODEGO_DEBUG=1 \
  nandocodego:latest --model qwen3 --ollama-url http://host.docker.internal:11434
```

## Building for Different Architectures

Build for multiple architectures using Docker buildx:

```bash
# Setup buildx (one time)
docker buildx create --name multiarch --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t nandocodego:latest \
  --push \
  .
```

## Security Notes

1. **Non-root User**: The container runs as user `nandocodego` (UID 1000) for security.
2. **Minimal Image**: Uses Alpine Linux base image (~5MB) for reduced attack surface.
3. **No Shell by Default**: The `sh` shell is only available when explicitly requested.
4. **CA Certificates**: Included for secure HTTPS connections.

## Troubleshooting

### Permission Issues

If you encounter permission errors with mounted volumes:

```bash
# Check the permissions of the host directories
ls -ld ~/.config/nandocodego \
       ~/.local/share/nandocodego \
       ~/.cache/nandocodego \
       ~/.local/state/nandocodego

# The container user has UID 1000, ensure your host user owns these directories
# or adjust permissions accordingly
chmod 755 ~/.config/nandocodego
chmod 755 ~/.local/share/nandocodego
chmod 755 ~/.cache/nandocodego
chmod 755 ~/.local/state/nandocodego
```

### Container Won't Start

Check the logs:

```bash
docker logs nandocodego-container
```

### Build Fails

Ensure you have:
- Docker 20.10+ installed
- At least 2GB of free disk space
- Network access to download Go modules

### Ollama Connection Issues

If nandocodego can't connect to Ollama:

1. Verify Ollama is running: `curl http://localhost:11434/api/tags`
2. Check firewall settings
3. Use the correct host address (`host.docker.internal` on macOS/Windows)

## Advanced Usage

### Custom Dockerfile

For development, you might want to modify the Dockerfile:

```dockerfile
# Add development tools
RUN apk add --no-cache vim curl

# Copy additional files
COPY scripts/ /home/nandocodego/scripts/
```

### Multi-stage Build Optimization

The Dockerfile uses multi-stage builds to:
1. Build with full Go toolchain (~800MB)
2. Create minimal runtime image (~20MB)
3. Reduce final image size by 97%

### Caching Dependencies

To speed up builds, dependencies are cached in a separate layer:

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
```

This means dependency downloads are cached and only invalidated when `go.mod` or `go.sum` change.

## Examples

### Check Version

```bash
make docker-run ARGS="--version"
# Output: nandocodego 0.0.0-dev (docker-build)
```

### Run Doctor

```bash
make docker-run ARGS="doctor"
# Output includes:
# - Version Information (Version, Commit, Build Time)
# - Runtime Information (Go Version, OS, Arch, CPUs)
# - Directory Paths (Config Dir, Data Dir)
# - Directory Status (existence and writability)
# - Environment Variables (XDG_*, NANDOCODEGO_DEBUG)
```

### Test LLM Client

```bash
# Run the example chat program (requires Ollama accessible)
docker run --rm -it \
  --add-host host.docker.internal:host-gateway \
  nandocodego:latest \
  sh -c "cd examples/chat && go run main.go --model qwen3 --prompt 'Hello!'"
```

### Interactive Shell

```bash
make docker-shell
# Inside container:
$ nandocodego --help
$ nandocodego doctor
$ nandocodego --version
```

## CI/CD Integration

Example GitHub Actions workflow:

```yaml
- name: Build Docker image
  run: make docker-build

- name: Test Docker image
  run: |
    make docker-run ARGS="--version"
    make docker-run ARGS="doctor"

- name: Run Docker tests
  run: |
    # Verify the doctor command output
    docker run --rm nandocodego:latest doctor | grep "Doctor check complete"
    
    # Verify XDG directories are configured
    docker run --rm nandocodego:latest doctor | grep "XDG_CONFIG_HOME"
```

### Complete CI Example

```yaml
name: Docker Build and Test

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      
      - name: Build Docker image
        run: make docker-build
      
      - name: Test version command
        run: make docker-run ARGS="--version"
      
      - name: Test doctor command
        run: make docker-run ARGS="doctor"
      
      - name: Verify XDG directories
        run: |
          docker run --rm nandocodego:latest sh -c '
            test -d /home/nandocodego/.config &&
            test -d /home/nandocodego/.local/share &&
            test -d /home/nandocodego/.cache &&
            test -d /home/nandocodego/.local/state
          '
```

## Further Reading

- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [Multi-stage Builds](https://docs.docker.com/build/building/multi-stage/)
