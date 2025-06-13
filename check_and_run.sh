#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Starting validation and execution workflow...${NC}"

# Check Vault status
echo -e "\n${YELLOW}Checking Vault status...${NC}"
if ! vault status > /dev/null 2>&1; then
    echo -e "${RED}Error: Vault is not running or not accessible${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Vault is running${NC}"

# Check Vault configuration
echo -e "\n${YELLOW}Checking Vault configuration...${NC}"
if ! vault kv get kv/ansible/github > /dev/null 2>&1; then
    echo -e "${RED}Error: GitHub configuration not found in Vault${NC}"
    exit 1
fi
if ! vault kv get kv/ansible/api > /dev/null 2>&1; then
    echo -e "${RED}Error: API configuration not found in Vault${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Vault configuration is valid${NC}"

# Check API status
echo -e "\n${YELLOW}Checking API status...${NC}"
if ! curl -s http://localhost:8080/api/health > /dev/null; then
    echo -e "${RED}Error: API is not running or not accessible${NC}"
    exit 1
fi
echo -e "${GREEN}✓ API is running${NC}"

# Execute playbook
echo -e "\n${YELLOW}Executing playbook...${NC}"
response=$(curl -s -X POST http://localhost:8080/api/playbook/run \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://github.com/vinnie-kaboom/ansible-repo.git",
    "playbook_path": "playbooks/site.yml",
    "inventory": {
      "webservers": {
        "ansible_host": "localhost"
      }
    },
    "environment": {
      "ANSIBLE_HOST_KEY_CHECKING": "False"
    }
  }')

# Check if the request was successful
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Playbook execution request sent successfully${NC}"
    echo -e "\nResponse:"
    echo "$response"
else
    echo -e "${RED}Error: Failed to execute playbook${NC}"
    exit 1
fi

echo -e "\n${GREEN}Workflow completed successfully!${NC}" 