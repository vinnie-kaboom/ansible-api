# GAD Infra Ansible Migration Plan

## 🎯 Overview

Restructuring the current Ansible project to follow enterprise best practices with the `gad-infra-ansible` template structure.

## 📊 Current State Analysis

### Existing Structure

```bash
ansible-api/
├── inventory/hosts.ini          # Basic INI format inventory
├── playbooks/
│   ├── site.yml                 # Nginx installation/configuration
│   ├── uninstall-nginx-rhel.yml # RHEL-specific nginx removal
│   ├── cross-platform-example.yml # Cross-platform example
│   ├── uninstall-nginx.yml     # Generic nginx removal
│   └── templates/nginx.conf.j2  # Nginx configuration template
├── templates/nginx.conf.j2      # Duplicate nginx template
└── [Go API application files]
```

### Current Inventory Content

- `webservers` group with localhost
- `windows_servers` group (commented example)
- Basic connection and privilege escalation settings

## 🏗️ Target Structure Implementation

### Phase 1: Create New Directory Structure

```bash
mkdir -p gad-infra-ansible/{inventory/{group_vars,host_vars},playbooks/{webservers,database,network,security,common},roles/{common_all/{defaults,vars,tasks,handlers,meta,templates,files},webserver/{defaults,vars,tasks,handlers,meta,templates,files},database_server},global_vars,library}
```

### Phase 2: Migrate Inventory

**Convert hosts.ini to hosts.yaml:**

```yaml
# inventory/hosts.yaml
all:
  children:
    webservers:
      hosts:
        localhost:
          ansible_connection: local
          ansible_python_interpreter: /usr/bin/python3
        GADHQMSCAP02.cce3.gpc:
          ansible_host: GADHQMSCAP02.cce3.gpc
          ansible_user: "{{ vault_ansible_user }}"
          ansible_become_method: sudo
          ansible_become_flags: "--preserve-env"
    
    windows_servers:
      hosts:
        # win-server:
        #   ansible_host: 192.168.1.100
        #   ansible_user: Administrator
        #   ansible_connection: winrm
        #   ansible_winrm_transport: basic
        #   ansible_winrm_server_cert_validation: ignore
        #   ansible_become_method: runas

    database:
      hosts:
        # Add database servers here

    network:
      hosts:
        # Add network devices here
```

**Create group_vars:**

```yaml
# inventory/group_vars/webservers.yml
nginx_user: nginx
nginx_port: 80
nginx_document_root: /var/www/html
nginx_error_log: /var/log/nginx/error.log
nginx_access_log: /var/log/nginx/access.log

# inventory/group_vars/windows_servers.yml
ansible_winrm_transport: ntlm
ansible_winrm_server_cert_validation: ignore
ansible_become_method: runas

# inventory/group_vars/all.yml
ansible_python_interpreter: python3
ansible_host_key_checking: false
```

### Phase 3: Create Roles

**webserver role structure:**

```bash
roles/webserver/
├── defaults/main.yml    # Default nginx variables
├── vars/main.yml        # Role-specific variables
├── tasks/main.yml       # Main nginx installation tasks
├── handlers/main.yml    # Nginx service handlers
├── templates/
│   └── nginx.conf.j2    # Moved from playbooks/templates/
├── files/
│   └── index.html       # Default web page
└── meta/main.yml        # Role metadata
```

**common_all role structure:**

```bash
roles/common_all/
├── defaults/main.yml    # Common system defaults
├── tasks/main.yml       # Common system tasks
├── handlers/main.yml    # Common handlers
└── meta/main.yml        # Role metadata
```

### Phase 4: Restructure Playbooks

**Main site.yml:**

```yaml
# playbooks/site.yml
---
- import_playbook: common/system_setup.yml
- import_playbook: webservers/deploy.yml
- import_playbook: webservers/configure.yml
```

**Webserver playbooks:**

```yaml
# playbooks/webservers/deploy.yml
- name: Deploy webservers
  hosts: webservers
  become: yes
  roles:
    - common_all
    - webserver

# playbooks/webservers/configure.yml
- name: Configure webservers
  hosts: webservers
  become: yes
  tasks:
    - name: Include webserver configuration tasks
      include_role:
        name: webserver
        tasks_from: configure
```

### Phase 5: Migration Mapping

| Current File | New Location | Action |
|--------------|--------------|---------|
| `inventory/hosts.ini` | `inventory/hosts.yaml` | Convert format + enhance |
| `playbooks/site.yml` | `roles/webserver/tasks/main.yml` | Extract to role |
| `playbooks/templates/nginx.conf.j2` | `roles/webserver/templates/nginx.conf.j2` | Move |
| `templates/nginx.conf.j2` | Remove | Duplicate |
| `playbooks/uninstall-nginx*.yml` | `playbooks/webservers/uninstall.yml` | Consolidate |
| `playbooks/cross-platform-example.yml` | `playbooks/common/cross_platform.yml` | Move |

## 🚀 Implementation Steps

### Step 1: Backup Current Structure

```bash
cp -r ansible-api ansible-api-backup
```

### Step 2: Create Role-based Structure

- Extract nginx tasks from site.yml into webserver role
- Create reusable components
- Implement proper variable hierarchy

### Step 3: Test Migration

- Verify inventory parsing: `ansible-inventory -i inventory/hosts.yaml --list`
- Test role syntax: `ansible-playbook --syntax-check playbooks/site.yml`
- Run in check mode: `ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --check`

### Step 4: Update API Integration

- Modify Go application to use new inventory path
- Update playbook references in drift detection
- Test API endpoints with new structure

## 🎯 Benefits of New Structure

1. **Scalability**: Easy to add new server types and roles
2. **Reusability**: Roles can be shared across playbooks
3. **Maintainability**: Clear separation of concerns
4. **Enterprise Ready**: Follows Ansible best practices
5. **Variable Management**: Proper hierarchy and scoping
6. **Team Collaboration**: Clear organization for multiple contributors

## 🔧 Configuration Management

### Variable Precedence (highest to lowest)

1. `host_vars/server.yml`
2. `inventory/hosts.yaml` host variables
3. `group_vars/group.yml`
4. `inventory/hosts.yaml` group variables
5. `roles/role/vars/main.yml`
6. `global_vars/`
7. `roles/role/defaults/main.yml`

### Vault Integration

- Sensitive variables in `group_vars/all/vault.yml`
- Reference vault variables in other files
- Maintain current Vault authentication method

This migration will transform your current project into a professional, enterprise-ready Ansible infrastructure management system!
