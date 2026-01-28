# gitea-tf-backend

[![ci](https://github.com/nbenn/gitea-tf-backend/actions/workflows/ci.yml/badge.svg)](https://github.com/nbenn/gitea-tf-backend/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/nbenn/gitea-tf-backend/graph/badge.svg?token=18V0XE7EOC)](https://codecov.io/gh/nbenn/gitea-tf-backend)

A lightweight HTTP backend for Terraform/OpenTofu that stores state in a Gitea repository.

State files are stored as commits in your Gitea repo, giving you full version history and the ability to use standard Git tools for inspection and recovery.

## Features

- Implements the Terraform HTTP backend protocol
- State locking support
- Token-based authentication
- State stored as files in a Gitea repository with full Git history
- Single binary, minimal dependencies

## Configuration

All configuration is via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GITEA_URL` | Yes | - | Gitea instance URL (e.g., `https://gitea.example.com`) |
| `GITEA_TOKEN` | Yes | - | Gitea API token with repo write access |
| `GITEA_OWNER` | Yes | - | Repository owner (user or organization) |
| `GITEA_REPO` | Yes | - | Repository name |
| `GITEA_BRANCH` | No | `main` | Branch to store state files |
| `LISTEN_ADDR` | No | `:8080` | Address to listen on |
| `AUTH_TOKEN` | No | - | Token for client authentication (recommended) |
| `MAX_BODY_SIZE_MB` | No | `50` | Maximum request body size in megabytes |

## Usage

### Running Locally

```bash
export GITEA_URL=https://gitea.example.com
export GITEA_TOKEN=your-gitea-api-token
export GITEA_OWNER=myorg
export GITEA_REPO=terraform-state
export AUTH_TOKEN=my-secret-token

./gitea-tf-backend
```

### Running with Docker

```bash
docker run -d \
  -e GITEA_URL=https://gitea.example.com \
  -e GITEA_TOKEN=your-gitea-api-token \
  -e GITEA_OWNER=myorg \
  -e GITEA_REPO=terraform-state \
  -e AUTH_TOKEN=my-secret-token \
  -p 8080:8080 \
  gitea-tf-backend
```

### Terraform Configuration

```hcl
terraform {
  backend "http" {
    address        = "https://tf-state.example.com/myproject"
    lock_address   = "https://tf-state.example.com/myproject"
    unlock_address = "https://tf-state.example.com/myproject"
    username       = "terraform"
    password       = "my-secret-token"
  }
}
```

The `username` field is ignored but required by Terraform. The `password` is your `AUTH_TOKEN`.

### OpenTofu Configuration

Same as Terraform - OpenTofu uses the same backend configuration format.

## State Storage Layout

State files are stored in the repository with this structure:

```
states/
└── {project-name}/
    └── terraform.tfstate
```

Each state update creates a commit, giving you full history of all state changes.

**Note:** Locks are held in-memory on the server, not in the repository. This keeps the Git history clean and avoids lock file pollution. The tradeoff is that locks are lost if the server restarts (which is generally fine since Terraform will re-acquire them).

## Building

```bash
# Build binary
go build -o gitea-tf-backend .

# Build Docker image
docker build -t gitea-tf-backend .
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/{name}` | Retrieve state |
| `POST` | `/{name}` | Save state |
| `LOCK` | `/{name}` | Acquire lock |
| `UNLOCK` | `/{name}` | Release lock |
| `GET` | `/health` | Health check (returns `{"status":"ok"}`) |
| `GET` | `/metrics` | Prometheus metrics |

## Monitoring

The `/metrics` endpoint exposes Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests (labels: `method`, `status`) |
| `http_request_duration_seconds` | Histogram | Request latency (labels: `method`) |
| `tfstate_locks_active` | Gauge | Number of currently held state locks |

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: 'gitea-tf-backend'
    static_configs:
      - targets: ['tf-state.example.com:8080']
```

## Security Notes

- Always set `AUTH_TOKEN` in production
- Use HTTPS (put behind a reverse proxy like Traefik/nginx)
- The Gitea token needs write access to the state repository
- Consider using a dedicated repository for state files
- The `/health` and `/metrics` endpoints do not require authentication

## License

MIT
