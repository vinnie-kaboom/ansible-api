# Setup Instructions

## Vault Development Setup

1. Start Vault in development mode:
```bash
vault server -dev
```
This will output a root token and unseal key. Save these somewhere secure.

2. In a new terminal, set the Vault address:
```bash
export VAULT_ADDR='http://127.0.0.1:8200'
```

3. Enable the KV secrets engine:
```bash
vault secrets enable -path=kv kv-v2
```
The KV-v2 secrets engine provides versioned key-value storage.

4. Create the AppRole auth method:
```bash
vault auth enable approle
```
AppRole is a secure way to introduce machines or applications to Vault.

5. Create a policy for the Ansible API (create a file named `ansible-policy.hcl`):
```hcl
path "kv/data/ansible/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "kv/metadata/ansible/*" {
  capabilities = ["list"]
}
```
This policy allows:
- Full access to secrets under `kv/data/ansible/*`
- Ability to list metadata for versioning

6. Write the policy to Vault:
```bash
vault policy write ansible-policy ansible-policy.hcl
```

7. Create an AppRole for the Ansible API:
```bash
vault write auth/approle/role/ansible-api \
    token_policies="ansible-policy" \
    token_ttl="1h" \
    token_max_ttl="4h" \
    bind_secret_id=true \
    secret_id_ttl="8760h" \
    secret_id_num_uses=0
```
This creates a role with:
- 1-hour token TTL
- 4-hour maximum token TTL
- Secret IDs that never expire
- Unlimited uses per Secret ID

8. Get the Role ID:
```bash
vault read auth/approle/role/ansible-api/role-id
```
Example output:
```
Key        Value
---        -----
role_id    xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

9. Generate a Secret ID:
```bash
vault write -f auth/approle/role/ansible-api/secret-id
```
Example output:
```
Key                   Value
---                   -----
secret_id             xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
secret_id_accessor    xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

10. Store the Role ID and Secret ID in your environment:
```bash
export VAULT_ROLE_ID='<role-id-from-step-8>'
export VAULT_SECRET_ID='<secret-id-from-step-9>'
```

11. Store a test secret:
```bash
vault kv put kv/ansible/github \
    app_id="test-app" \
    app_secret="test-secret" \
    private_key=@/path/to/private-key.pem
```

12. Verify the secret was stored:
```bash
vault kv get kv/ansible/github
```

## GitHub App Configuration

1. Create a new GitHub App in your GitHub account:
   - Go to Settings > Developer Settings > GitHub Apps
   - Click "New GitHub App"
   - Fill in the required fields:
     - Name: Ansible API
     - Homepage URL: http://localhost:9090
     - Webhook: Disable
     - Permissions:
       - Repository permissions:
         - Contents: Read & write (for accessing playbooks)
         - Metadata: Read-only (for repository info)
         - Pull requests: Read & write (for PR checks)
         - Workflows: Read & write (for workflow triggers)
     - Where can this GitHub App be installed?: Only on this account

2. After creating the app, you'll get:
   - App ID (numeric identifier)
   - Client ID (for OAuth)
   - Client Secret (for OAuth)
   - Private Key (for JWT generation)
   Save the private key as a .pem file

3. Store the GitHub App credentials in Vault:
```bash
vault kv put kv/ansible/github \
    app_id="<your-app-id>" \
    client_id="<your-client-id>" \
    client_secret="<your-client-secret>" \
    private_key=@/path/to/your/private-key.pem
```

4. Verify the GitHub App credentials:
```bash
vault kv get kv/ansible/github
```

## Important Notes

### Vault Security
- The Vault development server will show you the root token when you start it
- Never use the root token in production
- The KV secrets engine is mounted at `kv/`
- All secrets are versioned in KV-v2
- The AppRole credentials (Role ID and Secret ID) are used by the Ansible API to authenticate
- The policy allows access to secrets under `kv/data/ansible/*`

### GitHub App Security
- Keep your GitHub App private key secure and never commit it to version control
- The private key should be stored in Vault, not in the filesystem
- The GitHub App needs to be installed on the repositories where you want to use it
- App installation requires repository admin permissions

### Development vs Production
- This setup is for development only
- In production:
  - Use a proper Vault server with TLS
  - Enable audit logging
  - Use proper authentication methods
  - Set up auto-unsealing
  - Use proper backup strategies

## Environment Variables

Required environment variables for development:
```bash
# Vault Configuration
export VAULT_ADDR='http://127.0.0.1:8200'
export VAULT_ROLE_ID='<your-role-id>'
export VAULT_SECRET_ID='<your-secret-id>'

# Optional: For development only
export VAULT_SKIP_VERIFY=true  # Skip TLS verification in dev
```

## Testing the Setup

1. Verify Vault is running:
```bash
curl -s $VAULT_ADDR/v1/sys/health
```
Expected output: `{"initialized":true,"sealed":false,"standby":false,...}`

2. Verify GitHub App credentials:
```bash
vault kv get kv/ansible/github
```
Should show all stored GitHub App credentials

3. Test AppRole authentication:
```bash
vault write auth/approle/login role_id=$VAULT_ROLE_ID secret_id=$VAULT_SECRET_ID
```
Should return a token with the ansible-policy

4. Test secret access:
```bash
# First, get a token
TOKEN=$(vault write -field=token auth/approle/login role_id=$VAULT_ROLE_ID secret_id=$VAULT_SECRET_ID)

# Then use it to access secrets
curl -s -H "X-Vault-Token: $TOKEN" $VAULT_ADDR/v1/kv/data/ansible/github
``` 