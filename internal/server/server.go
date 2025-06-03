package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

type Server struct {
	mux         *http.ServeMux
	server      *http.Server
	logger      zerolog.Logger
	jobs        map[string]*Job
	jobMutex    sync.RWMutex
	jobQueue    chan *Job
	rateLimiter *rate.Limiter
}

type Job struct {
	ID            string
	Status        string
	StartTime     time.Time
	EndTime       time.Time
	Output        string
	Error         string
	RepositoryURL string
	PlaybookPath  string
	WebhookURL    string
}

type PlaybookRequest struct {
	RepositoryURL string                       `json:"repository_url"`
	PlaybookPath  string                       `json:"playbook_path"`
	Inventory     map[string]map[string]string `json:"inventory"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
	WebhookURL    string                       `json:"webhook_url"`
}

func New() (*Server, error) {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	s := &Server{
		mux:         http.NewServeMux(),
		logger:      log.With().Str("component", "server").Logger(),
		jobs:        make(map[string]*Job),
		jobQueue:    make(chan *Job, 100),
		rateLimiter: rate.NewLimiter(rate.Every(time.Second), 10), // 10 requests per second
	}

	// Register routes
	s.registerRoutes()

	s.server = &http.Server{
		Addr:    ":8080",
		Handler: s.mux,
	}

	// Start job processor
	go s.processJobs()

	return s, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth())
	s.mux.HandleFunc("/api/playbook/run", s.handlePlaybookRun())
	s.mux.HandleFunc("/api/jobs", s.handleJobs())
	s.mux.HandleFunc("/api/jobs/", s.handleJobStatus())
}

func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]string{
			"status":  "healthy",
			"version": "1.0.0",
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to encode health response")
			return
		}
	}
}

func (s *Server) handlePlaybookRun() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Rate limiting
		if !s.rateLimiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		var req PlaybookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.logger.Error().Err(err).Msg("Invalid request body")
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Create new job
		job := &Job{
			ID:            fmt.Sprintf("job-%d", time.Now().UnixNano()),
			Status:        "queued",
			StartTime:     time.Now(),
			RepositoryURL: req.RepositoryURL,
			PlaybookPath:  req.PlaybookPath,
			WebhookURL:    req.WebhookURL,
		}

		// Add job to map
		s.jobMutex.Lock()
		s.jobs[job.ID] = job
		s.jobMutex.Unlock()

		// Queue job
		s.jobQueue <- job

		// Respond with job ID
		response := map[string]string{
			"status": "queued",
			"job_id": job.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to encode response")
			return
		}
	}
}

func (s *Server) handleJobs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		s.jobMutex.RLock()
		defer s.jobMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(s.jobs)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to encode jobs response")
			return
		}
	}
}

func (s *Server) handleJobStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		jobID := r.URL.Path[len("/api/jobs/"):]
		s.jobMutex.RLock()
		job, exists := s.jobs[jobID]
		s.jobMutex.RUnlock()

		if !exists {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(job)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to encode job status response")
			return
		}
	}
}

func (s *Server) processJobs() {
	for job := range s.jobQueue {
		s.jobMutex.Lock()
		job.Status = "running"
		s.jobMutex.Unlock()

		// 1. Create a temp directory
		tmpDir, err := os.MkdirTemp("", "repo")
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			continue
		}
		defer os.RemoveAll(tmpDir)

		// 2. Clone the repo
		cmd := exec.Command("git", "clone", job.RepositoryURL, tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			s.updateJobStatus(job, "failed", string(output), err.Error())
			continue
		}

		// 3. Create inventory file
		inventoryFilePath := filepath.Join(tmpDir, "inventory.ini")
		inventoryFile, err := os.Create(inventoryFilePath)
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			continue
		}
		defer inventoryFile.Close()

		// 4. Run ansible-playbook
		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		ansibleCmd.Dir = tmpDir
		if output, err := ansibleCmd.CombinedOutput(); err != nil {
			s.updateJobStatus(job, "failed", string(output), err.Error())
			continue
		}

		s.updateJobStatus(job, "completed", "", "")
	}
}

func (s *Server) updateJobStatus(job *Job, status, output, errMsg string) {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()

	job.Status = status
	job.Output = output
	job.Error = errMsg
	job.EndTime = time.Now()

	// Webhook notification
	if job.WebhookURL != "" && (status == "completed" || status == "failed") {
		go func(jobCopy Job) {
			payload, err := json.Marshal(jobCopy)
			if err != nil {
				s.logger.Error().Err(err).Str("webhook", jobCopy.WebhookURL).Msg("Failed to marshal webhook payload")
				return
			}
			resp, err := http.Post(jobCopy.WebhookURL, "application/json", bytes.NewReader(payload))
			if err != nil {
				s.logger.Error().Err(err).Str("webhook", jobCopy.WebhookURL).Msg("Failed to send webhook")
				return
			}
			defer resp.Body.Close()
			s.logger.Info().Str("webhook", jobCopy.WebhookURL).Int("status", resp.StatusCode).Msg("Webhook sent")
		}(*job)
	}
}

func (s *Server) Start() error {
	s.logger.Info().Str("addr", s.server.Addr).Msg("Starting server")
	return s.server.ListenAndServe()
}

func (s *Server) Stop() error {
	if s.server != nil {
		s.logger.Info().Msg("Stopping server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}
