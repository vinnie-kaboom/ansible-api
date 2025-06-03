package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
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
	RetryCount    int
}

type PlaybookRequest struct {
	RepositoryURL string                       `json:"repository_url" validate:"required,url"`
	PlaybookPath  string                       `json:"playbook_path" validate:"required"`
	Inventory     map[string]map[string]string `json:"inventory" validate:"required,min=1"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
}

func New() (*Server, error) {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	s := &Server{
		mux:         http.NewServeMux(),
		logger:      log.With().Str("component", "server").Logger(),
		jobs:        make(map[string]*Job),
		jobQueue:    make(chan *Job, 100),
		rateLimiter: rate.NewLimiter(rate.Every(time.Second), 10),
	}

	s.registerRoutes()

	s.server = &http.Server{
		Addr:    ":8080",
		Handler: s.mux,
	}

	go s.processJobs()

	return s, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth())
	s.mux.HandleFunc("/api/playbook/run", s.handlePlaybookRun())
	s.mux.HandleFunc("/api/jobs", s.handleJobs())
	s.mux.HandleFunc("/api/jobs/", s.handleJobsDispatcher())
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

		// Use go-playground/validator for input validation
		validate := validator.New()
		if err := validate.Struct(req); err != nil {
			s.logger.Error().Err(err).Msg("Validation failed")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Create new job
		job := &Job{
			ID:            fmt.Sprintf("job-%d", time.Now().UnixNano()),
			Status:        "queued",
			StartTime:     time.Now(),
			RepositoryURL: req.RepositoryURL,
			PlaybookPath:  req.PlaybookPath,
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
		if err := json.NewEncoder(w).Encode(response); err != nil {
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

func (s *Server) handleJobsDispatcher() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/jobs/"):]
		if strings.HasSuffix(path, "/retry") && r.Method == http.MethodPost {
			// /api/jobs/{id}/retry
			jobID := strings.TrimSuffix(path, "/retry")
			jobID = strings.TrimSuffix(jobID, "/")
			s.handleJobRetry(jobID, w)
			return
		} else if r.Method == http.MethodGet {
			// /api/jobs/{id}
			jobID := path
			s.handleJobStatus(jobID, w)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (s *Server) handleJobStatus(jobID string, w http.ResponseWriter) {
	s.jobMutex.RLock()
	job, exists := s.jobs[jobID]
	s.jobMutex.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(job); err != nil {
		s.logger.Error().Err(err).Msg("Failed to encode job status response")
		return
	}
}

func (s *Server) handleJobRetry(jobID string, w http.ResponseWriter) {
	s.jobMutex.RLock()
	origJob, exists := s.jobs[jobID]
	s.jobMutex.RUnlock()
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Clone the job with incremented RetryCount and new ID
	newJob := *origJob
	newJob.ID = fmt.Sprintf("job-%d", time.Now().UnixNano())
	newJob.Status = "queued"
	newJob.StartTime = time.Now()
	newJob.EndTime = time.Time{}
	newJob.Output = ""
	newJob.Error = ""
	newJob.RetryCount = origJob.RetryCount + 1

	s.jobMutex.Lock()
	s.jobs[newJob.ID] = &newJob
	s.jobMutex.Unlock()

	s.jobQueue <- &newJob

	response := map[string]string{
		"status":   "queued",
		"job_id":   newJob.ID,
		"retry_of": jobID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error().Err(err).Msg("Failed to encode retry response")
		return
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

		// 2. Clone the repo
		cmd := exec.Command("git", "clone", job.RepositoryURL, tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			s.updateJobStatus(job, "failed", string(output), err.Error())
			os.RemoveAll(tmpDir)
			continue
		}

		// 3. Create inventory file
		inventoryFilePath := filepath.Join(tmpDir, "inventory.ini")
		inventoryFile, err := os.Create(inventoryFilePath)
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			os.RemoveAll(tmpDir)
			continue
		}

		// 4. Run ansible-playbook
		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		ansibleCmd.Dir = tmpDir
		if output, err := ansibleCmd.CombinedOutput(); err != nil {
			s.updateJobStatus(job, "failed", string(output), err.Error())
			inventoryFile.Close()
			os.RemoveAll(tmpDir)
			continue
		}

		s.updateJobStatus(job, "completed", "", "")
		inventoryFile.Close()
		os.RemoveAll(tmpDir)
	}
}

func (s *Server) updateJobStatus(job *Job, status, output, errMsg string) {
	s.jobMutex.Lock()
	defer s.jobMutex.Unlock()

	job.Status = status
	job.Output = output
	job.Error = errMsg
	job.EndTime = time.Now()
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
