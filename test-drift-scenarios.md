# Drift Detection Test Scenarios

This document provides comprehensive test scenarios to verify that drift detection is working correctly with the new GAD Infrastructure Ansible structure.

## Prerequisites

- Ansible API service running: `./ansiappgo`
- New GAD Infrastructure structure deployed: `gad-infra-ansible/`
- Initial playbook execution completed successfully using new structure
- Drift detection enabled (runs every 3 minutes)

### Initial Deployment (Required)

First, deploy the new structure to establish baseline state:

```bash
curl -X POST http://localhost:8080/api/execute \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://git.cce3.gpc/vincent-zamora/playbook-test.git",
    "playbook_path": "gad-infra-ansible/playbooks/webservers/deploy.yml",
    "target_hosts": "GADHQMSCAP02.cce3.gpc"
  }'
```

---

## Test Scenario 1: Service State Drift

### Test: Stop Nginx Service

```bash
# On the target server (GADHQMSCAP02.cce3.gpc)
sudo systemctl stop nginx
sudo systemctl status nginx
```

**Expected Result:**

- Drift detection should detect nginx service is stopped
- Auto-remediation should restart the service using webserver role
- Look for logs: `"Drift detected - running remediation"`

### Verification 1

```bash
# Check service status after remediation
sudo systemctl status nginx
sudo systemctl is-enabled nginx  # Should be enabled
curl http://GADHQMSCAP02.cce3.gpc  # Should return new landing page
```

---

## Test Scenario 2: Role-based Configuration Drift

### Test A: Modify Main Nginx Configuration

```bash
# On the target server
sudo cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.backup
sudo sed -i 's/worker_processes auto/worker_processes 1/' /etc/nginx/nginx.conf
sudo nginx -t  # Verify syntax is still valid
```

### Test B: Modify Virtual Host Configuration

```bash
# On the target server
sudo cp /etc/nginx/conf.d/custom.conf /etc/nginx/conf.d/custom.conf.backup
echo "# Modified by drift test - $(date)" | sudo tee -a /etc/nginx/conf.d/custom.conf
```

**Expected Result:**

- Drift detection should detect configuration file changes in role templates
- Auto-remediation should restore the correct configuration from webserver role
- Templates should regenerate with proper variables

### Verification 2

```bash
# Check if configuration was restored
sudo grep "worker_processes" /etc/nginx/nginx.conf  # Should be "auto"
sudo tail -5 /etc/nginx/conf.d/custom.conf  # Should not have test comment
sudo nginx -t  # Configuration should be valid
```

---

## Test Scenario 3: Role-based Directory Ownership Drift

### Test: Change Directory Ownership (Should NOT Cause False Drift)

```bash
# On the target server
sudo chown root:root /var/log/nginx
sudo chown root:root /var/cache/nginx
ls -la /var/log/ | grep nginx
ls -la /var/cache/ | grep nginx
```

**Expected Result:**

- Drift detection should detect ownership changes
- Auto-remediation should restore correct ownership per role variables
- nginx:nginx for log and cache directories
- root:root for configuration directories

### Verification 3

```bash
# Check ownership after remediation
ls -la /var/log/ | grep nginx        # Should be nginx:nginx
ls -la /var/cache/ | grep nginx      # Should be nginx:nginx
ls -la /etc/nginx/conf.d/            # Should be root:root
```

---

## Test Scenario 4: Enhanced Web Content Drift

### Test: Modify Role-generated Content

```bash
# On the target server
sudo cp /var/www/html/index.html /var/www/html/index.html.backup
sudo echo "<h1>DRIFT TEST - MODIFIED - $(date)</h1>" | sudo tee /var/www/html/index.html
```

**Expected Result:**

- Drift detection should detect content changes
- Auto-remediation should restore the original role-generated index.html
- New template should include system information and styling

### Verification

```bash
curl http://GADHQMSCAP02.cce3.gpc
# Should show:
# - "🎉 Nginx Successfully Configured!" header
# - System information cards
# - Professional styling
# - NOT the drift test message
```

---

## Test Scenario 5: Package and Role Dependencies Drift

### Test: Remove Nginx and Dependencies

```bash
# On the target server (CONTROLLED - will break service temporarily)
sudo systemctl stop nginx
sudo yum remove nginx -y
sudo rm -rf /etc/nginx  # Remove configuration
```

**Expected Result:**

- Drift detection should detect missing package and configuration
- Auto-remediation should reinstall nginx using webserver role
- All role tasks should execute: package install, config creation, service start
- Dependencies should be resolved automatically

### Verification 6

```bash
# Check complete restoration
rpm -qa | grep nginx                    # Package should be installed
sudo systemctl status nginx            # Service should be running
ls -la /etc/nginx/                     # Configuration should exist
curl http://GADHQMSCAP02.cce3.gpc     # Website should be accessible
sudo firewall-cmd --list-services | grep http  # Firewall should be configured
```

---

## Test Scenario 6: Variable-driven Configuration Drift

### Test: Modify Role-managed Variables

```bash
# On the target server
# Simulate changing worker_connections (role manages this)
sudo sed -i 's/worker_connections 1024/worker_connections 512/' /etc/nginx/nginx.conf

# Modify server_tokens setting
sudo sed -i '/server_tokens/d' /etc/nginx/conf.d/custom.conf
echo "    server_tokens on;" | sudo tee -a /etc/nginx/conf.d/custom.conf
```

**Expected Result:**

- Drift detection should detect variable-driven configuration changes
- Auto-remediation should restore values from role variables
- Templates should regenerate with correct values from group_vars/webservers.yml

### Verification 7

```bash
sudo grep "worker_connections" /etc/nginx/nginx.conf  # Should be 1024
sudo grep "server_tokens" /etc/nginx/conf.d/custom.conf  # Should be "off"
```

---

## Test Scenario 7: Cross-platform Compatibility Test

### Test: Verify Role Idempotency

```bash
# No changes made - test for false positives
# Wait for drift detection cycle (3 minutes)
# Monitor logs for false positives
```

**Expected Result:**

- NO drift should be detected after initial deployment
- NO remediation should run
- Logs should show: `"No significant drift detected"` or `changed=0`
- Role-based structure should be truly idempotent

### Verification 4

```bash
# Check API jobs for no unnecessary remediation
curl -s http://localhost:8080/api/jobs/summary
# Should show stable state, no recent remediation jobs
```

---

## Test Scenario 8: Role Handler Testing

### Test: Trigger Configuration Reload

```bash
# On the target server
# Make a minor change that should trigger handler
sudo touch /etc/nginx/conf.d/test-trigger.conf
echo "# Test file for handler trigger" | sudo tee /etc/nginx/conf.d/test-trigger.conf
```

**Expected Result:**

- Drift detection should detect new file
- Auto-remediation should remove unauthorized file
- Nginx should reload (not restart) via handler
- Service should remain running throughout

### Verification 5

```bash
ls /etc/nginx/conf.d/test-trigger.conf  # Should not exist after remediation
sudo systemctl status nginx            # Should show "active (running)" without restart
journalctl -u nginx --since "5 minutes ago" | grep reload  # Should show reload, not restart
```

---

## Enhanced Monitoring Commands

### Real-time Log Monitoring

```bash
# Monitor the Ansible API logs with filtering
tail -f /var/log/ansible-api.log | grep -E "(drift|remediation|changed)"

# Or if running in terminal, watch for specific patterns:
./ansiappgo 2>&1 | grep -E "(drift|changed|remediation)"
```

### Enhanced Jobs API Monitoring

```bash
# Get detailed job information
curl -s http://localhost:8080/api/jobs | jq '.[] | select(.status == "completed") | {id, playbook, duration, status}'

# Get formatted output with role information
curl -s http://localhost:8080/api/jobs

# Monitor for drift-specific jobs
curl -s http://localhost:8080/api/jobs | jq '.[] | select(.type == "drift_detection")'

# Get real-time summary
watch -n 10 'curl -s http://localhost:8080/api/jobs/summary'
```

### Advanced Testing Commands

```bash
# Test role-specific functionality
ansible-playbook -i gad-infra-ansible/inventory/hosts.yaml \
  gad-infra-ansible/playbooks/webservers/deploy.yml \
  --tags webserver,nginx --check --diff

# Test idempotency manually
ansible-playbook -i gad-infra-ansible/inventory/hosts.yaml \
  gad-infra-ansible/playbooks/webservers/deploy.yml \
  --check --diff
# Run twice - second run should show no changes

# Test specific role tags
ansible-playbook -i gad-infra-ansible/inventory/hosts.yaml \
  gad-infra-ansible/playbooks/webservers/deploy.yml \
  --tags config,test
```

---

## Expected Log Patterns (Updated)

### Normal Operation (No Drift) - Role-based

```bash
INF Starting drift detection component=drift
INF Loaded playbooks from state file component=drift playbook_count=1
INF Checking playbook for drift component=drift playbook=gad-infra-ansible/playbooks/webservers/deploy.yml
INF Ansible check mode completed changed_count=0 component=drift
INF No significant drift detected component=drift role=webserver
```

### Drift Detected - Role-based

```bash
INF Starting drift detection component=drift
INF Checking playbook for drift component=drift playbook=gad-infra-ansible/playbooks/webservers/deploy.yml
INF Ansible check mode completed changed_count=1 component=drift
WRN Drift detected - running remediation component=drift role=webserver
INF Ansible remediation completed successfully component=drift role=webserver
```

### Role-specific Drift Details

```bash
INF Detected change in drift check change_type=removed component=drift task="webserver : Start and enable nginx service"
INF Detected change in drift check change_type=added component=drift task="webserver : Create custom nginx vhost configuration"
INF Detected significant service changes - not ignorable component=drift role=webserver
```

---

## New Structure Testing Benefits

### Role-based Advantages

1. **Modular Testing**: Test individual role components
2. **Variable Consistency**: Role variables ensure consistent configuration
3. **Handler Management**: Proper service restart/reload management
4. **Tag Granularity**: Test specific aspects (config, packages, services)
5. **Idempotency**: Roles designed for repeated execution

### Enhanced Verification

1. **Template Validation**: Role templates generate consistent output
2. **Dependency Management**: Role dependencies are properly handled
3. **Cross-platform Support**: Role variables adapt to different OS families
4. **Security Compliance**: Role enforces security best practices
5. **Documentation**: Role meta provides clear documentation

### Troubleshooting with New Structure

#### Role-specific Issues

```bash
# Test individual role components
ansible-playbook -i gad-infra-ansible/inventory/hosts.yaml \
  gad-infra-ansible/playbooks/webservers/deploy.yml \
  --tags webserver --check -vvv

# Verify role variables
ansible-inventory -i gad-infra-ansible/inventory/hosts.yaml \
  --host GADHQMSCAP02.cce3.gpc --vars
```

#### Template Issues

```bash
# Test template rendering
ansible-playbook -i gad-infra-ansible/inventory/hosts.yaml \
  gad-infra-ansible/playbooks/webservers/deploy.yml \
  --tags config --check --diff
```

This updated testing approach ensures the new GAD Infrastructure Ansible structure works correctly with role-based drift detection and provides comprehensive coverage of all new features!
