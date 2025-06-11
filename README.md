# Ansible API (Go Edition)

A modern Go-based API service to run Ansible playbooks remotely. This service allows you to execute playbooks stored in Git repositories (public or private, including GitHub App integration) through HTTP endpoints. It supports job management, logging, and secure execution.

## Features

- Execute Ansible playbooks from Git repositories via HTTP API
- Supports GitHub App authentication for private repos
- Asynchronous job processing with status tracking
- File upload for playbooks and inventories
- Configurable worker pool and rate limiting
- Structured logging with zerolog
- Health check and job management endpoints
- Configuration via environment variables

## Prerequisites

- Go 1.18 or later
- Ansible installed on the server (in PATH)
- Git installed on the server

## Installation

```bash
# Clone the repository
git clone https://github.com/your-username/ansible-api.git
cd ansible-api

# Build the application
go build -o ansible-api ./cmd/service
```

## Configuration

The application is configured using environment variables. Copy the example environment file and update the values:

```bash
cp .env.example .env
```

Required environment variables:

- `GITHUB_APP_ID`: Your GitHub App ID
- `GITHUB_INSTALLATION_ID`: Your GitHub App Installation ID
- `GITHUB_PRIVATE_KEY_PATH`: Path to your GitHub App private key
- `GITHUB_API_BASE_URL`: GitHub API base URL (default: `https://api.github.com`)

Optional environment variables

- `PORT`: Server port (default: 8080)
- `WORKER_COUNT`: Number of worker goroutines (default: 4)
- `RETENTION_HOURS`: Hours to retain temporary files (default: 24)
- `TEMP_PATTERNS`: Comma-separated list of temporary file patterns (default: *_site.yml,*_hosts)
- `RATE_LIMIT_REQUESTS_PER_SECOND`: Rate limit for API requests (default: 10)

## Running the Server

```bash
# Using environment variables
source .env
./ansible-api
```

## API Endpoints

### Health Check

```bash
curl http://localhost:8080/api/health
```

### Run Playbook (Git Repo)

```bash
curl -X POST http://localhost:8080/api/playbook/run \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://github.com/OWNER/REPO.git",
    "playbook_path": "playbooks/site.yml",
    "inventory": {
      "webservers": {
        "ansible_user": "user",
        "ansible_password": "password"
      }
    }
  }'
```

### Upload Playbook File

```bash
curl -X POST http://localhost:8080/api/upload/playbook \
  -F "file=@/path/to/playbook.yml"
```

### Upload Inventory File

```bash
curl -X POST http://localhost:8080/api/upload/inventory \
  -F "file=@/path/to/inventory.ini"
```

### List Jobs

```bash
curl http://localhost:8080/api/jobs
```

### Get Job Status

```bash
curl http://localhost:8080/api/jobs/<job_id>
```

### Retry Job

```bash
curl -X POST http://localhost:8080/api/jobs/<job_id>/retry
```

### Cancel Job (if implemented)

```bash
curl -X POST http://localhost:8080/api/jobs/<job_id>/cancel
```

## Security

- GitHub App credentials are stored securely using environment variables
- Private keys are never committed to version control
- API endpoints are rate-limited
- Temporary files are automatically cleaned up
- All sensitive data is encrypted at rest

## Troubleshooting

- Ensure all required environment variables are set
- Check the logs for authentication errors
- Verify GitHub App permissions and installation
- Ensure Ansible and Git are in your PATH

## License

MIT
