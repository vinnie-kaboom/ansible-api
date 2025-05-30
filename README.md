# Ansible API

A simple API service that helps you run Ansible playbooks remotely. This service allows you to execute playbooks stored in a Git repository through HTTP endpoints.

## Quick Start

### 1. Prerequisites

- Go 1.16 or later
- Ansible installed on the server
- Git repository with your Ansible playbooks
- Basic understanding of Ansible playbooks

### 2. Installation

```bash
# Clone the repository
git clone https://github.com/your-username/ansible-api.git
cd ansible-api

# Build the application
go build -o ansible-api cmd/api/main.go
```

### 3. Configuration

Create a `.env` file in the project root:

```env
ANSIBLE_PLAYBOOK_PATH=/etc/ansible/playbooks
ANSIBLE_REPO_URL=https://github.com/your-org/your-playbooks.git
PORT=8080
```

### 4. Running the Service

```bash
# Start the service
./ansible-api
```

The service will start on port 8080 (or the port specified in your .env file).

## Using the API

### Run a Playbook

```bash
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "site",
    "inventory": "production",
    "extra_vars": {
      "env": "prod",
      "app_name": "myapp"
    }
  }'
```

### Check Playbook Status

```bash
curl http://localhost:8080/status/{job_id}
```

## Available Playbooks

The service is configured to run these playbooks:
- `site` - Main deployment playbook
- `deploy` - Application deployment
- `backup` - Backup operations
- `maintenance` - Maintenance tasks

## Testing

### 1. Local Testing

```bash
# Test the API is running
curl http://localhost:8080/health

# Test running a simple playbook
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "site",
    "inventory": "local",
    "extra_vars": {
      "env": "dev",
      "app_name": "testapp"
    }
  }'
```

### 2. Production Testing

1. Deploy to your server:
```bash
ansible-playbook -i inventory/production.ini site.yml -e "env=prod app_name=myapp"
```

2. Test the deployment:
```bash
# Check if the service is running
curl http://your-server:8080/health

# Run a test playbook
curl -X POST http://your-server:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "site",
    "inventory": "production",
    "extra_vars": {
      "env": "prod",
      "app_name": "myapp"
    }
  }'
```

## Monitoring

The service includes monitoring endpoints:
- Prometheus metrics: `http://your-server:9090`
- Node metrics: `http://your-server:9100/metrics`
- Nginx metrics: `http://your-server:9113/metrics`
- Application metrics: `http://your-server:8080/metrics`

## Security

The service includes:
- Secure SSH configuration
- Fail2ban protection
- Firewall rules (in production)
- Secure file permissions

## Troubleshooting

1. Check the service logs:
```bash
journalctl -u ansible-api
```

2. Verify playbook execution:
```bash
ansible-playbook -i inventory/production.ini site.yml --check
```

3. Test connectivity:
```bash
curl -v http://localhost:8080/health
```

## Support

For issues and feature requests, please create an issue in the GitHub repository.