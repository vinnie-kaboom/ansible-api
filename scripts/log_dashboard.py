#!/usr/bin/env python3

from flask import Flask, render_template, jsonify
import os
import json
from datetime import datetime
import glob

app = Flask(__name__)

# Configuration
CONFIG = {
    'log_dir': '/etc/ansible/playbooks/logs',
    'repos_file': '/etc/ansible/playbooks/repos.json',
    'template_dir': '/etc/ansible/playbooks/templates'
}

def get_repo_info():
    """Get repository information from repos.json"""
    try:
        with open(CONFIG['repos_file'], 'r') as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError):
        return []

def get_log_files(repo_name):
    """Get all log files for a repository"""
    log_path = os.path.join(CONFIG['log_dir'], repo_name)
    if not os.path.exists(log_path):
        return []
    return sorted(glob.glob(os.path.join(log_path, '*.log')), reverse=True)

def parse_log_file(log_file):
    """Parse a log file and extract relevant information"""
    try:
        with open(log_file, 'r') as f:
            content = f.read()
            
        # Extract basic information
        info = {
            'filename': os.path.basename(log_file),
            'timestamp': datetime.fromtimestamp(os.path.getmtime(log_file)).strftime('%Y-%m-%d %H:%M:%S'),
            'size': os.path.getsize(log_file),
            'content': content,
            'status': 'success' if 'failed=0' in content else 'failed'
        }
        
        return info
    except Exception as e:
        return {
            'filename': os.path.basename(log_file),
            'error': str(e),
            'status': 'error'
        }

@app.route('/')
def index():
    """Render the main dashboard"""
    repos = get_repo_info()
    return render_template('dashboard.html', repos=repos)

@app.route('/api/logs/<repo_name>')
def get_logs(repo_name):
    """API endpoint to get logs for a repository"""
    log_files = get_log_files(repo_name)
    logs = [parse_log_file(log_file) for log_file in log_files]
    return jsonify(logs)

@app.route('/api/stats')
def get_stats():
    """API endpoint to get overall statistics"""
    repos = get_repo_info()
    stats = {
        'total_repos': len(repos),
        'total_logs': 0,
        'successful_runs': 0,
        'failed_runs': 0
    }
    
    for repo in repos:
        log_files = get_log_files(repo['name'])
        stats['total_logs'] += len(log_files)
        for log_file in log_files:
            content = open(log_file, 'r').read()
            if 'failed=0' in content:
                stats['successful_runs'] += 1
            else:
                stats['failed_runs'] += 1
    
    return jsonify(stats)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=True) 