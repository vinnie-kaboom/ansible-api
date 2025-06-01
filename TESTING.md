# Testing Guide

This guide provides step-by-step instructions to start and test the Ansible API and its UI.

## Prerequisites

1. Install required dependencies:
```bash
# Install Go
sudo dnf install golang

# Install Python and Flask
sudo dnf install python3 python3-pip
pip3 install flask

# Install Ansible
sudo dnf install ansible

# Install jq for JSON processing
sudo dnf install jq
```

## Starting the Services

### 1. Set Up Directory Structure

```bash
# Create required directories
sudo mkdir -p /etc/ansible/playbooks/{logs,scripts,templates,playbooks,inventory}
sudo chown -R $USER:$USER /etc/ansible/playbooks

# Copy dashboard template
cp templates/dashboard.html /etc/ansible/playbooks/templates/

# Create initial repos.json
echo "[]" > /etc/ansible/playbooks/repos.json
```

### 2. Start the API Service

```bash
# Build the API
go build -o ansible-api cmd/api/main.go

# Start the API service
./ansible-api
```

The API will start on port 8080. You should see output like:
```
Ansible API running on :8080
Playbook directory: /etc/ansible/playbooks
Allowed playbooks: [run_playbook site deploy backup maintenance rollback]
```

### 3. Start the Dashboard

```bash
# Navigate to playbooks directory
cd /etc/ansible/playbooks

# Start the dashboard
python3 scripts/log_dashboard.py
```

The dashboard will start on port 5000.

## Testing the Application

### 1. Basic Health Check

```bash
# Test API health
curl http://localhost:8080/health

# Expected output:
{"status":"healthy","time":"2024-03-14T12:00:00Z"}
```

### 2. Add a Test Repository

```bash
# Add a test repository
./scripts/manage_repos.sh add test-repo https://github.com/your-org/test-playbooks.git

# List repositories to verify
./scripts/manage_repos.sh list
```

### 3. Create Test Playbook

```bash
# Create a test playbook
cat > /etc/ansible/playbooks/playbooks/test.yml << 'EOF'
---
- name: Test Playbook
  hosts: localhost./scripts/manage_repos.sh add test-repo https://github.com/your-org/test-playbooks.git
  tasks:
    - name: Echo test message./scripts/manage_repos.sh add test-repo https://github.com/your-org/test-playbooks.git
      debug:
        msg: "This is a test playbook"
EOF

# Create inventory file
cat > /etc/ansible/playbooks/inventory/local.ini << 'EOF'
[local]
localhost ansible_connection=local
EOF
```

### 4. Run a Test Playbook

```bash
# Run the test playbook
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "run_playbook",
    "inventory": "local",
    "extra_vars": {
      "repo_url": "https://github.com/your-org/test-playbooks.git",
      "repo_name": "test-repo",
      "playbook_name": "test",
      "branch": "main"
    }
  }'

# Expected output:
{"job_id":"1234567890"}
```

### 5. Check Job Status

```bash
# Replace {job_id} with the ID from the previous response
curl http://localhost:8080/status/{job_id}

# Expected output:
{
  "id": "1234567890",
  "status": "completed",
  "start_time": "2024-03-14T12:00:00Z",
  "end_time": "2024-03-14T12:00:01Z",
  "output": "PLAY [Test Playbook] ..."
}
```

### 6. Test the Dashboard

1. Open your web browser and navigate to `http://localhost:5000`
2. You should see:
   - Statistics at the top (Total Repositories, Total Logs, etc.)
   - List of repositories on the left
   - Log display area on the right
3. Click on the "test-repo" in the repository list
4. Verify that the test playbook execution log appears

### 7. Test Error Handling

```bash
# Try to run a non-existent playbook
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "nonexistent",
    "inventory": "local",
    "extra_vars": {}
  }'

# Expected output:
{"error":"Invalid playbook name"}
```

### 8. Test Repository Management

```bash
# List repositories
./scripts/manage_repos.sh list

# Remove test repository
./scripts/manage_repos.sh remove test-repo

# Verify removal
./scripts/manage_repos.sh list
```

## Troubleshooting

### API Issues

1. Check API logs:
```bash
# If running in terminal, check the output
# If running as service:
journalctl -u ansible-api
```

2. Verify API is running:
```bash
curl -v http://localhost:8080/health
```

### Dashboard Issues

1. Check dashboard logs:
```bash
# If running in terminal, check the output
# If running as service:
journalctl -u ansible-dashboard
```

2. Verify dashboard is running:
```bash
curl -v http://localhost:5000
```

### Common Issues

1. Permission denied:
```bash
# Fix permissions
sudo chown -R $USER:$USER /etc/ansible/playbooks
```

2. Port already in use:
```bash
# Check what's using the port
sudo lsof -i :8080
sudo lsof -i :5000

# Kill the process if needed
sudo kill <PID>
```

3. Playbook not found:
```bash
# Verify playbook exists
ls -l /etc/ansible/playbooks/playbooks/
```

## Cleanup

```bash
# Stop the services
pkill ansible-api
pkill -f "python3 scripts/log_dashboard.py"

# Clean up test files
rm -rf /etc/ansible/playbooks/logs/test-repo
rm /etc/ansible/playbooks/playbooks/test.yml
``` 