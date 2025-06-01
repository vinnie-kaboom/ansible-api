#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "Testing Ansible API..."

# Start the API server in the background
echo "Starting API server..."
go run main.go &
API_PID=$!

# Wait for the server to start
sleep 2

# Test 1: Health check
echo -e "\n${GREEN}Test 1: Health Check${NC}"
curl -s http://localhost:8080/health
echo -e "\n"

# Test 2: Run the test playbook
echo -e "${GREEN}Test 2: Running Test Playbook${NC}"
curl -s -X POST http://localhost:8080/api/v1/playbooks/run \
  -H "Content-Type: application/json" \
  -d '{
    "playbook": "test_playbook.yml",
    "repo_url": "https://github.com/vinnie-kaboom/ansible-repo.git",
    "branch": "main"
  }' | jq .

# Wait for the playbook to complete
sleep 5

# Test 3: Check the created files
echo -e "\n${GREEN}Test 3: Verifying Created Files${NC}"
echo "Contents of /tmp/ansible-test:"
ls -la /tmp/ansible-test

echo -e "\nContents of test.txt:"
cat /tmp/ansible-test/test.txt

# Cleanup
echo -e "\n${GREEN}Cleaning up...${NC}"
rm -rf /tmp/ansible-test
kill $API_PID

echo -e "\n${GREEN}Test completed!${NC}" 