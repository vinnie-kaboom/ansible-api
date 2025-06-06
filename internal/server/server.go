package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	servicemodel "ansible-api/datamodel/service-model"
	"ansible-api/internal/githubapp"
	"ansible-api/internal/vault"

	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
	"gopkg.in/src-d/go-git.v4"
)

var httpsGitRegex = regexp.MustCompile(`^https://[\w.@:/\-~]+\.git$`)

func httpsGitURLValidator(fl validator.FieldLevel) bool {
	gitURL := fl.Field().String()
	return httpsGitRegex.MatchString(gitURL)
}

type PlaybookRequest struct {
	RepositoryURL string                       `json:"repository_url" validate:"required,httpsgit"`
	PlaybookPath  string                       `json:"playbook_path" validate:"required"`
	Inventory     map[string]map[string]string `json:"inventory" validate:"required,min=1"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
}

type Server struct {
	Mux                  *http.ServeMux
	Server               *http.Server
	Logger               zerolog.Logger
	Jobs                 map[string]*servicemodel.Job
	JobMutex             sync.RWMutex
	JobQueue             chan *servicemodel.Job
	RateLimiter          *rate.Limiter
	GithubAppID          int
	GithubInstallationID int
	GithubPrivateKeyPath string
	GithubAPIBaseURL     string
	VaultClient          *vault.Client
}

func New() (*Server, error) {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Initialize Vault client
	vaultClient, err := vault.NewClient()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize Vault client, falling back to environment variables")
	}

	// Get configuration from Vault if available, otherwise use environment variables
	var (
		appID          string
		installationID string
		privateKeyPath string
		apiBaseURL     string
		serverPort     string
		workerCount    string
		retentionHours string
		tempPatterns   string
		rateLimit      string
	)

	if vaultClient != nil {
		// Try to get GitHub configuration from Vault
		if githubConfig, err := vaultClient.GetSecret("ansible/github"); err == nil {
			appID = fmt.Sprint(githubConfig["app_id"])
			installationID = fmt.Sprint(githubConfig["installation_id"])
			privateKeyPath = fmt.Sprint(githubConfig["private_key_path"])
		}

		// Try to get API configuration from Vault
		if apiConfig, err := vaultClient.GetSecret("ansible/api"); err == nil {
			serverPort = fmt.Sprint(apiConfig["port"])
			workerCount = fmt.Sprint(apiConfig["worker_count"])
			retentionHours = fmt.Sprint(apiConfig["retention_hours"])
		}
	}

	// Fall back to environment variables if Vault is not available or secrets not found
	if appID == "" {
		appID = os.Getenv("GITHUB_APP_ID")
	}
	if installationID == "" {
		installationID = os.Getenv("GITHUB_INSTALLATION_ID")
	}
	if privateKeyPath == "" {
		privateKeyPath = os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	}
	if apiBaseURL == "" {
		apiBaseURL = os.Getenv("GITHUB_API_BASE_URL")
	}
	if serverPort == "" {
		serverPort = os.Getenv("PORT")
	}
	if workerCount == "" {
		workerCount = os.Getenv("WORKER_COUNT")
	}
	if retentionHours == "" {
		retentionHours = os.Getenv("RETENTION_HOURS")
	}
	if tempPatterns == "" {
		tempPatterns = os.Getenv("TEMP_PATTERNS")
	}
	if rateLimit == "" {
		rateLimit = os.Getenv("RATE_LIMIT_REQUESTS_PER_SECOND")
	}

	// Convert string values to integers
	appIDInt, _ := strconv.Atoi(appID)
	installationIDInt, _ := strconv.Atoi(installationID)
	workerCountInt, _ := strconv.Atoi(workerCount)
	retentionHoursInt, _ := strconv.Atoi(retentionHours)
	rateLimitInt, _ := strconv.Atoi(rateLimit)

	// Set defaults if values are not set
	if serverPort == "" {
		serverPort = "8080"
	}
	if workerCountInt == 0 {
		workerCountInt = 4
	}
	if retentionHoursInt == 0 {
		retentionHoursInt = 24
	}
	if rateLimitInt == 0 {
		rateLimitInt = 10
	}
	if tempPatterns == "" {
		tempPatterns = "*_site.yml,*_hosts"
	}
	if apiBaseURL == "" {
		apiBaseURL = "https://api.github.com"
	}

	s := &Server{
		Mux:                  http.NewServeMux(),
		Logger:               log.With().Str("component", "server").Logger(),
		Jobs:                 make(map[string]*servicemodel.Job),
		JobQueue:             make(chan *servicemodel.Job, 100),
		RateLimiter:          rate.NewLimiter(rate.Every(time.Second), rateLimitInt),
		GithubAppID:          appIDInt,
		GithubInstallationID: installationIDInt,
		GithubPrivateKeyPath: privateKeyPath,
		GithubAPIBaseURL:     apiBaseURL,
		VaultClient:          vaultClient,
	}

	s.registerRoutes()

	s.Server = &http.Server{
		Addr:    ":" + serverPort,
		Handler: s.Mux,
	}

	go s.processJobs()

	return s, nil
}

func (s *Server) registerRoutes() {
	s.Mux.HandleFunc("/api/health", s.handleHealth())
	s.Mux.HandleFunc("/api/playbook/run", s.handlePlaybookRun())
	s.Mux.HandleFunc("/api/jobs", s.handleJobs())
	s.Mux.HandleFunc("/api/jobs/", s.handleJobsDispatcher())
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
			s.Logger.Error().Err(err).Msg("Failed to encode health response")
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

		if !s.RateLimiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		var req PlaybookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.Logger.Error().Err(err).Msg("Invalid request body")
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		validate := validator.New()
		err := validate.RegisterValidation("httpsgit", httpsGitURLValidator)
		if err != nil {
			s.Logger.Error().Err(err).Msg("Failed to register httpsgit validator")
			return
		}
		if err := validate.Struct(req); err != nil {
			s.Logger.Error().Err(err).Msg("Validation failed")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		job := &servicemodel.Job{
			ID:            fmt.Sprintf("job-%d", time.Now().UnixNano()),
			Status:        "queued",
			StartTime:     time.Now(),
			RepositoryURL: req.RepositoryURL,
			PlaybookPath:  req.PlaybookPath,
		}

		s.JobMutex.Lock()
		s.Jobs[job.ID] = job
		s.JobMutex.Unlock()

		s.JobQueue <- job

		response := map[string]string{
			"status": "queued",
			"job_id": job.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.Logger.Error().Err(err).Msg("Failed to encode response")
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

		s.JobMutex.RLock()
		defer s.JobMutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(s.Jobs)
		if err != nil {
			s.Logger.Error().Err(err).Msg("Failed to encode jobs response")
			return
		}
	}
}

func (s *Server) handleJobsDispatcher() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/jobs/"):]
		if strings.HasSuffix(path, "/retry") && r.Method == http.MethodPost {
			jobID := strings.TrimSuffix(path, "/retry")
			jobID = strings.TrimSuffix(jobID, "/")
			s.handleJobRetry(jobID, w)
			return
		} else if r.Method == http.MethodGet {
			jobID := path
			s.handleJobStatus(jobID, w)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (s *Server) handleJobStatus(jobID string, w http.ResponseWriter) {
	s.JobMutex.RLock()
	job, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(job); err != nil {
		s.Logger.Error().Err(err).Msg("Failed to encode job status response")
		return
	}
}

func (s *Server) handleJobRetry(jobID string, w http.ResponseWriter) {
	s.JobMutex.RLock()
	origJob, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()
	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	newJob := *origJob
	newJob.ID = fmt.Sprintf("job-%d", time.Now().UnixNano())
	newJob.Status = "queued"
	newJob.StartTime = time.Now()
	newJob.EndTime = time.Time{}
	newJob.Output = ""
	newJob.Error = ""
	newJob.RetryCount = origJob.RetryCount + 1

	s.JobMutex.Lock()
	s.Jobs[newJob.ID] = &newJob
	s.JobMutex.Unlock()

	s.JobQueue <- &newJob

	response := map[string]string{
		"status":   "queued",
		"job_id":   newJob.ID,
		"retry_of": jobID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.Logger.Error().Err(err).Msg("Failed to encode retry response")
		return
	}
}

func (s *Server) processJobs() {
	for job := range s.JobQueue {
		s.JobMutex.Lock()
		job.Status = "running"
		s.JobMutex.Unlock()

		tmpDir, err := os.MkdirTemp("", "repo")
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			continue
		}

		s.Logger.Info().Msg("Attempting to authenticate with GitHub")

		token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
			AppID:          s.GithubAppID,
			InstallationID: s.GithubInstallationID,
			PrivateKeyPath: s.GithubPrivateKeyPath,
			APIBaseURL:     s.GithubAPIBaseURL,
		})
		if err != nil {
			s.Logger.Error().Err(err).Msg("Failed to authenticate with GitHub")
			s.updateJobStatus(job, "failed", "", "GitHub App authentication failed: "+err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		repoPath := extractRepoPath(job.RepositoryURL)
		cloneURL := githubapp.BuildCloneURL(token, repoPath, "github.com")

		maskedCloneURL := maskTokenInURL(cloneURL)
		s.Logger.Info().Str("clone_url", maskedCloneURL).Msg("Cloning repository")

		_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL:      cloneURL,
			Progress: os.Stdout,
		})
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		inventoryFilePath := filepath.Join(tmpDir, "inventory.ini")
		inventoryFile, err := os.Create(inventoryFilePath)
		if err != nil {
			s.updateJobStatus(job, "failed", "", err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		ansibleCmd.Dir = tmpDir
		if output, err := ansibleCmd.CombinedOutput(); err != nil {
			s.updateJobStatus(job, "failed", string(output), err.Error())
			err := inventoryFile.Close()
			if err != nil {
				s.Logger.Error().Err(err).Msg("Failed to close inventory file")
				return
			}
			err = os.RemoveAll(tmpDir)
			if err != nil {
				s.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		s.updateJobStatus(job, "completed", "", "")
		err = inventoryFile.Close()
		if err != nil {
			s.Logger.Error().Err(err).Msg("Failed to close inventory file")
			return
		}
		err = os.RemoveAll(tmpDir)
		if err != nil {
			s.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
			return
		}
	}
}

func (s *Server) updateJobStatus(job *servicemodel.Job, status, output, errMsg string) {
	s.JobMutex.Lock()
	defer s.JobMutex.Unlock()

	job.Status = status
	job.Output = output
	job.Error = errMsg
	job.EndTime = time.Now()
}

func (s *Server) Start() error {
	s.Logger.Info().Str("addr", s.Server.Addr).Msg("Starting server")
	return s.Server.ListenAndServe()
}

func (s *Server) Stop() error {
	if s.Server != nil {
		s.Logger.Info().Msg("Stopping server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Server.Shutdown(ctx)
	}
	return nil
}

func extractRepoPath(fullURL string) string {
	u, err := url.Parse(fullURL)
	if err != nil {
		return fullURL // fallback
	}
	return u.Path[1:] // remove leading slash
}

func maskTokenInURL(cloneURL string) string {
	u, err := url.Parse(cloneURL)
	if err != nil || u.User == nil {
		return cloneURL
	}
	username := u.User.Username()
	if _, hasToken := u.User.Password(); hasToken {
		u.User = url.UserPassword(username, "****")
		return u.String()
	}
	return cloneURL
}
