#!/bin/bash

# Nginx Uninstaller for RHEL 9
# This script completely removes nginx and all associated files/configurations

set -e

echo "🔄 Starting nginx uninstallation on RHEL 9..."
echo "========================================"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "❌ This script must be run as root (use sudo)"
        exit 1
    fi
}

# Function to stop nginx service
stop_nginx() {
    echo "🛑 Stopping nginx service..."
    if systemctl is-active --quiet nginx; then
        systemctl stop nginx
        systemctl disable nginx
        echo "✅ Nginx service stopped and disabled"
    else
        echo "ℹ️  Nginx service is not running"
    fi
}

# Function to remove nginx package
remove_package() {
    echo "📦 Removing nginx package..."
    if command_exists dnf; then
        dnf remove -y nginx
        dnf autoremove -y
        echo "✅ Nginx package removed"
    else
        echo "❌ DNF package manager not found"
        exit 1
    fi
}

# Function to remove configuration files
remove_configs() {
    echo "🗂️  Removing configuration files..."
    local dirs_to_remove=(
        "/etc/nginx"
        "/var/log/nginx"
        "/var/cache/nginx"
        "/var/lib/nginx"
        "/etc/logrotate.d/nginx"
        "/var/www/html"
    )
    
    for dir in "${dirs_to_remove[@]}"; do
        if [[ -d "$dir" ]] || [[ -f "$dir" ]]; then
            rm -rf "$dir"
            echo "✅ Removed $dir"
        fi
    done
    
    # Remove PID file if exists
    if [[ -f "/run/nginx.pid" ]]; then
        rm -f "/run/nginx.pid"
        echo "✅ Removed nginx PID file"
    fi
}

# Function to remove nginx user and group
remove_user_group() {
    echo "👤 Removing nginx user and group..."
    
    if id "nginx" &>/dev/null; then
        userdel -r nginx 2>/dev/null || userdel nginx 2>/dev/null
        echo "✅ Removed nginx user"
    fi
    
    if getent group nginx &>/dev/null; then
        groupdel nginx 2>/dev/null
        echo "✅ Removed nginx group"
    fi
}

# Function to close firewall ports
close_firewall_ports() {
    echo "🔥 Closing firewall ports..."
    
    if command_exists firewall-cmd && systemctl is-active --quiet firewalld; then
        firewall-cmd --permanent --remove-service=http 2>/dev/null || true
        firewall-cmd --permanent --remove-service=https 2>/dev/null || true
        firewall-cmd --reload
        echo "✅ Closed HTTP/HTTPS ports in firewall"
    else
        echo "ℹ️  Firewall not active or firewall-cmd not found"
    fi
}

# Function to find and remove remaining files
cleanup_remaining_files() {
    echo "🧹 Cleaning up remaining nginx files..."
    
    # Find and remove any remaining nginx files
    find /etc /var/log /var/cache /usr/share -name "*nginx*" -type f -delete 2>/dev/null || true
    find /etc /var/log /var/cache /usr/share -name "*nginx*" -type d -empty -delete 2>/dev/null || true
    
    echo "✅ Cleaned up remaining files"
}

# Function to clean package cache
clean_cache() {
    echo "🧽 Cleaning package cache..."
    dnf clean all
    echo "✅ Package cache cleaned"
}

# Function to verify removal
verify_removal() {
    echo "🔍 Verifying nginx removal..."
    
    if command_exists nginx; then
        echo "❌ Warning: nginx binary still found at $(which nginx)"
        echo "   You may need to remove it manually"
        return 1
    else
        echo "✅ Nginx completely removed from system"
        return 0
    fi
}

# Function to display summary
display_summary() {
    echo ""
    echo "📋 Uninstallation Summary"
    echo "========================"
    echo "✅ Nginx service stopped and disabled"
    echo "✅ Nginx package removed via DNF"
    echo "✅ Configuration files removed"
    echo "✅ Log files removed"
    echo "✅ Cache files removed"
    echo "✅ Web content removed"
    echo "✅ System user/group removed"
    echo "✅ Firewall rules removed"
    echo "✅ Package cache cleaned"
    echo ""
    
    if verify_removal; then
        echo "🎉 Nginx has been completely uninstalled!"
        echo "   Your RHEL 9 system is clean and ready for fresh installation if needed."
    else
        echo "⚠️  Some nginx components may still remain on the system"
        echo "   Please check manually and remove them if necessary"
    fi
}

# Main execution
main() {
    echo "🐧 Detected system: $(cat /etc/redhat-release 2>/dev/null || echo 'RHEL-based system')"
    echo "🏷️  Kernel: $(uname -r)"
    echo ""
    
    # Check if we're root
    check_root
    
    # Ask for confirmation
    read -p "❓ Are you sure you want to completely uninstall nginx? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "❌ Uninstallation cancelled by user"
        exit 0
    fi
    
    # Execute uninstallation steps
    stop_nginx
    remove_package
    remove_configs
    remove_user_group
    close_firewall_ports
    cleanup_remaining_files
    clean_cache
    
    # Display results
    display_summary
}

# Run the main function
main "$@" 