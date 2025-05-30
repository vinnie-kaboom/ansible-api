#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting Ansible API setup...${NC}"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Go is not installed. Please install Go 1.16 or later.${NC}"
    exit 1
fi

# Check if Ansible is installed
if ! command -v ansible &> /dev/null; then
    echo -e "${RED}Ansible is not installed. Please install Ansible.${NC}"
    exit 1
fi

# Create necessary directories
echo "Creating directories..."
mkdir -p /etc/ansible/playbooks
mkdir -p /etc/ansible/playbooks/inventory
mkdir -p /etc/ansible/playbooks/roles

# Build the application
echo "Building the application..."
go build -o ansible-api cmd/api/main.go

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file..."
    cat > .env << EOL
ANSIBLE_PLAYBOOK_PATH=/etc/ansible/playbooks
ANSIBLE_REPO_URL=https://github.com/your-org/your-playbooks.git
PORT=8080
EOL
    echo -e "${GREEN}Created .env file. Please update ANSIBLE_REPO_URL with your repository URL.${NC}"
fi

# Create systemd service file
echo "Creating systemd service..."
sudo cat > /etc/systemd/system/ansible-api.service << EOL
[Unit]
Description=Ansible API Service
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$(pwd)
EnvironmentFile=$(pwd)/.env
ExecStart=$(pwd)/ansible-api
Restart=always

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd and enable service
echo "Enabling service..."
sudo systemctl daemon-reload
sudo systemctl enable ansible-api
sudo systemctl start ansible-api

# Test the setup
echo "Testing the setup..."
sleep 5  # Wait for service to start

# Test the health endpoint
if curl -s http://localhost:8080/health > /dev/null; then
    echo -e "${GREEN}Service is running!${NC}"
else
    echo -e "${RED}Service failed to start. Check logs with: journalctl -u ansible-api${NC}"
    exit 1
fi

# Run test playbook
echo "Running test playbook..."
curl -X POST http://localhost:8080/run-playbook \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "test",
    "inventory": "local",
    "extra_vars": {
      "playbook_base_path": "/etc/ansible/playbooks"
    }
  }'

echo -e "${GREEN}Setup complete!${NC}"
echo "You can now use the Ansible API at http://localhost:8080"
echo "Check the README.md for more information on how to use the API." 