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

type Config struct {
	AppID          int
	InstallationID int
	PrivateKeyPath string
	APIBaseURL     string
	ServerPort     string
	WorkerCount    int
	RetentionHours int
	TempPatterns   string
	RateLimit      int
}

func (c *Config) setIntValue(key string, value interface{}) {
	if str, ok := value.(string); ok {
		if intVal, err := strconv.Atoi(str); err == nil {
			switch key {
			case "app_id":
				c.AppID = intVal
			case "installation_id":
				c.InstallationID = intVal
			case "worker_count":
				c.WorkerCount = intVal
			case "retention_hours":
				c.RetentionHours = intVal
			case "rate_limit":
				c.RateLimit = intVal
			}
		}
	}
}

func (c *Config) setStringValue(key string, value interface{}) {
	if str, ok := value.(string); ok {
		switch key {
		case "private_key_path":
			c.PrivateKeyPath = str
		case "api_base_url":
			c.APIBaseURL = str
		case "port":
			c.ServerPort = str
		case "temp_patterns":
			c.TempPatterns = str
		}
	}
}

func loadConfigFromVault(vaultClient *vault.Client) (*Config, error) {
	config := &Config{}

	if vaultClient == nil {
		return config, nil
	}

	// Load GitHub configuration
	if githubConfig, err := vaultClient.GetSecret("ansible/github"); err == nil {
		for key, value := range githubConfig {
			config.setIntValue(key, value)
			config.setStringValue(key, value)
		}
	} else {
		log.Info().Msg("GitHub configuration not found in Vault, will use environment variables")
	}

	// Load API configuration
	if apiConfig, err := vaultClient.GetSecret("ansible/api"); err == nil {
		for key, value := range apiConfig {
			config.setIntValue(key, value)
			config.setStringValue(key, value)
		}
	} else {
		log.Info().Msg("API configuration not found in Vault, will use environment variables")
	}

	return config, nil
}

func loadConfigFromEnv(config *Config) {
	envMap := map[string]string{
		"GITHUB_APP_ID":                  "",
		"GITHUB_INSTALLATION_ID":         "",
		"GITHUB_PRIVATE_KEY_PATH":        "",
		"GITHUB_API_BASE_URL":            "",
		"PORT":                           "",
		"WORKER_COUNT":                   "",
		"RETENTION_HOURS":                "",
		"TEMP_PATTERNS":                  "",
		"RATE_LIMIT_REQUESTS_PER_SECOND": "",
	}

	// Load all environment variables
	for key := range envMap {
		envMap[key] = os.Getenv(key)
	}

	// Set values if they're not already set from Vault
	if config.AppID == 0 && envMap["GITHUB_APP_ID"] != "" {
		config.AppID, _ = strconv.Atoi(envMap["GITHUB_APP_ID"])
	}
	if config.InstallationID == 0 && envMap["GITHUB_INSTALLATION_ID"] != "" {
		config.InstallationID, _ = strconv.Atoi(envMap["GITHUB_INSTALLATION_ID"])
	}
	if config.PrivateKeyPath == "" {
		config.PrivateKeyPath = envMap["GITHUB_PRIVATE_KEY_PATH"]
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = envMap["GITHUB_API_BASE_URL"]
	}
	if config.ServerPort == "" {
		config.ServerPort = envMap["PORT"]
	}
	if config.WorkerCount == 0 && envMap["WORKER_COUNT"] != "" {
		config.WorkerCount, _ = strconv.Atoi(envMap["WORKER_COUNT"])
	}
	if config.RetentionHours == 0 && envMap["RETENTION_HOURS"] != "" {
		config.RetentionHours, _ = strconv.Atoi(envMap["RETENTION_HOURS"])
	}
	if config.TempPatterns == "" {
		config.TempPatterns = envMap["TEMP_PATTERNS"]
	}
	if config.RateLimit == 0 && envMap["RATE_LIMIT_REQUESTS_PER_SECOND"] != "" {
		config.RateLimit, _ = strconv.Atoi(envMap["RATE_LIMIT_REQUESTS_PER_SECOND"])
	}
}

func setDefaultConfig(config *Config) {
	defaults := map[string]interface{}{
		"port":            "8080",
		"worker_count":    4,
		"retention_hours": 24,
		"rate_limit":      10,
		"temp_patterns":   "*_site.yml,*_hosts",
		"api_base_url":    "https://api.github.com",
	}

	for key, value := range defaults {
		switch v := value.(type) {
		case int:
			switch key {
			case "worker_count":
				if config.WorkerCount == 0 {
					config.WorkerCount = v
				}
			case "retention_hours":
				if config.RetentionHours == 0 {
					config.RetentionHours = v
				}
			case "rate_limit":
				if config.RateLimit == 0 {
					config.RateLimit = v
				}
			}
		case string:
			switch key {
			case "port":
				if config.ServerPort == "" {
					config.ServerPort = v
				}
			case "temp_patterns":
				if config.TempPatterns == "" {
					config.TempPatterns = v
				}
			case "api_base_url":
				if config.APIBaseURL == "" {
					config.APIBaseURL = v
				}
			}
		}
	}
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

	// Load configuration
	config, err := loadConfigFromVault(vaultClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration from Vault: %w", err)
	}

	loadConfigFromEnv(config)
	setDefaultConfig(config)

	s := &Server{
		Mux:                  http.NewServeMux(),
		Logger:               log.With().Str("component", "server").Logger(),
		Jobs:                 make(map[string]*servicemodel.Job),
		JobQueue:             make(chan *servicemodel.Job, 100),
		RateLimiter:          rate.NewLimiter(rate.Every(time.Second), config.RateLimit),
		GithubAppID:          config.AppID,
		GithubInstallationID: config.InstallationID,
		GithubPrivateKeyPath: config.PrivateKeyPath,
		GithubAPIBaseURL:     config.APIBaseURL,
		VaultClient:          vaultClient,
	}

	s.registerRoutes()

	s.Server = &http.Server{
		Addr:    ":" + config.ServerPort,
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
