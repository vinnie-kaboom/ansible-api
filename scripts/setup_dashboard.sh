#!/bin/bash

# Create required directories
sudo mkdir -p /etc/ansible/playbooks/{logs,scripts,templates,playbooks,inventory}

# Copy dashboard files
sudo cp scripts/log_dashboard.py /etc/ansible/playbooks/scripts/
sudo cp templates/dashboard.html /etc/ansible/playbooks/templates/

# Create initial repos.json
echo "[]" | sudo tee /etc/ansible/playbooks/repos.json

# Set permissions
sudo chown -R $USER:$USER /etc/ansible/playbooks

# Install required packages
pip3 install flask

# Make the script executable
sudo chmod +x /etc/ansible/playbooks/scripts/log_dashboard.py

echo "Setup complete! You can now run the dashboard with:"
echo "cd /etc/ansible/playbooks && python3 scripts/log_dashboard.py" 