#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration file
CONFIG_FILE="repos.json"

# Function to display help
show_help() {
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  list              List all repositories"
    echo "  add <name> <url>  Add a new repository"
    echo "  remove <name>     Remove a repository"
    echo "  run <name>        Run a playbook from a repository"
    echo "  logs <name>       Show logs for a repository"
    echo ""
    echo "Options:"
    echo "  -h, --help        Show this help message"
    echo "  -b, --branch      Specify branch (default: main)"
    echo "  -p, --playbook    Specify playbook name"
    echo "  -e, --extra       Extra arguments for playbook"
    echo "  -r, --rollback    Enable rollback on failure"
}

# Function to check if jq is installed
check_jq() {
    if ! command -v jq &> /dev/null; then
        echo -e "${RED}Error: jq is not installed. Please install it first.${NC}"
        exit 1
    fi
}

# Function to initialize config file
init_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        echo "[]" > "$CONFIG_FILE"
    fi
}

# Function to list repositories
list_repos() {
    if [ ! -s "$CONFIG_FILE" ]; then
        echo -e "${YELLOW}No repositories configured.${NC}"
        return
    fi

    echo -e "${GREEN}Configured Repositories:${NC}"
    jq -r '.[] | "Name: \(.name)\nURL: \(.url)\nBranch: \(.branch // "main")\n---"' "$CONFIG_FILE"
}

# Function to add repository
add_repo() {
    local name=$1
    local url=$2
    local branch=${3:-main}

    # Check if repository already exists
    if jq -e ".[] | select(.name == \"$name\")" "$CONFIG_FILE" > /dev/null; then
        echo -e "${RED}Error: Repository '$name' already exists.${NC}"
        exit 1
    fi

    # Add new repository
    jq --arg name "$name" --arg url "$url" --arg branch "$branch" \
       '. += [{"name": $name, "url": $url, "branch": $branch}]' "$CONFIG_FILE" > tmp.json
    mv tmp.json "$CONFIG_FILE"

    echo -e "${GREEN}Repository '$name' added successfully.${NC}"
}

# Function to remove repository
remove_repo() {
    local name=$1

    # Check if repository exists
    if ! jq -e ".[] | select(.name == \"$name\")" "$CONFIG_FILE" > /dev/null; then
        echo -e "${RED}Error: Repository '$name' not found.${NC}"
        exit 1
    fi

    # Remove repository
    jq "del(.[] | select(.name == \"$name\"))" "$CONFIG_FILE" > tmp.json
    mv tmp.json "$CONFIG_FILE"

    echo -e "${GREEN}Repository '$name' removed successfully.${NC}"
}

# Function to run playbook
run_playbook() {
    local name=$1
    local playbook=$2
    local branch=$3
    local extra_args=$4
    local rollback=$5

    # Get repository URL
    local url=$(jq -r ".[] | select(.name == \"$name\") | .url" "$CONFIG_FILE")
    if [ -z "$url" ]; then
        echo -e "${RED}Error: Repository '$name' not found.${NC}"
        exit 1
    fi

    # Prepare curl command
    local curl_cmd="curl -X POST http://localhost:8080/run-playbook \
      -H \"Content-Type: application/json\" \
      -d '{
        \"playbook\": \"run_playbook\",
        \"inventory\": \"local\",
        \"extra_vars\": {
          \"repo_url\": \"$url\",
          \"repo_name\": \"$name\",
          \"playbook_name\": \"$playbook\",
          \"branch\": \"$branch\""

    # Add optional parameters
    if [ ! -z "$extra_args" ]; then
        curl_cmd="$curl_cmd, \"extra_args\": \"$extra_args\""
    fi
    if [ "$rollback" = "true" ]; then
        curl_cmd="$curl_cmd, \"rollback\": true"
    fi

    curl_cmd="$curl_cmd}}'"

    # Execute curl command
    eval "$curl_cmd"
}

# Function to show logs
show_logs() {
    local name=$1
    local log_dir="/etc/ansible/playbooks/logs/$name"

    if [ ! -d "$log_dir" ]; then
        echo -e "${YELLOW}No logs found for repository '$name'.${NC}"
        return
    fi

    echo -e "${GREEN}Logs for repository '$name':${NC}"
    ls -t "$log_dir"/*.log 2>/dev/null | while read -r log; do
        echo -e "\n${YELLOW}=== $(basename "$log") ===${NC}"
        cat "$log"
    done
}

# Main script
check_jq
init_config

case "$1" in
    list)
        list_repos
        ;;
    add)
        if [ -z "$2" ] || [ -z "$3" ]; then
            echo -e "${RED}Error: Repository name and URL are required.${NC}"
            show_help
            exit 1
        fi
        add_repo "$2" "$3" "$4"
        ;;
    remove)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Repository name is required.${NC}"
            show_help
            exit 1
        fi
        remove_repo "$2"
        ;;
    run)
        if [ -z "$2" ] || [ -z "$3" ]; then
            echo -e "${RED}Error: Repository name and playbook name are required.${NC}"
            show_help
            exit 1
        fi
        run_playbook "$2" "$3" "$4" "$5" "$6"
        ;;
    logs)
        if [ -z "$2" ]; then
            echo -e "${RED}Error: Repository name is required.${NC}"
            show_help
            exit 1
        fi
        show_logs "$2"
        ;;
    -h|--help)
        show_help
        ;;
    *)
        echo -e "${RED}Error: Unknown command.${NC}"
        show_help
        exit 1
        ;;
esac 