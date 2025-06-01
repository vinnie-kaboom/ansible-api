package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type PlaybookRequest struct {
	Playbook  string            `json:"playbook"`
	Inventory string            `json:"inventory,omitempty"`
	ExtraVars map[string]string `json:"extra_vars,omitempty"`
	Branch    string            `json:"branch,omitempty"` // Optional branch to use
}

type Job struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time,omitempty"`
	Output    string                 `json:"output,omitempty"`
	Playbook  string                 `json:"playbook"`
	Inventory string                 `json:"inventory"`
	ExtraVars map[string]interface{} `json:"extra_vars"`
}

// Handler handles API requests
type Handler struct {
	jobStore         map[string]*Job
	mu               sync.Mutex
	playbookBasePath string
	allowedPlaybooks []string
}

// NewHandler creates a new API handler
func NewHandler(playbookBasePath string, allowedPlaybooks []string) *Handler {
	return &Handler{
		jobStore:         make(map[string]*Job),
		playbookBasePath: playbookBasePath,
		allowedPlaybooks: allowedPlaybooks,
	}
}

// HealthCheckHandler handles health check requests
func (h *Handler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// RunPlaybookHandler handles playbook execution requests
func (h *Handler) RunPlaybookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Playbook  string                 `json:"playbook"`
		Inventory string                 `json:"inventory"`
		ExtraVars map[string]interface{} `json:"extra_vars"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate playbook name
	validPlaybook := false
	for _, p := range h.allowedPlaybooks {
		if p == req.Playbook {
			validPlaybook = true
			break
		}
	}
	if !validPlaybook {
		http.Error(w, "Invalid playbook name", http.StatusBadRequest)
		return
	}

	// Create job
	job := &Job{
		ID:        uuid.New().String(),
		Status:    "pending",
		StartTime: time.Now(),
		Playbook:  req.Playbook,
		Inventory: req.Inventory,
		ExtraVars: req.ExtraVars,
	}

	// Store job
	h.mu.Lock()
	h.jobStore[job.ID] = job
	h.mu.Unlock()

	// Start job in background
	go h.executeJob(job)

	// Return job ID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id": job.ID,
	})
}

// StatusHandler handles job status requests
func (h *Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/status/")
	if jobID == "" {
		http.Error(w, "Job ID required", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	job, exists := h.jobStore[jobID]
	h.mu.Unlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// executeJob runs the specified playbook
func (h *Handler) executeJob(job *Job) {
	// Create command
	cmd := exec.Command("ansible-playbook",
		"-i", filepath.Join(h.playbookBasePath, "inventory", job.Inventory+".ini"),
		filepath.Join(h.playbookBasePath, "playbooks", job.Playbook+".yml"),
	)

	// Add extra vars if provided
	if len(job.ExtraVars) > 0 {
		extraVars, err := json.Marshal(job.ExtraVars)
		if err != nil {
			job.Status = "failed"
			job.Output = fmt.Sprintf("Error marshaling extra vars: %v", err)
			job.EndTime = time.Now()
			return
		}
		cmd.Args = append(cmd.Args, "-e", string(extraVars))
	}

	// Capture output
	output, err := cmd.CombinedOutput()
	job.Output = string(output)
	job.EndTime = time.Now()

	if err != nil {
		job.Status = "failed"
	} else {
		job.Status = "completed"
	}
}

func (h *Handler) notifyTeams(job *Job) {
	webhookURL := "<YOUR_TEAMS_WEBHOOK_URL>"
	statusColor := "00FF00"
	if job.Status == "failed" {
		statusColor = "FF0000"
	}

	payload := fmt.Sprintf(`{
		"@type": "MessageCard",
		"@context": "https://schema.org/extensions",
		"summary": "Ansible Job Status",
		"themeColor": "%s",
		"title": "Ansible Job %s",
		"sections": [
			{
				"text": "**Status**: %s\n\n**Job ID**: %s\n\n**Output**:\n\n%s"
			}
		]
	}`, statusColor, strings.ToUpper(job.Status), job.Status, job.ID, truncate(job.Output, 300))

	http.Post(webhookURL, "application/json", strings.NewReader(payload))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n..."
}
