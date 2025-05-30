# Ansible API

A simple API service that helps you run Ansible playbooks remotely. This service allows you to execute playbooks stored in Git repositories through HTTP endpoints, with features like repository management, logging, and monitoring.

## Features

- Execute Ansible playbooks from Git repositories
- Repository management (add, remove, list repositories)
- Automatic backup and rollback capabilities
- Comprehensive logging and monitoring dashboard
- Secure execution environment
- Clean and simple API interface
- GitHub Actions integration for automated deployments

## Quick Start

### 1. Prerequisites

- Go 1.16 or later
- Ansible installed on the server
- Python 3.x (for dashboard)
- Git
- jq (for repository management)

### 2. Installation

```bash
# Clone the repository
git clone https://github.com/your-username/ansible-api.git
cd ansible-api

# Build the application
go build -o ansible-api cmd/api/main.go

# Install Python dependencies
pip3 install flask

# Create required directories
sudo mkdir -p /etc/ansible/playbooks/{logs,scripts,templates}
sudo chown -R $USER:$USER /etc/ansible/playbooks
```

### 3. Configuration

1. Create a `.env` file in the project root:
```env
# Where to store Ansible playbooks on your server
ANSIBLE_PLAYBOOK_PATH=/etc/ansible/playbooks

# Port number for the API
PORT=8080
```

2. Copy the dashboard template:
```bash
cp templates/dashboard.html /etc/ansible/playbooks/templates/
```

3. Create an initial `repos.json` file:
```bash
echo "[]" > /etc/ansible/playbooks/repos.json
```

### 4. Running the Services

1. Start the API service:
```bash
./ansible-api
```

2. Start the dashboard (in a separate terminal):
```bash
cd /etc/ansible/playbooks
python3 scripts/log_dashboard.py
```

The API will be available on port 8080, and the dashboard on port 5000.

## Using the System

### Repository Management

Use the `manage_repos.sh` script to manage repositories:

```bash
# List repositories
./scripts/manage_repos.sh list

# Add a repository
./scripts/manage_repos.sh add my-repo https://github.com/user/repo.git

# Remove a repository
./scripts/manage_repos.sh remove my-repo

# Run a playbook
./scripts/manage_repos.sh run my-repo site --branch main --extra "-e env=prod"

# View logs
./scripts/manage_repos.sh logs my-repo
```

### API Endpoints

1. Run a playbook:
```bash
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "run_playbook",
    "inventory": "local",
    "extra_vars": {
      "repo_url": "https://github.com/user/repo.git",
      "repo_name": "my-repo",
      "playbook_name": "site",
      "branch": "main",
      "rollback": true
    }
  }'
```

2. View dashboard:
- Open `http://localhost:5000` in your browser
- View statistics and logs for all repositories
- Monitor playbook execution status

## GitHub Actions Integration

### 1. Repository Setup

1. Create a `.github/workflows/ansible-deploy.yml` file in your playbook repository:

```yaml
name: Ansible Deployment

on:
  push:
    branches: [ main ]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Deployment environment'
        required: true
        default: 'staging'
        type: choice
        options:
          - staging
          - production

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout playbooks
        uses: actions/checkout@v3

      - name: Deploy to staging
        if: github.event.inputs.environment == 'staging' || github.event_name == 'push'
        run: |
          curl -X POST ${{ secrets.STAGING_API_URL }}/run-playbook \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${{ secrets.API_TOKEN }}" \
            -d '{
              "playbook": "run_playbook",
              "inventory": "staging",
              "extra_vars": {
                "repo_url": "${{ github.server_url }}/${{ github.repository }}.git",
                "repo_name": "staging-playbooks",
                "playbook_name": "site",
                "branch": "${{ github.ref_name }}",
                "rollback": true
              }
            }'

      - name: Deploy to production
        if: github.event.inputs.environment == 'production'
        run: |
          curl -X POST ${{ secrets.PROD_API_URL }}/run-playbook \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${{ secrets.API_TOKEN }}" \
            -d '{
              "playbook": "run_playbook",
              "inventory": "production",
              "extra_vars": {
                "repo_url": "${{ github.server_url }}/${{ github.repository }}.git",
                "repo_name": "prod-playbooks",
                "playbook_name": "site",
                "branch": "${{ github.ref_name }}",
                "rollback": true
              }
            }'
```

2. Add repository secrets in GitHub:
   - Go to your repository settings
   - Navigate to Secrets and Variables > Actions
   - Add the following secrets:
     - `STAGING_API_URL`: Your staging server API URL
     - `PROD_API_URL`: Your production server API URL
     - `API_TOKEN`: Your API authentication token (if implemented)

### 2. Manual Deployment

1. Go to your repository on GitHub
2. Click on the "Actions" tab
3. Select the "Ansible Deployment" workflow
4. Click "Run workflow"
5. Select the environment (staging/production)
6. Click "Run workflow"

### 3. Automatic Deployment

The workflow will automatically trigger when:
- Code is pushed to the main branch (deploys to staging)
- A new release is created (deploys to production)
- Manually triggered through the GitHub Actions interface

### 4. Monitoring Deployments

1. Check deployment status in GitHub Actions:
   - Go to the "Actions" tab
   - Click on the latest workflow run
   - View the execution logs

2. Monitor through the dashboard:
   - Open `http://your-server:5000`
   - Look for the repository-specific logs
   - Check execution status and details

### 5. Rollback

If a deployment fails:
1. The system will automatically attempt to rollback
2. Check the logs in the dashboard for rollback status
3. If automatic rollback fails, you can manually trigger a rollback:
   ```bash
   curl -X POST http://your-server:8080/run-playbook \
     -H "Content-Type: application/json" \
     -d '{
       "playbook": "run_playbook",
       "inventory": "production",
       "extra_vars": {
         "repo_url": "https://github.com/user/repo.git",
         "repo_name": "prod-playbooks",
         "playbook_name": "rollback",
         "branch": "main"
       }
     }'
   ```

## Testing

### 1. Local Testing

1. Add a test repository:
```bash
./scripts/manage_repos.sh add test-repo https://github.com/user/test-playbooks.git
```

2. Create a test log:
```bash
mkdir -p /etc/ansible/playbooks/logs/test-repo
echo "=== Playbook Execution Log ===
Start Time: $(date)
Repository: https://github.com/user/test-playbooks.git
Branch: main
Playbook: test.yml
============================" > /etc/ansible/playbooks/logs/test-repo/test.log
```

3. Test the dashboard:
- Open `http://localhost:5000`
- Verify that the test repository appears
- Check that the log is displayed correctly

4. Test API endpoints:
```bash
# Health check
curl http://localhost:8080/health

# Run a test playbook
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "run_playbook",
    "inventory": "local",
    "extra_vars": {
      "repo_url": "https://github.com/user/test-playbooks.git",
      "repo_name": "test-repo",
      "playbook_name": "test",
      "branch": "main"
    }
  }'
```

### 2. Production Testing

1. Deploy to your server:
```bash
# Copy files to server
scp ansible-api user@server:/usr/local/bin/
scp -r /etc/ansible/playbooks user@server:/etc/ansible/

# Set up systemd service
sudo cp scripts/ansible-dashboard.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable ansible-dashboard
sudo systemctl start ansible-dashboard
```

2. Test the deployment:
```bash
# Check if services are running
curl http://your-server:8080/health
curl http://your-server:5000

# Add a production repository
./scripts/manage_repos.sh add prod-repo https://github.com/org/prod-playbooks.git

# Run a production playbook
./scripts/manage_repos.sh run prod-repo site --branch main --extra "-e env=prod"
```

## Monitoring

The system includes:
- Real-time dashboard at `http://your-server:5000`
- Repository-specific logs
- Execution statistics
- Success/failure tracking

## Security

The system includes:
- Secure repository management
- Automatic backups before execution
- Rollback capabilities
- Logging of all operations
- Clean separation of concerns

## Troubleshooting

1. Check service logs:
```bash
# API service
journalctl -u ansible-api

# Dashboard
journalctl -u ansible-dashboard
```

2. Verify repository configuration:
```bash
cat /etc/ansible/playbooks/repos.json
```

3. Check log files:
```bash
ls -l /etc/ansible/playbooks/logs/
```

4. Test connectivity:
```bash
curl -v http://localhost:8080/health
curl -v http://localhost:5000
```

## Support

For issues and feature requests, please create an issue in the GitHub repository.