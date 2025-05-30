package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	ID      string    `json:"id"`
	Status  string    `json:"status"` // pending, running, success, failed
	Output  string    `json:"output"`
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
}

type Handler struct {
	jobStore         map[string]*Job
	mu               sync.Mutex
	playbookBasePath string
	allowedPlaybooks map[string]bool
	repoURL          string
}

func NewHandler(playbookBasePath, repoURL string, allowedPlaybooks []string) *Handler {
	allowed := make(map[string]bool)
	for _, name := range allowedPlaybooks {
		allowed[name] = true
	}

	return &Handler{
		jobStore:         make(map[string]*Job),
		playbookBasePath: playbookBasePath,
		allowedPlaybooks: allowed,
		repoURL:          repoURL,
	}
}

func (h *Handler) ensurePlaybookRepo(branch string) error {
	// If branch is not specified, use default
	if branch == "" {
		branch = "main"
	}

	// Check if directory exists
	if _, err := os.Stat(h.playbookBasePath); os.IsNotExist(err) {
		// Clone the repository
		cmd := exec.Command("git", "clone", "-b", branch, h.repoURL, h.playbookBasePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to clone repository: %v\nOutput: %s", err, output)
		}
	} else {
		// Pull latest changes
		cmd := exec.Command("git", "-C", h.playbookBasePath, "fetch", "origin", branch)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to fetch repository: %v\nOutput: %s", err, output)
		}

		cmd = exec.Command("git", "-C", h.playbookBasePath, "checkout", branch)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to checkout branch: %v\nOutput: %s", err, output)
		}

		cmd = exec.Command("git", "-C", h.playbookBasePath, "pull", "origin", branch)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to pull changes: %v\nOutput: %s", err, output)
		}
	}
	return nil
}

func (h *Handler) RunPlaybookHandler(w http.ResponseWriter, r *http.Request) {
	var req PlaybookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON input", http.StatusBadRequest)
		return
	}

	// Validate playbook name
	if !h.allowedPlaybooks[req.Playbook] {
		http.Error(w, "Playbook not allowed", http.StatusForbidden)
		return
	}

	// Ensure we have the latest playbooks
	if err := h.ensurePlaybookRepo(req.Branch); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update playbooks: %v", err), http.StatusInternalServerError)
		return
	}

	// Construct full playbook path
	playbookPath := filepath.Join(h.playbookBasePath, req.Playbook)
	if !strings.HasSuffix(playbookPath, ".yml") && !strings.HasSuffix(playbookPath, ".yaml") {
		playbookPath += ".yml"
	}

	// Verify playbook exists
	if _, err := os.Stat(playbookPath); os.IsNotExist(err) {
		http.Error(w, "Playbook not found", http.StatusNotFound)
		return
	}

	jobID := uuid.New().String()
	job := &Job{
		ID:      jobID,
		Status:  "pending",
		Started: time.Now(),
	}

	h.mu.Lock()
	h.jobStore[jobID] = job
	h.mu.Unlock()

	go h.runPlaybookAsync(playbookPath, req, job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func (h *Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Missing job ID", http.StatusBadRequest)
		return
	}
	jobID := parts[2]

	h.mu.Lock()
	job, ok := h.jobStore[jobID]
	h.mu.Unlock()

	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) runPlaybookAsync(playbookPath string, req PlaybookRequest, job *Job) {
	h.mu.Lock()
	job.Status = "running"
	h.mu.Unlock()

	cmdArgs := []string{"ansible-playbook", playbookPath}
	if req.Inventory != "" {
		// Validate inventory path is within base path
		inventoryPath := filepath.Join(h.playbookBasePath, "inventory", req.Inventory)
		if _, err := os.Stat(inventoryPath); err == nil {
			cmdArgs = append(cmdArgs, "-i", inventoryPath)
		}
	}
	if len(req.ExtraVars) > 0 {
		vars := []string{}
		for k, v := range req.ExtraVars {
			vars = append(vars, fmt.Sprintf("%s=%s", k, v))
		}
		cmdArgs = append(cmdArgs, "--extra-vars", strings.Join(vars, " "))
	}

	output, err := exec.Command(cmdArgs[0], cmdArgs[1:]...).CombinedOutput()

	h.mu.Lock()
	defer h.mu.Unlock()
	job.Output = string(output)
	job.Ended = time.Now()
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	} else {
		job.Status = "success"
	}

	// Optionally notify Teams
	h.notifyTeams(job)
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
