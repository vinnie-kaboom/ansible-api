# SSH Key Authentication Setup Guide

This guide covers setting up SSH key authentication for the Ansible API to work with both local and remote hosts without sudo password prompts.

## Prerequisites

- ✅ GitHub App configured and working
- ✅ Vault configured and accessible
- ✅ Ansible API running

## Step 1: Generate SSH Key Pair

Generate a dedicated SSH key pair for Ansible authentication:

```bash
# Generate SSH key pair (no passphrase for automation)
ssh-keygen -t rsa -b 4096 -f ~/.ssh/ansible_key -N ""

# Verify the keys were created
ls -la ~/.ssh/ansible_key*
```

## Step 2: Configure Local SSH Access

For local testing, add the public key to your authorized_keys:

```bash
# Add public key to authorized_keys for localhost access
cat ~/.ssh/ansible_key.pub >> ~/.ssh/authorized_keys

# Set proper permissions
chmod 600 ~/.ssh/authorized_keys
chmod 700 ~/.ssh
```

## Step 3: Store Private Key in Vault

Store the private key securely in Vault:

```bash
# Store the private key in Vault
vault kv put kv/ansible/ssh private_key="$(cat ~/.ssh/ansible_key)"

# Verify it was stored
vault kv get kv/ansible/ssh
```

## Step 4: Configure Remote Hosts (Optional)

For remote hosts, add the public key to their authorized_keys:

```bash
# Copy public key to remote host (replace with your remote host)
ssh-copy-id -i ~/.ssh/ansible_key.pub user@remote-host.com

# Or manually add to remote host's ~/.ssh/authorized_keys
cat ~/.ssh/ansible_key.pub | ssh user@remote-host.com "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys"
```

## Step 5: Update Inventory Configuration

The inventory file is already configured for SSH authentication:

```ini
# inventory/hosts.ini
[webservers]
localhost ansible_connection=ssh ansible_user=vincent ansible_ssh_private_key_file=/tmp/ansible_key
```

For remote hosts, update the inventory:

```ini
# Example for remote hosts
[webservers]
web1.example.com ansible_user=ubuntu
web2.example.com ansible_user=centos
db1.example.com ansible_user=admin
```

## Step 6: Test SSH Connection

Test that SSH key authentication works:

```bash
# Test local SSH connection
ssh -i ~/.ssh/ansible_key vincent@localhost "echo 'SSH key authentication works!'"

# Test remote SSH connection (if applicable)
ssh -i ~/.ssh/ansible_key user@remote-host.com "echo 'Remote SSH key authentication works!'"
```

## Step 7: Test Ansible API

Test the complete setup with your nginx playbook:

```bash
# Test the playbook execution
curl -X POST http://localhost:8080/api/playbook/run \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://github.com/vinnie-kaboom/ansible-repo.git",
    "playbook_path": "playbooks/site.yml",
    "target_hosts": "localhost"
  }'
```

## Step 8: Test Drift Detection

Test that drift detection works without sudo prompts:

```bash
# Wait for drift detection to run (every 3 minutes)
# Or manually trigger by modifying nginx configuration:
sudo echo "# drift test" >> /etc/nginx/conf.d/custom.conf

# Check logs for drift detection
tail -f /path/to/your/api/logs
```

## Configuration Files

### Current Inventory Setup

```ini
# inventory/hosts.ini
[webservers]
localhost ansible_connection=ssh ansible_user=vincent ansible_ssh_private_key_file=/tmp/ansible_key
```

### Current Playbook Setup

```yaml
# playbooks/site.yml
---
- name: Install and configure nginx
  hosts: webservers
  gather_facts: yes
  
  tasks:
    - name: Install nginx
      package:
        name: nginx
        state: present
    # ... rest of tasks
```

## Troubleshooting

### SSH Connection Issues

1. **Check SSH key permissions:**

   ```bash

   chmod 600 ~/.ssh/ansible_key
   chmod 600 ~/.ssh/authorized_keys
   ```

2. **Test SSH manually:**

   ```bash
   ssh -i ~/.ssh/ansible_key -v vincent@localhost
   ```

3. **Check SSH service:*

   ```bash
   sudo systemctl status sshd
   ```

### Vault Issues

1. **Verify Vault secret exists:**

   ```bash
   vault kv get kv/ansible/ssh
   ```

2. **Check Vault authentication:**

   ```bash
   vault token lookup
   ```

### Ansible Issues

1. **Test Ansible manually:**

   ```bash
   ansible-playbook -i inventory/hosts.ini playbooks/site.yml --private-key ~/.ssh/ansible_key
   ```

2. **Check Ansible version:**

   ```bash
   ansible --version
   ```

## Security Considerations

1. **SSH Key Security:**
   - Keep private key secure (600 permissions)
   - Rotate keys regularly
   - Use different keys for different environments

2. **Vault Security:**
   - Rotate Vault tokens regularly
   - Use least-privilege policies
   - Monitor Vault access logs

3. **Network Security:**
   - Use SSH key authentication only (disable password auth)
   - Restrict SSH access to specific IPs if needed
   - Use SSH config for additional security

## Environment Variables

The following environment variables are automatically set by the API:

```bash
ANSIBLE_PYTHON_INTERPRETER=/usr/bin/python3
ANSIBLE_HOST_KEY_CHECKING=False
```

## Next Steps

1. **Commit and push changes to repository:**

   ```bash
   git add .
   git commit -m "Add SSH key authentication setup"
   git push origin main
   ```

2. **Test with remote hosts:**
   - Update inventory with remote host details
   - Add SSH keys to remote hosts
   - Test playbook execution

3. **Monitor drift detection:**
   - Watch logs for drift detection
   - Test manual changes to verify remediation
   - Adjust ignorable patterns as needed

## Summary

This setup provides:

- ✅ **SSH key authentication** for both local and remote hosts
- ✅ **No sudo password prompts**
- ✅ **Secure credential management** via Vault
- ✅ **Scalable solution** for production environments
- ✅ **Proper drift detection** without false positives

The system is now ready for both local development and production deployment! 🚀
