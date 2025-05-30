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

	// Get Git repository URL from environment
	repoURL := os.Getenv("ANSIBLE_REPO_URL")
	if repoURL == "" {
		log.Fatal("ANSIBLE_REPO_URL environment variable is required")
	}

	// Ensure the playbook directory exists
	if err := os.MkdirAll(playbookBasePath, 0755); err != nil {
		log.Fatalf("Failed to create playbook directory: %v", err)
	}

	// List of allowed playbooks (you might want to load this from a config file)
	allowedPlaybooks := []string{
		"site",
		"deploy",
		"backup",
		"maintenance",
		// Add more playbook names here
	}

	handler := api.NewHandler(playbookBasePath, repoURL, allowedPlaybooks)

	http.HandleFunc("/run-playbook", handler.RunPlaybookHandler)
	http.HandleFunc("/status/", handler.StatusHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Ansible API with Job Tracker running on :%s\n", port)
	fmt.Printf("Playbook directory: %s\n", playbookBasePath)
	fmt.Printf("Repository URL: %s\n", repoURL)
	fmt.Printf("Allowed playbooks: %v\n", allowedPlaybooks)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Server failed: ", err)
	}
}
