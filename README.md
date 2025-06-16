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

The application is configured using HashiCorp Vault secrets. All sensitive configuration values are loaded from Vault.

### Required Vault Configuration

1. Enable KV secrets engine:
```bash
vault secrets enable -version=2 -path=kv kv
```

2. Create a policy for the application:
```bash
vault policy write ansible-policy -<<EOF
path "kv/data/ansible/*" {
  capabilities = ["read", "list"]
}
EOF
```

3. Create an AppRole:
```bash
vault write auth/approle/role/ansible-role \
  token_policies="ansible-policy" \
  token_ttl="1h" \
  token_max_ttl="4h"
```

4. Get role ID and secret ID:
```bash
vault read auth/approle/role/ansible-role/role-id
vault write -f auth/approle/role/ansible-role/secret-id
```

5. Set environment variables:
```bash
export VAULT_ROLE_ID=<role-id>
export VAULT_SECRET_ID=<secret-id>
```

### Required GitHub App Configuration

The following keys must be present in the `kv/ansible/github` secret:

- `app_id`: Your GitHub App ID
- `installation_id`: Your GitHub App Installation ID
- `private_key`: Your GitHub App private key content
- `api_base_url`: GitHub API base URL (required for GitHub Enterprise, e.g., `https://git.cce3.gpc/api/v3`)

### Optional Configuration

The following keys can be set in the `kv/ansible/api` secret:

- `port`: Server port (default: 8080)
- `worker_count`: Number of worker goroutines (default: 4)
- `retention_hours`: Hours to retain temporary files (default: 24)
- `temp_patterns`: Comma-separated list of temporary file patterns (default: *_site.yml,*_hosts)
- `rate_limit`: Rate limit for API requests (default: 10)

## Running the Server

```bash
# Set Vault authentication
export VAULT_ROLE_ID=<your-role-id>
export VAULT_SECRET_ID=<your-secret-id>

# Run the server
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
