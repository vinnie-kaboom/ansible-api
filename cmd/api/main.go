package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/vincent/ansible-api/internal/api"
)

func main() {
	// Get playbook base path from environment or use default
	playbookBasePath := os.Getenv("ANSIBLE_PLAYBOOK_PATH")
	if playbookBasePath == "" {
		playbookBasePath = "/etc/ansible/playbooks" // Default path
	}

	// Ensure the playbook directory exists
	if err := os.MkdirAll(playbookBasePath, 0755); err != nil {
		log.Fatalf("Failed to create playbook directory: %v", err)
	}

	// List of allowed playbooks (you might want to load this from a config file)
	allowedPlaybooks := []string{
		"run_playbook", // Main playbook for running other playbooks
		"site",         // Site deployment
		"deploy",       // Application deployment
		"backup",       // Backup operations
		"maintenance",  // Maintenance tasks
		"rollback",     // Rollback operations
	}

	handler := api.NewHandler(playbookBasePath, allowedPlaybooks)

	// API endpoints
	http.HandleFunc("/run-playbook", handler.RunPlaybookHandler)
	http.HandleFunc("/status/", handler.StatusHandler)
	http.HandleFunc("/health", handler.HealthCheckHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Ansible API running on :%s\n", port)
	fmt.Printf("Playbook directory: %s\n", playbookBasePath)
	fmt.Printf("Allowed playbooks: %v\n", allowedPlaybooks)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Server failed: ", err)
	}
}
