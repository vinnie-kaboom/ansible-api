# GAD Infrastructure Ansible

Enterprise-ready Ansible automation for GAD infrastructure management, built following Ansible best practices with role-based organization and comprehensive variable management.

## 📁 Project Structure

```bash
gad-infra-ansible/
├── inventory/                  # Inventory management
│   ├── hosts.yaml             # Main inventory file (YAML format)
│   ├── group_vars/            # Group-specific variables
│   │   ├── all.yml            # Variables for all hosts
│   │   ├── webservers.yml     # Webserver-specific variables
│   │   └── windows_servers.yml # Windows server variables
│   └── host_vars/             # Host-specific variables
├── playbooks/                 # Organized playbooks
│   ├── site.yml              # Main orchestration playbook
│   ├── webservers/           # Webserver management
│   │   ├── deploy.yml        # Deploy nginx webservers
│   │   └── uninstall.yml     # Uninstall nginx
│   ├── common/               # Common system tasks
│   │   └── system_setup.yml  # Basic system configuration
│   ├── database/             # Database management (future)
│   ├── network/              # Network device configuration (future)
│   └── security/             # Security hardening (future)
├── roles/                    # Reusable roles
│   ├── webserver/            # Nginx webserver role
│   │   ├── defaults/main.yml # Default variables
│   │   ├── tasks/main.yml    # Main tasks
│   │   ├── handlers/main.yml # Service handlers
│   │   ├── templates/        # Jinja2 templates
│   │   │   ├── nginx.conf.j2 # Main nginx config
│   │   │   ├── vhost.conf.j2 # Virtual host config
│   │   │   └── index.html.j2 # Default web page
│   │   └── meta/main.yml     # Role metadata
│   └── common_all/           # Common role for all systems
└── global_vars/              # Global variables
```

## 🚀 Quick Start

### Prerequisites

- Ansible 2.9+
- Python 3.6+
- SSH access to target hosts
- Vault authentication for sensitive variables

### Basic Usage

1. **Deploy all infrastructure:**

   ```bash
   ansible-playbook -i inventory/hosts.yaml playbooks/site.yml
   ```

2. **Deploy only webservers:**

   ```bash
   ansible-playbook -i inventory/hosts.yaml playbooks/webservers/deploy.yml
   ```

3. **Deploy with specific tags:**

   ```bash
   ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --tags webserver,nginx
   ```

4. **Check mode (dry run):**

   ```bash
   ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --check
   ```

## 📋 Available Playbooks

### Main Playbook

- `playbooks/site.yml` - Main orchestration playbook
- `playbooks/webservers/deploy.yml` - Deploy nginx webservers
- `playbooks/webservers/uninstall.yml` - Remove nginx installations
- `playbooks/common/system_setup.yml` - Basic system configuration

### Role-based Deployment

- **webserver role**: Complete nginx installation and configuration
- **common_all role**: Basic system setup for all hosts

## 🔧 Configuration

### Inventory Management

- **Main inventory**: `inventory/hosts.yaml` (YAML format)
- **Group variables**: `inventory/group_vars/`
- **Host variables**: `inventory/host_vars/`

### Variable Hierarchy (highest to lowest precedence)

1. Host variables (`host_vars/`)
2. Group variables (`group_vars/`)
3. Role variables (`roles/*/vars/`)
4. Global variables (`global_vars/`)
5. Role defaults (`roles/*/defaults/`)

### Key Variables

#### Webserver Configuration

```yaml
# inventory/group_vars/webservers.yml
nginx_user: nginx
nginx_port: 80
nginx_document_root: /var/www/html
nginx_server_tokens: "off"
ssl_enabled: false
```

#### Global Configuration

```yaml
# inventory/group_vars/all.yml
ansible_python_interpreter: python3
system_timezone: "America/New_York"
common_packages:
  - curl
  - wget
  - vim
  - htop
```

## 🏷️ Tags

Use tags to run specific parts of playbooks:

- `common` - Common system configuration
- `webserver` - Webserver-related tasks
- `nginx` - Nginx-specific tasks
- `packages` - Package installation
- `config` - Configuration management
- `firewall` - Firewall configuration
- `test` - Testing and verification

### Examples

```bash
# Install packages only
ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --tags packages

# Skip testing tasks
ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --skip-tags test

# Run nginx configuration only
ansible-playbook -i inventory/hosts.yaml playbooks/webservers/deploy.yml --tags nginx,config
```

## 🛡️ Security

### Vault Integration

- Sensitive variables stored in Vault
- Reference vault variables using: `"{{ vault_variable_name }}"`
- Maintain current Vault authentication method

### Best Practices

- Use least privilege access
- Enable firewall by default
- Disable unnecessary services
- Regular security updates

## 🔍 Testing and Validation

### Syntax Checking

```bash
# Check playbook syntax
ansible-playbook --syntax-check playbooks/site.yml

# Check inventory
ansible-inventory -i inventory/hosts.yaml --list
```

### Dry Run

```bash
# Test without making changes
ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --check --diff
```

### Health Checks

Built-in health checks verify:

- Service status
- HTTP connectivity
- Configuration validity
- Firewall rules

## 📊 Migration from Original Structure

### What Changed

- **Inventory**: Converted from INI to YAML format
- **Playbooks**: Split into role-based structure
- **Templates**: Moved to role-specific templates
- **Variables**: Organized into proper hierarchy
- **Tasks**: Extracted into reusable roles

### Migration Benefits

1. **Scalability**: Easy to add new server types
2. **Reusability**: Roles can be shared across playbooks
3. **Maintainability**: Clear separation of concerns
4. **Enterprise Ready**: Follows Ansible best practices
5. **Team Collaboration**: Clear organization for multiple contributors

## 🔄 Integration with Ansible API

### Update API Configuration

The Go application should be updated to use:

- New inventory path: `gad-infra-ansible/inventory/hosts.yaml`
- New playbook path: `gad-infra-ansible/playbooks/webservers/deploy.yml`
- Role-based structure for better modularity

### API Endpoints

- Deploy webservers: Use `playbooks/webservers/deploy.yml`
- Full deployment: Use `playbooks/site.yml`
- Drift detection: Compatible with new structure

## 🆘 Troubleshooting

### Common Issues

1. **Inventory not found**: Ensure path is correct
2. **Role not found**: Check roles directory structure
3. **Variables undefined**: Verify variable hierarchy
4. **Permission denied**: Check SSH keys and sudo access

### Debug Mode

```bash
# Enable verbose output
ansible-playbook -i inventory/hosts.yaml playbooks/site.yml -vvv

# Debug specific tasks
ansible-playbook -i inventory/hosts.yaml playbooks/site.yml --tags debug
```

## 📈 Future Enhancements

### Planned Features

- Database server roles
- Network device configuration
- Security hardening automation
- Windows server management
- Monitoring and alerting integration
- Backup automation

### Extensibility

- Add new roles in `roles/` directory
- Create new playbooks in appropriate subdirectories
- Extend variable hierarchy as needed
- Implement custom modules in `library/`

## 🤝 Contributing

1. Follow Ansible best practices
2. Use proper variable naming conventions
3. Include appropriate tags
4. Document new roles and playbooks
5. Test changes before committing

---

**Maintained by**: GAD Infrastructure Team  
**Version**: 1.0.0  
**Last Updated**: {{ ansible_date_time.date }}  
**Ansible Version**: 2.9+
