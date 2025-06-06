# Vault Setup and Usage Guide

## Table of Contents
1. [Installation](#installation)
2. [Initial Setup](#initial-setup)
3. [Configuration](#configuration)
4. [Starting Vault](#starting-vault)
5. [Initializing Vault](#initializing-vault)
6. [Unsealing Vault](#unsealing-vault)
7. [Storing Secrets](#storing-secrets)
8. [Retrieving Secrets](#retrieving-secrets)
9. [Troubleshooting](#troubleshooting)
10. [Environment Variables](#environment-variables)
11. [Role-Based Access Control (RBAC)](#role-based-access-control-rbac)
12. [Backup and Restore](#backup-and-restore)
13. [Monitoring and Logging](#monitoring-and-logging)

## Installation

1. Download Vault:
```bash
curl -L -o vault_1.15.6_linux_amd64.zip https://releases.hashicorp.com/vault/1.15.6/vault_1.15.6_linux_amd64.zip
```

2. Unzip and install:
```bash
unzip vault_1.15.6_linux_amd64.zip
sudo mv vault /usr/local/bin/
```

3. Create Vault user and directories:
```bash
sudo useradd --system --home /etc/vault.d --shell /bin/false vault
sudo mkdir -p /etc/vault.d
sudo chown -R vault:vault /etc/vault.d
```

4. Verify installation:
```bash
vault version
```

## Initial Setup

1. Create Vault configuration file:
```bash
sudo nano /etc/vault.d/vault.hcl
```

2. Add the following configuration:
```hcl
storage "file" {
  path = "/etc/vault.d/data"
}

listener "tcp" {
  address     = "127.0.0.1:8200"
  tls_disable = 1
}

api_addr = "http://127.0.0.1:8200"
cluster_addr = "https://127.0.0.1:8201"

ui = true
disable_mlock = true

# Enable audit logging
audit "file" {
  path = "/etc/vault.d/audit.log"
  type = "file"
}
```

3. Create data directory and set permissions:
```bash
sudo mkdir -p /etc/vault.d/data
sudo chown -R vault:vault /etc/vault.d/data
sudo chmod 700 /etc/vault.d/data
```

## Configuration

1. Create systemd service file:
```bash
sudo nano /etc/systemd/system/vault.service
```

2. Add the following configuration:
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
RestartSec=5
TimeoutStopSec=30
StartLimitBurst=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

## Starting Vault

1. Reload systemd:
```bash
sudo systemctl daemon-reload
```

2. Start Vault:
```bash
sudo systemctl start vault
```

3. Enable Vault to start on boot:
```bash
sudo systemctl enable vault
```

4. Check status:
```bash
sudo systemctl status vault
```

5. Verify Vault is running:
```bash
vault status
```

## Initializing Vault

1. Set Vault address:
```bash
export VAULT_ADDR='http://127.0.0.1:8200'
```

2. Initialize Vault:
```bash
vault operator init
```

3. Save the output securely. You'll receive:
   - 5 Unseal Keys
   - 1 Initial Root Token

Example output:
```
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

2. Verify Vault is unsealed:
```bash
vault status
```

Expected output:
```
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

1. Enable the KV secrets engine:
```bash
vault secrets enable -version=2 kv
```

2. Store GitHub configuration:
```bash
vault kv put kv/ansible/github \
  app_id="your_app_id" \
  installation_id="your_installation_id" \
  private_key_path="/path/to/private/key"
```

3. Store API configuration:
```bash
vault kv put kv/ansible/api \
  port="8080" \
  worker_count="4" \
  retention_hours="24" \
  temp_patterns="*_site.yml,*_hosts" \
  rate_limit="10"
```

4. Store SSH key:
```bash
vault kv put kv/ansible/ssh-key \
  private_key=@/path/to/private_key.pem
```

5. Store multiple values in a single path:
```bash
vault kv put kv/ansible/config \
  database_url="postgresql://user:pass@localhost:5432/db" \
  redis_url="redis://localhost:6379" \
  api_key="your-api-key" \
  webhook_secret="your-webhook-secret"
```

6. Store JSON data:
```bash
vault kv put kv/ansible/json-config \
  config='{"database":{"host":"localhost","port":5432},"redis":{"host":"localhost","port":6379}}'
```

## Retrieving Secrets

1. Get GitHub configuration:
```bash
vault kv get kv/ansible/github
```

2. Get API configuration:
```bash
vault kv get kv/ansible/api
```

3. Get SSH key:
```bash
vault kv get kv/ansible/ssh-key
```

4. Get specific field from a secret:
```bash
vault kv get -field=app_id kv/ansible/github
```

5. Get JSON output:
```bash
vault kv get -format=json kv/ansible/config
```

6. List all secrets in a path:
```bash
vault kv list kv/ansible
```

## Environment Variables

1. Set Vault address:
```bash
export VAULT_ADDR='http://127.0.0.1:8200'
```

2. Set Vault token:
```bash
export VAULT_TOKEN='your-token'
```

3. Set Vault namespace (if using namespaces):
```bash
export VAULT_NAMESPACE='your-namespace'
```

4. Add to your shell profile:
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

2. Create a policy:
```bash
vault policy write ansible-policy -<<EOF
path "kv/ansible/*" {
  capabilities = ["read", "list"]
}
EOF
```

3. Create a user:
```bash
vault write auth/userpass/users/ansible-user \
  password="your-password" \
  policies="ansible-policy"
```

4. Login with the user:
```bash
vault login -method=userpass username=ansible-user
```

## Backup and Restore

1. Backup Vault data:
```bash
sudo tar -czf vault-backup.tar.gz /etc/vault.d/data
```

2. Backup Vault configuration:
```bash
sudo tar -czf vault-config-backup.tar.gz /etc/vault.d/vault.hcl
```

3. Restore Vault data:
```bash
sudo tar -xzf vault-backup.tar.gz -C /
sudo chown -R vault:vault /etc/vault.d/data
```

4. Restore Vault configuration:
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

2. View audit logs:
```bash
sudo tail -f /etc/vault.d/audit.log
```

3. Monitor Vault metrics:
```bash
vault operator metrics
```

4. Check Vault health:
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

1. Check Vault status:
```bash
vault status
```

2. View Vault logs:
```bash
sudo journalctl -u vault -f
```

3. Check Vault configuration:
```bash
vault read sys/config/state
```

4. List enabled secrets engines:
```bash
vault secrets list
```

5. Check token information:
```bash
vault token lookup
```

6. List auth methods:
```bash
vault auth list
```

7. Check seal status:
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

## Additional Resources

- [Official Vault Documentation](https://www.vaultproject.io/docs)
- [Vault GitHub Repository](https://github.com/hashicorp/vault)
- [Vault Security Best Practices](https://learn.hashicorp.com/tutorials/vault/security-best-practices)
- [Vault Architecture](https://www.vaultproject.io/docs/internals/architecture)
- [Vault API Documentation](https://www.vaultproject.io/api-docs)
- [Vault CLI Commands](https://www.vaultproject.io/docs/commands) 