# Drift Detection Test Scenarios

This document provides test scenarios to verify that drift detection is working correctly after fixing the playbook idempotency issues.

## Prerequisites

- Ansible API service running: `./ansiappgo`
- Initial playbook execution completed successfully
- Drift detection enabled (runs every 3 minutes)

## Test Scenario 1: Service State Drift

### Test: Stop Nginx Service

```bash
# On the target server (GADHQMSCAP02.cce3.gpc)
sudo systemctl stop nginx
sudo systemctl status nginx
```

**Expected Result:**

- Drift detection should detect nginx service is stopped
- Auto-remediation should restart the service
- Look for logs: `"Drift detected - running remediation"`

### Verification1

```bash
# Check service status after remediation
sudo systemctl status nginx
curl http://GADHQMSCAP02.cce3.gpc
```

---

## Test Scenario 2: Configuration File Drift

### Test: Modify Nginx Configuration

```bash
# On the target server
sudo cp /etc/nginx/conf.d/custom.conf /etc/nginx/conf.d/custom.conf.backup
sudo echo "# Modified by drift test" | sudo tee -a /etc/nginx/conf.d/custom.conf
```

**Expected Result:**

- Drift detection should detect configuration file changes
- Auto-remediation should restore the correct configuration

### Verification 1

```bash
# Check if configuration was restored
sudo cat /etc/nginx/conf.d/custom.conf
```

---

## Test Scenario 3: File Ownership Drift

### Test: Change Directory Ownership

```bash
# On the target server
sudo chown root:root /var/log/nginx
sudo chown root:root /var/cache/nginx
ls -la /var/log/ | grep nginx
ls -la /var/cache/ | grep nginx
```

**Expected Result:**

- Drift detection should detect ownership changes
- Auto-remediation should restore nginx:nginx ownership

### Verification 2

```bash
# Check ownership after remediation
ls -la /var/log/ | grep nginx
ls -la /var/cache/ | grep nginx
```

---

## Test Scenario 4: Web Content Drift

### Test: Modify Index File

```bash
# On the target server
sudo cp /var/www/html/index.html /var/www/html/index.html.backup
sudo echo "<h1>DRIFT TEST - MODIFIED</h1>" | sudo tee /var/www/html/index.html
```

**Expected Result:**

- Drift detection should detect content changes
- Auto-remediation should restore the original index.html

### Verification 3

```bash
curl http://GADHQMSCAP02.cce3.gpc
# Should show the original nginx welcome page, not the drift test message
```

---

## Test Scenario 5: Package State Drift

### Test: Uninstall Nginx Package

```bash
# On the target server (DANGEROUS - will break service)
sudo systemctl stop nginx
sudo yum remove nginx -y
```

**Expected Result:**

- Drift detection should detect missing package
- Auto-remediation should reinstall nginx and restore configuration

### Verification 4

```bash
# Check package and service status
rpm -qa | grep nginx
sudo systemctl status nginx
curl http://GADHQMSCAP02.cce3.gpc
```

---

## Test Scenario 6: Firewall Configuration Drift

### Test: Disable HTTP Firewall Rule

```bash
# On the target server
sudo firewall-cmd --remove-service=http --permanent
sudo firewall-cmd --reload
sudo firewall-cmd --list-services
```

**Expected Result:**

- Drift detection should detect missing firewall rule
- Auto-remediation should re-enable HTTP service

### Verification 5

```bash
sudo firewall-cmd --list-services | grep http
curl http://GADHQMSCAP02.cce3.gpc  # Should work from external hosts
```

---

## Test Scenario 7: Positive Test - No False Drift

### Test: No Changes Made

```bash
# Wait for drift detection cycle (3 minutes)
# Monitor logs for false positives
```

**Expected Result:**

- NO drift should be detected
- NO remediation should run
- Logs should show: `"No drift detected"` or similar

---

## Monitoring Commands

### Real-time Log Monitoring

```bash
# Monitor the Ansible API logs
tail -f /path/to/ansiappgo.log

# Or if running in terminal:
# Watch the console output for drift detection messages
```

### Check Jobs API

```bash
# List recent jobs to see drift remediation activities
curl -s http://localhost:8080/api/jobs | jq '.'

# Get formatted output
curl -s http://localhost:8080/api/jobs

# Get summary
curl -s http://localhost:8080/api/jobs/summary
```

### Manual Drift Check

```bash
# Trigger manual playbook run to compare with drift detection
curl -X POST http://localhost:8080/api/playbook/run \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://git.cce3.gpc/vincent-zamora/playbook-test.git",
    "playbook_path": "playbooks/site.yml",
    "target_hosts": "GADHQMSCAP02.cce3.gpc"
  }'
```

---

## Expected Log Patterns

### Normal Operation (No Drift)

```bash
INF Starting drift detection component=drift
INF Loaded playbooks from state file component=drift playbook_count=1
INF Checking playbook for drift component=drift playbook=playbooks/site.yml
INF Ansible check mode completed changed_count=0 component=drift
INF No significant drift detected component=drift
```

### Drift Detected

```bash
INF Starting drift detection component=drift
INF Checking playbook for drift component=drift playbook=playbooks/site.yml
INF Ansible check mode completed changed_count=1 component=drift
WRN Drift detected - running remediation component=drift
INF Ansible remediation completed successfully component=drift
```

### Drift Details

```bash
INF Detected change in drift check change_type=removed|added component=drift
INF Detected significant package or service changes component=drift
```

---

## Troubleshooting

### If No Drift is Detected When Expected

1. Check if the change actually affects Ansible's desired state
2. Verify the playbook tasks cover the modified component
3. Check if `ignore_errors: yes` is masking the detection

### If False Drift is Still Detected

1. Check for remaining non-idempotent tasks
2. Look for tasks that always show "changed" status
3. Verify file timestamps, checksums, or other volatile attributes

### Performance Monitoring

- Drift detection should complete within 30-60 seconds
- High frequency detection (multiple times per minute) indicates issues
- Monitor system resources during drift remediation
