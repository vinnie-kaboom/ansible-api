#!/bin/bash
# Idempotent Playbook Checkout Script
# Usage: ./scripts/checkout-playbooks.sh [repository_url] [target_directory] [branch]

set -euo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
DEFAULT_REPO="https://git.cce3.gpc/vincent-zamora/playbook-test.git"
DEFAULT_TARGET="./playbooks-checkout"
DEFAULT_BRANCH="main"

# Parse arguments
REPO_URL="${1:-$DEFAULT_REPO}"
TARGET_DIR="${2:-$DEFAULT_TARGET}"
BRANCH="${3:-$DEFAULT_BRANCH}"

echo -e "${BLUE}🔄 Idempotent Playbook Checkout${NC}"
echo -e "Repository: ${YELLOW}$REPO_URL${NC}"
echo -e "Target Directory: ${YELLOW}$TARGET_DIR${NC}"
echo -e "Branch: ${YELLOW}$BRANCH${NC}"
echo ""

# Function to log with timestamp
log() {
    echo -e "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Function to check if directory is a git repository
is_git_repo() {
    [ -d "$1/.git" ]
}

# Function to get current remote URL
get_remote_url() {
    if is_git_repo "$1"; then
        cd "$1" && git remote get-url origin 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# Function to get current branch
get_current_branch() {
    if is_git_repo "$1"; then
        cd "$1" && git branch --show-current 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# Function to check if working directory is clean
is_working_dir_clean() {
    if is_git_repo "$1"; then
        cd "$1" && [ -z "$(git status --porcelain)" ]
    else
        return 1
    fi
}

# Function to perform git operations safely
safe_git_operation() {
    local operation="$1"
    local dir="$2"
    
    case "$operation" in
        "clone")
            log "${GREEN}📥 Cloning repository...${NC}"
            git clone --branch "$BRANCH" "$REPO_URL" "$TARGET_DIR"
            ;;
        "pull")
            log "${GREEN}🔄 Updating existing repository...${NC}"
            cd "$dir"
            git fetch origin
            git checkout "$BRANCH"
            git pull origin "$BRANCH"
            ;;
        "reset")
            log "${YELLOW}⚠️  Resetting to clean state...${NC}"
            cd "$dir"
            git fetch origin
            git reset --hard "origin/$BRANCH"
            git clean -fd
            ;;
    esac
}

# Function to validate playbook structure
validate_playbook_structure() {
    local dir="$1"
    local errors=0
    
    log "${BLUE}🔍 Validating playbook structure...${NC}"
    
    # Check for essential directories and files
    local required_paths=(
        "gad-infra-ansible"
        "gad-infra-ansible/inventory"
        "gad-infra-ansible/playbooks"
        "gad-infra-ansible/roles"
        "gad-infra-ansible/inventory/hosts.yaml"
        "gad-infra-ansible/playbooks/site.yml"
    )
    
    for path in "${required_paths[@]}"; do
        if [ ! -e "$dir/$path" ]; then
            log "${RED}❌ Missing required path: $path${NC}"
            ((errors++))
        else
            log "${GREEN}✅ Found: $path${NC}"
        fi
    done
    
    # Check for specific playbooks
    local playbook_paths=(
        "gad-infra-ansible/playbooks/webservers/deploy.yml"
        "gad-infra-ansible/playbooks/webservers/uninstall.yml"
    )
    
    for playbook in "${playbook_paths[@]}"; do
        if [ -f "$dir/$playbook" ]; then
            log "${GREEN}✅ Found playbook: $playbook${NC}"
            # Validate YAML syntax
            if command -v ansible-playbook >/dev/null 2>&1; then
                if ansible-playbook --syntax-check "$dir/$playbook" >/dev/null 2>&1; then
                    log "${GREEN}✅ YAML syntax valid: $playbook${NC}"
                else
                    log "${RED}❌ YAML syntax error: $playbook${NC}"
                    ((errors++))
                fi
            fi
        else
            log "${YELLOW}⚠️  Optional playbook not found: $playbook${NC}"
        fi
    done
    
    # Check for roles
    if [ -d "$dir/gad-infra-ansible/roles/webserver" ]; then
        log "${GREEN}✅ Found webserver role${NC}"
        
        # Check role structure
        local role_dirs=("tasks" "handlers" "templates" "defaults" "meta")
        for role_dir in "${role_dirs[@]}"; do
            if [ -d "$dir/gad-infra-ansible/roles/webserver/$role_dir" ]; then
                log "${GREEN}✅ Role directory: webserver/$role_dir${NC}"
            else
                log "${YELLOW}⚠️  Missing role directory: webserver/$role_dir${NC}"
            fi
        done
    else
        log "${RED}❌ Missing webserver role${NC}"
        ((errors++))
    fi
    
    return $errors
}

# Function to create backup if needed
create_backup() {
    local dir="$1"
    local backup_dir="${dir}.backup.$(date +%Y%m%d_%H%M%S)"
    
    if [ -d "$dir" ]; then
        log "${YELLOW}📦 Creating backup: $backup_dir${NC}"
        cp -r "$dir" "$backup_dir"
        echo "$backup_dir"
    fi
}

# Function to cleanup old backups (keep last 5)
cleanup_old_backups() {
    local base_dir="$1"
    local backup_pattern="${base_dir}.backup.*"
    
    # Find and remove old backups, keeping only the 5 most recent
    if ls ${backup_pattern} >/dev/null 2>&1; then
        log "${BLUE}🧹 Cleaning up old backups...${NC}"
        ls -dt ${backup_pattern} | tail -n +6 | xargs rm -rf
    fi
}

# Main execution logic
main() {
    local backup_created=""
    local operation_needed=""
    
    # Check if target directory exists
    if [ -d "$TARGET_DIR" ]; then
        if is_git_repo "$TARGET_DIR"; then
            local current_url=$(get_remote_url "$TARGET_DIR")
            local current_branch=$(get_current_branch "$TARGET_DIR")
            
            log "${BLUE}📁 Found existing git repository${NC}"
            log "Current URL: ${current_url}"
            log "Current branch: ${current_branch}"
            
            # Check if it's the same repository
            if [ "$current_url" = "$REPO_URL" ]; then
                log "${GREEN}✅ Repository URLs match${NC}"
                
                # Check if on correct branch
                if [ "$current_branch" = "$BRANCH" ]; then
                    log "${GREEN}✅ On correct branch${NC}"
                    
                    # Check if working directory is clean
                    if is_working_dir_clean "$TARGET_DIR"; then
                        log "${GREEN}✅ Working directory is clean${NC}"
                        operation_needed="pull"
                    else
                        log "${YELLOW}⚠️  Working directory has uncommitted changes${NC}"
                        backup_created=$(create_backup "$TARGET_DIR")
                        operation_needed="reset"
                    fi
                else
                    log "${YELLOW}⚠️  On different branch: $current_branch${NC}"
                    if is_working_dir_clean "$TARGET_DIR"; then
                        operation_needed="pull"
                    else
                        backup_created=$(create_backup "$TARGET_DIR")
                        operation_needed="reset"
                    fi
                fi
            else
                log "${RED}❌ Different repository URL${NC}"
                backup_created=$(create_backup "$TARGET_DIR")
                rm -rf "$TARGET_DIR"
                operation_needed="clone"
            fi
        else
            log "${YELLOW}⚠️  Directory exists but is not a git repository${NC}"
            backup_created=$(create_backup "$TARGET_DIR")
            rm -rf "$TARGET_DIR"
            operation_needed="clone"
        fi
    else
        log "${BLUE}📁 Target directory does not exist${NC}"
        operation_needed="clone"
    fi
    
    # Perform the determined operation
    case "$operation_needed" in
        "clone")
            safe_git_operation "clone" "$TARGET_DIR"
            ;;
        "pull")
            safe_git_operation "pull" "$TARGET_DIR"
            ;;
        "reset")
            safe_git_operation "reset" "$TARGET_DIR"
            ;;
    esac
    
    # Validate the result
    if validate_playbook_structure "$TARGET_DIR"; then
        log "${GREEN}🎉 Playbook checkout completed successfully!${NC}"
        
        # Show useful information
        echo ""
        echo -e "${BLUE}📋 Checkout Summary:${NC}"
        echo -e "Directory: ${GREEN}$TARGET_DIR${NC}"
        echo -e "Repository: ${GREEN}$REPO_URL${NC}"
        echo -e "Branch: ${GREEN}$BRANCH${NC}"
        if [ -n "$backup_created" ]; then
            echo -e "Backup created: ${YELLOW}$backup_created${NC}"
        fi
        
        # Show available playbooks
        echo ""
        echo -e "${BLUE}📚 Available Playbooks:${NC}"
        if [ -f "$TARGET_DIR/gad-infra-ansible/playbooks/site.yml" ]; then
            echo -e "  ${GREEN}✅${NC} gad-infra-ansible/playbooks/site.yml (Main orchestration)"
        fi
        if [ -f "$TARGET_DIR/gad-infra-ansible/playbooks/webservers/deploy.yml" ]; then
            echo -e "  ${GREEN}✅${NC} gad-infra-ansible/playbooks/webservers/deploy.yml"
        fi
        if [ -f "$TARGET_DIR/gad-infra-ansible/playbooks/webservers/uninstall.yml" ]; then
            echo -e "  ${GREEN}✅${NC} gad-infra-ansible/playbooks/webservers/uninstall.yml"
        fi
        
        # Show example API calls
        echo ""
        echo -e "${BLUE}🚀 Example API Calls:${NC}"
        echo -e "${YELLOW}# Deploy webservers${NC}"
        echo "curl -X POST http://localhost:8080/api/execute \\"
        echo "  -H \"Content-Type: application/json\" \\"
        echo "  -d '{"
        echo "    \"repository_url\": \"$REPO_URL\","
        echo "    \"playbook_path\": \"gad-infra-ansible/playbooks/webservers/deploy.yml\","
        echo "    \"target_hosts\": \"GADHQMSCAP02.cce3.gpc\""
        echo "  }'"
        
        # Cleanup old backups
        cleanup_old_backups "$TARGET_DIR"
        
        exit 0
    else
        log "${RED}❌ Playbook structure validation failed${NC}"
        if [ -n "$backup_created" ]; then
            log "${YELLOW}💾 Backup available at: $backup_created${NC}"
        fi
        exit 1
    fi
}

# Show help if requested
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    echo "Idempotent Playbook Checkout Script"
    echo ""
    echo "Usage: $0 [repository_url] [target_directory] [branch]"
    echo ""
    echo "Arguments:"
    echo "  repository_url    Git repository URL (default: $DEFAULT_REPO)"
    echo "  target_directory  Local checkout directory (default: $DEFAULT_TARGET)"
    echo "  branch           Git branch to checkout (default: $DEFAULT_BRANCH)"
    echo ""
    echo "Features:"
    echo "  ✅ Idempotent - safe to run multiple times"
    echo "  🔄 Auto-updates existing checkouts"
    echo "  💾 Creates backups of modified directories"
    echo "  🔍 Validates playbook structure"
    echo "  🧹 Cleans up old backups"
    echo "  ⚡ Fast - only fetches changes when needed"
    echo ""
    echo "Examples:"
    echo "  $0"
    echo "  $0 https://github.com/user/ansible-playbooks.git"
    echo "  $0 https://github.com/user/ansible-playbooks.git ./my-playbooks feature-branch"
    exit 0
fi

# Run main function
main 