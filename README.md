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
- Configuration via `config.cfg`

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

Create a `config.cfg` file in the project root. Example:

```ini
[server]
port = 8080
worker_count = 4

[files]
retention_hours = 24
temp_patterns = *_site.yml, *_hosts

[rate_limit]
requests_per_second = 10

[githubapp]
app_id = <your_github_app_id>
installation_id = <your_installation_id>
private_key_path = C:/path/to/your/private-key.pem
api_base_url = https://api.github.com
```

- For GitHub App integration, generate a private key in your GitHub App settings and set the correct App and Installation IDs.

## Running the Server

```bash
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

## Example config.cfg

```ini
[server]
port = 8080
worker_count = 4

[githubapp]
app_id = 12345
installation_id = 67890
private_key_path = C:/Users/youruser/Downloads/github-app.pem
api_base_url = https://api.github.com
```

## Notes

- All configuration is loaded from `config.cfg`.
- The server logs to stdout in structured format.
- Jobs are processed asynchronously; use the job endpoints to track status.
- For private GitHub repos, configure the GitHub App section and use HTTPS URLs.
- No shell scripts or Python dashboard are required or supported in this Go version.

## Troubleshooting

- Ensure Ansible and Git are installed and in your PATH.
- Check the logs for errors related to config, authentication, or playbook execution.
- For GitHub App issues, verify your App ID, Installation ID, and PEM file.

## License

MIT
