# Vault Setup and Usage Guide

## Table of Contents

- [Vault Setup and Usage Guide](#vault-setup-and-usage-guide)
  - [Table of Contents](#table-of-contents)
  - [Installation](#installation)
  - [Initial Setup](#initial-setup)
  - [Configuration](#configuration)
  - [Starting Vault](#starting-vault)
  - [Initializing Vault](#initializing-vault)
  - [Unsealing Vault](#unsealing-vault)
  - [Storing Secrets](#storing-secrets)
  - [Retrieving Secrets](#retrieving-secrets)
  - [Environment Variables](#environment-variables)
  - [Role-Based Access Control (RBAC)](#role-based-access-control-rbac)
  - [AppRole Authentication Setup](#approle-authentication-setup)
  - [Backup and Restore](#backup-and-restore)
  - [Monitoring and Logging](#monitoring-and-logging)
  - [Troubleshooting](#troubleshooting)
    - [Common Issues](#common-issues)
    - [Useful Commands](#useful-commands)
    - [Security Best Practices](#security-best-practices)
    - [Vault Service Fails to Start: Permission Denied](#vault-service-fails-to-start-permission-denied)
      - [**Checklist to Resolve:**](#checklist-to-resolve)
      - [Vault Fails to Initialize with 'read-only file system' Error](#vault-fails-to-initialize-with-read-only-file-system-error)
      - [403 Error or 'permission denied' When Accessing kv/\* Paths](#403-error-or-permission-denied-when-accessing-kv-paths)
  - [Structured Logging Configuration](#structured-logging-configuration)
  - [Additional Resources](#additional-resources)

## Installation

1 Download Vault:

  ```bash
  curl -L -o vault_1.15.6_linux_amd64.zip https://releases.hashicorp.com/vault/1.15.6/vault_1.15.6_linux_amd64.zip
  ```

2 Unzip and install:

  ```bash
  unzip vault_1.15.6_linux_amd64.zip
  sudo mv vault /usr/local/bin/
  ```

3 Create Vault user and directories:

  ```bash
  sudo useradd --system --home /etc/vault.d --shell /bin/false vault
  sudo mkdir -p /etc/vault.d
  sudo chown -R vault:vault /etc/vault.d
  ```

4 Verify installation:

  ```bash
  vault version
  ```

## Initial Setup

1 Create Vault configuration file:

  ```bash
  sudo nano /etc/vault.d/vault.hcl
  ```

2 Add the following configuration:

  ```hcl
    storage "file" {
    path = "/etc/vault.d/data"
  }

  listener "tcp" {
    address     = "127.0.0.1:8200"
    tls_disable = 1
  }

  ui = true
  disable_mlock = true

# Enable audit logging
audit "file" {
  path = "/etc/vault.d/audit.log"
  type = "file"
}
```

1 Create data directory and set permissions:

  ```bash
sudo mkdir -p /etc/vault.d/data
sudo chown -R vault:vault /etc/vault.d/data
sudo chmod 700 /etc/vault.d/data
```

## Configuration

1 Create systemd service file:

  ```bash
  sudo nano /etc/systemd/system/vault.service
  ```

2 Add the following configuration:

  ```ini
  [Unit]
  Description="HashiCorp Vault - A tool for managing secrets"
  Documentation=https://www.vaultproject.io/docs/
  Requires=network-online.target
  After=network-online.target
  ConditionFileNotEmpty=/etc/vault.d/vault.hcl

  [Service]
  User=vault
  Group=vault
  ProtectSystem=full
  ReadWritePaths=/etc/vault.d
  ProtectHome=read-only
  PrivateTmp=yes
  PrivateDevices=yes
  SecureBits=keep-caps
  AmbientCapabilities=CAP_IPC_LOCK
  Capabilities=CAP_IPC_LOCK+ep
  CapabilityBoundingSet=CAP_SYSLOG CAP_IPC_LOCK
  NoNewPrivileges=yes
  ExecStart=/usr/local/bin/vault server -config=/etc/vault.d/vault.hcl
  ExecReload=/bin/kill --signal HUP $MAINPID
  KillMode=process
  KillSignal=SIGINT
  Restart=on-failure

  [Install]
  WantedBy=multi-user.target
```

## Starting Vault

1 Reload systemd:

  ```bash
  sudo systemctl daemon-reload
  ```

2 Start Vault:

  ```bash
  sudo systemctl start vault
  ```

3 Enable Vault to start on boot:

  ```bash
  sudo systemctl enable vault
  ```

4 Check status:

  ```bash
  sudo systemctl status vault
```

5 Verify Vault is running:

  ```bash
  vault status
  ```

## Initializing Vault

  1. Set Vault address:

  ```bash
  export VAULT_ADDR='http://127.0.0.1:8200'
  ```

2 Initialize Vault:

  ```bash
  vault operator init
  ```

3 Save the output securely. You'll receive:
      - 5 Unseal Keys
      - 1 Initial Root Token

Example output:

```bash
Unseal Key 1: xxxxx
Unseal Key 2: xxxxx
Unseal Key 3: xxxxx
Unseal Key 4: xxxxx
Unseal Key 5: xxxxx
Initial Root Token: xxxxx

Vault initialized with 5 key shares and a key threshold of 3. Please securely
distribute the key shares printed above. When the Vault is re-sealed,
restarted, or stopped, you must supply at least 3 of these keys to unseal it
before it can start servicing requests.

Vault does not store the generated master key. Without at least 3 key to
reconstruct the master key, Vault will remain permanently sealed!
```

## Unsealing Vault

1. Unseal Vault (requires 3 of 5 unseal keys):

```bash
vault operator unseal <unseal-key-1>
vault operator unseal <unseal-key-2>
vault operator unseal <unseal-key-3>
```

2 Verify Vault is unsealed:

```bash
  vault status
```

Expected output:

  ```bash
  Key             Value
  ---             -----
  Seal Type       shamir
  Initialized     true
  Sealed          false
  Total Shares    5
  Threshold       3
  Version         1.15.6
  Build Date      2025-06-06T15:40:01Z
  Storage Type    file
  Cluster Name    vault-cluster-xxxxx
  Cluster ID      xxxxx
  HA Enabled      false
  ```

## Storing Secrets

1 Enable the KV secrets engine:

  ```bash
  vault secrets enable -version=2 kv
```

2 Store GitHub configuration:

```bash
# First, read and store the private key content
PRIVATE_KEY=$(cat /path/to/your/private-key.pem)

# Then store the GitHub configuration with the private key content
vault kv put kv/ansible/github \
  app_id="your_app_id" \
  installation_id="your_installation_id" \
  private_key="$PRIVATE_KEY" \
  api_base_url="https://github.<yourdomain>.com/api/v3"
```

> **Note:** Store the private key content directly in Vault instead of the file path. This is more secure as it keeps the private key content encrypted at rest.

3 Store API configuration:

```bash
vault kv put kv/ansible/api \
  port="8080" \
  worker_count="4" \
  retention_hours="24" \
  temp_patterns="*_site.yml,*_hosts" \
  rate_limit="10"
```

4 Store SSH key:

```bash
vault kv put kv/ansible/ssh-key \
  private_key=@/path/to/private_key.pem
```

5 Store multiple values in a single path:

```bash
vault kv put kv/ansible/config \
  database_url="postgresql://user:pass@localhost:5432/db" \
  redis_url="redis://localhost:6379" \
  api_key="your-api-key" \
  webhook_secret="your-webhook-secret"
```

6 Store JSON data:

```bash
vault kv put kv/ansible/json-config \
  config='{"database":{"host":"localhost","port":5432},"redis":{"host":"localhost","port":6379}}'
```

## Retrieving Secrets

1 Get GitHub configuration:

  ```bash
  vault kv get kv/ansible/github
  ```

2 Get API configuration:

  ```bash
  vault kv get kv/ansible/api
  ```
error="VAULT_ROLE_ID and VAULT_SECRET_ID must be set" component=server-builder
3 Get SSH key:

  ```bash
  vault kv get kv/ansible/ssh-key
  ```

4 Get specific field from a secret:

  ```bash
  vault kv get -field=app_id kv/ansible/github
  ```

5 Get JSON output:

```bash
vault kv get -format=json kv/ansible/config
```

6 List all secrets in a path:

```bash
vault kv list kv/ansible
```

## Environment Variables

1. Set Vault address:

```bash
export VAULT_ADDR='http://127.0.0.1:8200'
```

2 Set Vault token:

  ```bash
  export VAULT_TOKEN='your-token'
```

3 Set Vault namespace (if using namespaces):

  ```bash
  export VAULT_NAMESPACE='your-namespace'
  ```

4 Add to your shell profile:

  ```bash
  echo 'export VAULT_ADDR="http://127.0.0.1:8200"' >> ~/.bashrc
  echo 'export VAULT_TOKEN="your-token"' >> ~/.bashrc
  source ~/.bashrc
  ```

## Role-Based Access Control (RBAC)

1. Enable userpass auth method:

```bash
vault auth enable userpass
```

2 Create a policy:

  ```bash
  vault policy write ansible-policy -<<EOF
  path "kv/ansible/*" {
    capabilities = ["read", "list"]
  }
  EOF
```

3 Create a user:

  ```bash
  vault write auth/userpass/users/ansible-user \
    password="your-password" \
    policies="ansible-policy"
  ```

4 Login with the user:

  ```bash
  vault login -method=userpass username=ansible-user
  ```

## AppRole Authentication Setup

The Ansible API uses AppRole authentication to securely access Vault secrets. This method is more suitable for applications than userpass authentication.

### 1. Enable the AppRole Auth Method

```bash
vault auth enable approle
```

### 2. Create a Policy for the Application

Create a file named `ansible-policy.hcl` with the following content:

```hcl
path "secret/data/ansible/*" {
  capabilities = ["read", "list"]
}
```

Upload the policy to Vault:

```bash
vault policy write ansible-policy ansible-policy.hcl
```

### 3. Create an AppRole and Attach the Policy

```bash
vault write auth/approle/role/ansible-role token_policies="ansible-policy"
```

### 4. Get the Role ID

```bash
vault read -field=role_id auth/approle/role/ansible-role/role-id
```

Copy the output and set it as your `VAULT_ROLE_ID`.

### 5. Generate a Secret ID

```bash
vault write -f -field=secret_id auth/approle/role/ansible-role/secret-id
```

Copy the output and set it as your `VAULT_SECRET_ID`.

### 6. Set the Environment Variables

**For Bash:**
```bash
export VAULT_ROLE_ID="your-role-id-here"
export VAULT_SECRET_ID="your-secret-id-here"
```

**For PowerShell:**
```powershell
$env:VAULT_ROLE_ID="your-role-id-here"
$env:VAULT_SECRET_ID="your-secret-id-here"
```

**For persistent storage, add to your shell profile:**
```bash
echo 'export VAULT_ROLE_ID="your-role-id-here"' >> ~/.bashrc
echo 'export VAULT_SECRET_ID="your-secret-id-here"' >> ~/.bashrc
source ~/.bashrc
```

### 7. Verify AppRole Authentication

Test that your application can authenticate and access secrets:

```bash
# The application should now be able to start without the warning:
# "Failed to initialize Vault client, falling back to environment variables"
```

### Summary Table

| Step | Command/Action | Description |
|------|---------------|-------------|
| 1    | `vault auth enable approle` | Enable AppRole auth method |
| 2    | `vault policy write ansible-policy ansible-policy.hcl` | Create and upload policy |
| 3    | `vault write auth/approle/role/ansible-role token_policies="ansible-policy"` | Create AppRole |
| 4    | `vault read -field=role_id auth/approle/role/ansible-role/role-id` | Get Role ID |
| 5    | `vault write -f -field=secret_id auth/approle/role/ansible-role/secret-id` | Get Secret ID |
| 6    | Set env vars | Set `VAULT_ROLE_ID` and `VAULT_SECRET_ID` |
| 7    | Run app | App authenticates to Vault |

### Security Notes

- **Rotate Secret IDs regularly** by generating new ones and updating your environment variables
- **Keep Role IDs and Secret IDs secure** - never commit them to source control
- **Use different AppRoles** for different applications or environments
- **Monitor AppRole usage** through Vault audit logs

## Backup and Restore

1. Backup Vault data:

```bash
sudo tar -czf vault-backup.tar.gz /etc/vault.d/data
```

2 Backup Vault configuration:

  ```bash
  sudo tar -czf vault-config-backup.tar.gz /etc/vault.d/vault.hcl
  ```

3 Restore Vault data:

  ```bash
  sudo tar -xzf vault-backup.tar.gz -C /
  sudo chown -R vault:vault /etc/vault.d/data
  ```

4 Restore Vault configuration:

  ```bash
  sudo tar -xzf vault-config-backup.tar.gz -C /
  sudo chown vault:vault /etc/vault.d/vault.hcl
```

## Monitoring and Logging

1. Enable audit logging in vault.hcl:

```hcl
audit "file" {
  path = "/etc/vault.d/audit.log"
  type = "file"
}
```

2 View audit logs:

  ```bash
  sudo tail -f /etc/vault.d/audit.log
  ```

3 Monitor Vault metrics:

  ```bash
  vault operator metrics
  ```

4 Check Vault health:

  ```bash
  curl -s http://127.0.0.1:8200/v1/sys/health | jq
  ```

## Troubleshooting

### Common Issues

1. **Vault won't start**
   - Check permissions: `sudo chown -R vault:vault /etc/vault.d`
   - Check logs: `sudo journalctl -u vault`
   - Verify configuration: `vault read sys/config/state`

2. **Can't unseal Vault**
   - Verify Vault is running: `vault status`
   - Check if you're using the correct unseal keys
   - Ensure you're using the correct Vault address

3. **Can't access secrets**
   - Verify Vault is unsealed: `vault status`
   - Check if you have the correct permissions
   - Verify the KV secrets engine is enabled
   - Check your token permissions: `vault token lookup`

### Useful Commands

1 Check Vault status:

  ```bash
  vault status
  ```

2 View Vault logs:

  ```bash
  sudo journalctl -u vault -f
  ```

3 Check Vault configuration:

  ```bash
  vault read sys/config/state
  ```

4 List enabled secrets engines:

  ```bash
  vault secrets list
  ```

5 Check token information:

  ```bash
  vault token lookup
  ```

6 List auth methods:

  ```bash
  vault auth list
  ```

7 Check seal status:

  ```bash
  vault status | grep Sealed
  ```

### Security Best Practices

1. Always store unseal keys and root token securely
2. Use environment variables for sensitive data
3. Regularly rotate secrets
4. Use the principle of least privilege
5. Enable audit logging
6. Use TLS in production
7. Regularly backup Vault data
8. Implement proper RBAC
9. Use namespaces for multi-tenancy
10. Monitor Vault metrics and logs
11. Implement secret rotation policies
12. Use Vault Agent for automatic authentication

### Vault Service Fails to Start: Permission Denied

If you see errors like:

```bash
vault.service: Failed to locate executable /usr/local/bin/vault: Permission denied
vault.service: Failed at step EXEC spawning /usr/local/bin/vault: Permission denied
vault.service: Main process exited, code=exited, status=203/EXEC
```

#### **Checklist to Resolve:**

1. **Check File Permissions**

   ```bash
   ls -l /usr/local/bin/vault
   ```

   Should be:

   ```text
   -rwxr-xr-x 1 root root ... /usr/local/bin/vault
   ```

   If not, fix with:

   ```bash
   sudo chmod 755 /usr/local/bin/vault
   sudo chown root:root /usr/local/bin/vault
   ```

2. **Check SELinux Status**

   ```bash
   getenforce
   ```

   If it returns `Enforcing`, SELinux may be blocking execution.  
   Temporarily set to permissive to test:

   ```bash
   sudo setenforce 0
   sudo systemctl restart vault
   sudo systemctl status vault
   ```

   If Vault starts, restore the correct context:

   ```bash
   sudo restorecon -v /usr/local/bin/vault
   sudo setenforce 1
   ```

   If you need to keep SELinux permissive (not recommended for production), set `SELINUX=permissive` in `/etc/selinux/config`.

3. **Check Filesystem Mount Options**

   ```bash
   mount | grep /usr/local
   ```

   If you see `noexec`, move the binary to `/usr/bin` and update your service file.

4. **Test as the Vault User**

   ```bash
   sudo -u vault /usr/local/bin/vault --version
   ```

   If you see "Permission denied", the problem is with user execution rights or SELinux.

5. **Check for Extended Attributes**

   ```bash
   lsattr /usr/local/bin/vault
   ```

   Remove any unusual attributes if present.

#### Vault Fails to Initialize with 'read-only file system' Error

If you see an error like:

```bash
failed to initialize barrier: failed to persist keyring: mkdir /etc/vault.d/data/core: read-only file system
```

when running Vault as a service, and your systemd service file contains `ProtectSystem=full`, this means Vault does not have write access to `/etc/vault.d` due to systemd's filesystem protection.

**Solution:**

1. Edit your `/etc/systemd/system/vault.service` file.
2. In the `[Service]` section, add:

   ```ini
   ReadWritePaths=/etc/vault.d
   ```

   so it looks like:

   ```ini
   [Service]
   User=vault
   Group=vault
   ProtectSystem=full
   ReadWritePaths=/etc/vault.d
   ...
   ```

3. Save and exit the editor.
4. Reload systemd and restart Vault:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl restart vault
   sudo systemctl status vault
   ```

This will allow Vault to write to `/etc/vault.d` while keeping the rest of the system protected.

#### 403 Error or 'permission denied' When Accessing kv/* Paths

If you see errors like:

```bash
Error making API request.

URL: GET http://127.0.0.1:8200/v1/sys/internal/ui/mounts/kv/ansible/api
Code: 403. Errors:

* preflight capability check returned 403, please ensure client's policies grant access to path "kv/ansible/api/"
```

or

```bash
* permission denied
```

and running `vault secrets list` does **not** show a `kv/` path, it means the KV secrets engine is not enabled at the expected path.

**Solution:**

1. Enable the KV secrets engine at the `kv/` path:

   ```bash
   vault secrets enable -version=2 -path=kv kv
   ```

   You should see:

   ```text
   Success! Enabled the kv secrets engine at: kv/
   ```

2. Retry your command (e.g., `vault kv put kv/ansible/api ...`).

This will allow you to store and retrieve secrets at the `kv/` path as expected.

## Structured Logging Configuration

The Ansible API uses structured logging with zerolog for better debugging and observability.

### Log Levels

Set the log level using the `LOG_LEVEL` environment variable:

```bash
export LOG_LEVEL=debug    # Most verbose
export LOG_LEVEL=info     # Default level
export LOG_LEVEL=warn     # Warnings and errors only
export LOG_LEVEL=error    # Errors only
```

### Log Format

Logs are output in JSON format for easy parsing:

```json
{
  "level": "info",
  "time": "2025-06-20T13:49:43-04:00",
  "component": "server",
  "job_id": "job-1234567890",
  "repository": "owner/repo",
  "playbook": "site.yml",
  "duration": "2.5s",
  "message": "Job processing completed"
}
```

### Key Log Fields

| Field | Description | Example |
|-------|-------------|---------|
| `component` | Component generating the log | `server`, `vault`, `processor` |
| `job_id` | Unique job identifier | `job-1234567890` |
| `request_id` | Unique request identifier | `req-1234567890` |
| `endpoint` | API endpoint being called | `/api/playbook/run` |
| `method` | HTTP method | `POST`, `GET` |
| `remote_addr` | Client IP address | `192.168.1.100` |
| `duration` | Operation duration | `2.5s` |
| `path` | Vault secret path | `ansible/github` |
| `repository` | Git repository URL | `owner/repo` |
| `playbook` | Ansible playbook path | `site.yml` |
| `target_hosts` | Target hosts for playbook | `web_servers` |

### Debugging Common Issues

#### Vault Authentication Issues
```bash
# Look for these log entries:
{"level":"error","component":"vault","error":"VAULT_ROLE_ID and VAULT_SECRET_ID must be set"}
{"level":"error","component":"vault","error":"Failed to authenticate with Vault"}
```

#### Job Processing Issues
```bash
# Look for these log entries:
{"level":"error","component":"processor","job_id":"job-123","error":"Failed to clone repository"}
{"level":"error","component":"processor","job_id":"job-123","error":"Ansible playbook execution failed"}
```

#### API Request Issues
```bash
# Look for these log entries:
{"level":"error","component":"server","request_id":"req-123","error":"Invalid request body"}
{"level":"warn","component":"server","request_id":"req-123","message":"Rate limit exceeded"}
```

### Log Filtering Examples

#### Filter by Job ID
```bash
# Show all logs for a specific job
grep "job-1234567890" app.log
```

#### Filter by Component
```bash
# Show only Vault-related logs
grep '"component":"vault"' app.log
```

#### Filter by Error Level
```bash
# Show only error logs
grep '"level":"error"' app.log
```

#### Filter by Duration (slow requests)
```bash
# Show requests taking longer than 5 seconds
grep '"duration":"[5-9]\|1[0-9]"' app.log
```

### Log Aggregation

For production environments, consider:

1. **Log Shipping**: Send logs to a centralized logging system (ELK Stack, Splunk, etc.)
2. **Log Rotation**: Use logrotate to manage log file sizes
3. **Metrics Extraction**: Parse logs to extract metrics for monitoring
4. **Alerting**: Set up alerts based on error patterns

Example logrotate configuration:
```
/var/log/ansible-api/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 644 ansible-api ansible-api
}
```

## Configuration Precedence

The application loads configuration in the following order:

1. **Vault** (if available)
2. **Environment variables**
3. **Built-in defaults**

If a key is missing in Vault, the environment variable is used. If both are missing, the default is used.

## Additional Resources

- [Official Vault Documentation](https://www.vaultproject.io/docs)
- [Vault GitHub Repository](https://github.com/hashicorp/vault)
- [Vault Security Best Practices](https://learn.hashicorp.com/tutorials/vault/security-best-practices)
- [Vault Architecture](https://www.vaultproject.io/docs/internals/architecture)
- [Vault API Documentation](https://www.vaultproject.io/api-docs)
- [Vault CLI Commands](https://www.vaultproject.io/docs/commands)
