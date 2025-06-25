#!/bin/bash

# Ansible API Jobs Formatter
# Provides nicely formatted output for job status

API_BASE=${API_BASE:-"http://localhost:8080"}

show_help() {
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo "Commands:"
    echo "  summary    Show a quick overview of all jobs"
    echo "  list       Show detailed list of all jobs"
    echo "  raw        Show raw JSON output"
    echo "  watch      Watch job status in real-time"
    echo ""
    echo "Examples:"
    echo "  $0 summary              # Quick overview"
    echo "  $0 list                 # Detailed job list"
    echo "  $0 raw                  # Raw JSON for scripts"
    echo "  $0 watch                # Real-time monitoring"
}

format_summary() {
    echo "🎯 Ansible API Jobs Summary"
    echo "═══════════════════════════════════════"
    
    response=$(curl -s "${API_BASE}/api/jobs/summary" 2>/dev/null)
    
    if [ $? -ne 0 ] || [ -z "$response" ]; then
        echo "❌ Unable to connect to Ansible API at ${API_BASE}"
        return 1
    fi
    
    # Extract overview
    total=$(echo "$response" | jq -r '.overview.total_jobs // 0')
    completed=$(echo "$response" | jq -r '.overview.completed // 0')
    failed=$(echo "$response" | jq -r '.overview.failed // 0')
    running=$(echo "$response" | jq -r '.overview.running // 0')
    queued=$(echo "$response" | jq -r '.overview.queued // 0')
    
    echo "📊 Overview:"
    echo "   Total Jobs: $total"
    echo "   ✅ Completed: $completed"
    echo "   ❌ Failed: $failed"
    echo "   🔄 Running: $running"
    echo "   ⏳ Queued: $queued"
    echo ""
    
    # Show recent jobs
    echo "📋 Recent Jobs:"
    echo "$response" | jq -r '.recent_jobs[]? | "   \(.status_icon) \(.job_id[0:16])... \(.playbook) → \(.target) (\(.duration // "no duration"))"'
}

format_list() {
    echo "📋 Ansible API Jobs - Detailed List"
    echo "═══════════════════════════════════════════════════════════════"
    
    response=$(curl -s "${API_BASE}/api/jobs" 2>/dev/null)
    
    if [ $? -ne 0 ] || [ -z "$response" ]; then
        echo "❌ Unable to connect to Ansible API at ${API_BASE}"
        return 1
    fi
    
    # Show summary header
    total=$(echo "$response" | jq -r '.total_jobs // 0')
    completed=$(echo "$response" | jq -r '.summary.completed // 0')
    failed=$(echo "$response" | jq -r '.summary.failed // 0')
    running=$(echo "$response" | jq -r '.summary.running // 0')
    queued=$(echo "$response" | jq -r '.summary.queued // 0')
    
    echo "📊 Total: $total | ✅ $completed | ❌ $failed | 🔄 $running | ⏳ $queued"
    echo ""
    
    # Show each job
    echo "$response" | jq -r '.jobs[]? | 
        "🔹 Job: \(.id)
   Status: \(.status_emoji) \(.status)
   Playbook: \(.playbook)
   Target: \(.target_hosts)
   Repository: \(.repository)
   Started: \(.start_time)
   Duration: \(.duration // "not finished")
   Retries: \(.retry_count)
   Summary: \(.short_summary)
   ───────────────────────────────────────"'
}

show_raw() {
    echo "📄 Raw JSON Output:"
    echo "════════════════════"
    curl -s "${API_BASE}/api/jobs?format=raw" | jq '.'
}

watch_jobs() {
    echo "👀 Watching Jobs (Press Ctrl+C to stop)"
    echo "══════════════════════════════════════"
    
    while true; do
        clear
        echo "🕐 $(date '+%Y-%m-%d %H:%M:%S')"
        echo ""
        format_summary
        echo ""
        echo "Refreshing in 5 seconds..."
        sleep 5
    done
}

# Main script logic
case "${1:-summary}" in
    "help"|"-h"|"--help")
        show_help
        ;;
    "summary"|"s")
        format_summary
        ;;
    "list"|"l")
        format_list
        ;;
    "raw"|"r")
        show_raw
        ;;
    "watch"|"w")
        watch_jobs
        ;;
    *)
        echo "❌ Unknown command: $1"
        echo ""
        show_help
        exit 1
        ;;
esac 