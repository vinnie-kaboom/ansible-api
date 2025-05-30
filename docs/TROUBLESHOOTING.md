# Troubleshooting Guide

## Common Issues and Solutions

### 1. Service Won't Start

**Symptoms:**
- `systemctl status ansible-api` shows failed
- Service exits immediately after starting

**Solutions:**
```bash
# Check service logs
journalctl -u ansible-api -n 50

# Verify environment variables
cat .env

# Check file permissions
ls -l ansible-api
ls -l /etc/ansible/playbooks

# Restart the service
sudo systemctl restart ansible-api
```

### 2. API Not Responding

**Symptoms:**
- `curl http://localhost:8080/health` fails
- Connection refused errors

**Solutions:**
```bash
# Check if service is running
systemctl status ansible-api

# Check if port is in use
sudo lsof -i :8080

# Check firewall rules
sudo ufw status

# Test local connectivity
curl -v http://localhost:8080/health
```

### 3. Playbook Execution Fails

**Symptoms:**
- API returns error when running playbook
- Playbook fails to execute

**Solutions:**
```bash
# Check playbook syntax
ansible-playbook playbooks/test.yml --syntax-check

# Run playbook in verbose mode
ansible-playbook playbooks/test.yml -vvv

# Check Ansible logs
tail -f /var/log/ansible.log

# Verify playbook permissions
ls -l playbooks/
```

### 4. Permission Issues

**Symptoms:**
- "Permission denied" errors
- Cannot create or access files

**Solutions:**
```bash
# Fix directory permissions
sudo chown -R $USER:$USER /etc/ansible/playbooks
sudo chmod -R 755 /etc/ansible/playbooks

# Check service user
ps aux | grep ansible-api

# Verify file ownership
ls -la /etc/ansible/playbooks
```

### 5. Environment Variable Problems

**Symptoms:**
- Missing configuration
- Wrong paths or URLs

**Solutions:**
```bash
# Check current environment
env | grep ANSIBLE

# Verify .env file
cat .env

# Test environment in service
sudo systemctl show ansible-api -p Environment
```

## Diagnostic Commands

### System Health Check
```bash
# Check system resources
free -h
df -h
top -b -n 1

# Check service status
systemctl status ansible-api

# Check logs
journalctl -u ansible-api --since "1 hour ago"
```

### Network Check
```bash
# Test local connectivity
curl -v http://localhost:8080/health

# Check port availability
netstat -tulpn | grep 8080

# Test DNS resolution
nslookup github.com
```

### Ansible Check
```bash
# Verify Ansible installation
ansible --version

# Check playbook syntax
ansible-playbook playbooks/test.yml --syntax-check

# Test playbook execution
ansible-playbook playbooks/test.yml --check
```

## Recovery Procedures

### 1. Service Recovery
```bash
# Stop the service
sudo systemctl stop ansible-api

# Clear any stale processes
pkill -f ansible-api

# Start the service
sudo systemctl start ansible-api

# Check status
systemctl status ansible-api
```

### 2. Configuration Recovery
```bash
# Backup current config
cp .env .env.backup

# Restore default config
cat > .env << EOL
ANSIBLE_PLAYBOOK_PATH=/etc/ansible/playbooks
ANSIBLE_REPO_URL=https://github.com/your-org/your-playbooks.git
PORT=8080
EOL

# Restart service
sudo systemctl restart ansible-api
```

### 3. Playbook Recovery
```bash
# Clean playbook directory
sudo rm -rf /etc/ansible/playbooks/*

# Recreate structure
sudo mkdir -p /etc/ansible/playbooks/{inventory,roles}

# Set permissions
sudo chown -R $USER:$USER /etc/ansible/playbooks
sudo chmod -R 755 /etc/ansible/playbooks
```

## Getting Help

If you're still experiencing issues:

1. Check the logs:
```bash
journalctl -u ansible-api -f
```

2. Run the test playbook:
```bash
ansible-playbook playbooks/test.yml -vvv
```

3. Create an issue on GitHub with:
   - Error messages
   - System information
   - Steps to reproduce
   - Log files 