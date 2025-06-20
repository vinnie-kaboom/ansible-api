package server

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"ansible-api/internal/vault"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

// Config methods
func (c *Config) SetIntValue(key string, value interface{}) {
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

func (c *Config) SetStringValue(key string, value interface{}) {
	if str, ok := value.(string); ok {
		switch key {
		case "private_key":
			c.PrivateKey = str
		case "api_base_url":
			c.APIBaseURL = str
		case "port":
			c.ServerPort = str
		case "temp_patterns":
			c.TempPatterns = str
		}
	}
}

// NewConfigManager creates a new configuration manager
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		logger: log.With().Str("component", "config").Logger(),
	}
}

// LoadConfiguration loads configuration from multiple sources
func (cm *ConfigManager) LoadConfiguration(vaultClient *vault.VaultClient) (*Config, error) {
	config := &Config{}

	// Load from Vault first
	if err := cm.loadFromVault(vaultClient, config); err != nil {
		cm.logger.Warn().Err(err).Msg("Failed to load configuration from Vault")
	}

	// Load from environment variables
	cm.loadFromEnvironment(config)

	// Set defaults for missing values
	cm.setDefaults(config)

	return config, nil
}

// loadFromVault loads configuration from Vault
func (cm *ConfigManager) loadFromVault(vaultClient *vault.VaultClient, config *Config) error {
	if vaultClient == nil {
		return fmt.Errorf("vault client is nil")
	}

	// Load GitHub configuration
	if err := cm.loadGitHubConfig(vaultClient, config); err != nil {
		cm.logger.Info().Err(err).Msg("GitHub configuration not found in Vault")
	}

	// Load API configuration
	if err := cm.loadAPIConfig(vaultClient, config); err != nil {
		cm.logger.Info().Err(err).Msg("API configuration not found in Vault")
	}

	return nil
}

// loadGitHubConfig loads GitHub-specific configuration from Vault
func (cm *ConfigManager) loadGitHubConfig(vaultClient *vault.VaultClient, config *Config) error {
	githubConfig, err := vaultClient.GetSecret("ansible/github")
	if err != nil {
		return err
	}

	for key, value := range githubConfig {
		config.SetIntValue(key, value)
		config.SetStringValue(key, value)
	}

	return nil
}

// loadAPIConfig loads API-specific configuration from Vault
func (cm *ConfigManager) loadAPIConfig(vaultClient *vault.VaultClient, config *Config) error {
	apiConfig, err := vaultClient.GetSecret("ansible/api")
	if err != nil {
		return err
	}

	for key, value := range apiConfig {
		config.SetIntValue(key, value)
		config.SetStringValue(key, value)
	}

	return nil
}

// loadFromEnvironment loads configuration from environment variables
func (cm *ConfigManager) loadFromEnvironment(config *Config) {
	envVars := map[string]string{
		"GITHUB_APP_ID":                  "",
		"GITHUB_INSTALLATION_ID":         "",
		"GITHUB_PRIVATE_KEY":             "",
		"GITHUB_API_BASE_URL":            "",
		"PORT":                           "",
		"WORKER_COUNT":                   "",
		"RETENTION_HOURS":                "",
		"TEMP_PATTERNS":                  "",
		"RATE_LIMIT_REQUESTS_PER_SECOND": "",
	}

	// Load all environment variables
	for key := range envVars {
		envVars[key] = os.Getenv(key)
	}

	// Set values if they're not already set from Vault
	cm.setIntFromEnv(config, "AppID", envVars["GITHUB_APP_ID"])
	cm.setIntFromEnv(config, "InstallationID", envVars["GITHUB_INSTALLATION_ID"])
	cm.setStringFromEnv(config, "PrivateKey", envVars["GITHUB_PRIVATE_KEY"])
	cm.setStringFromEnv(config, "APIBaseURL", envVars["GITHUB_API_BASE_URL"])
	cm.setStringFromEnv(config, "ServerPort", envVars["PORT"])
	cm.setIntFromEnv(config, "WorkerCount", envVars["WORKER_COUNT"])
	cm.setIntFromEnv(config, "RetentionHours", envVars["RETENTION_HOURS"])
	cm.setStringFromEnv(config, "TempPatterns", envVars["TEMP_PATTERNS"])
	cm.setIntFromEnv(config, "RateLimit", envVars["RATE_LIMIT_REQUESTS_PER_SECOND"])
}

// setIntFromEnv sets an integer field from environment variable if not already set
func (cm *ConfigManager) setIntFromEnv(config *Config, field, value string) {
	if value == "" {
		return
	}

	switch field {
	case "AppID":
		if config.AppID == 0 {
			if intVal, err := strconv.Atoi(value); err == nil {
				config.AppID = intVal
			}
		}
	case "InstallationID":
		if config.InstallationID == 0 {
			if intVal, err := strconv.Atoi(value); err == nil {
				config.InstallationID = intVal
			}
		}
	case "WorkerCount":
		if config.WorkerCount == 0 {
			if intVal, err := strconv.Atoi(value); err == nil {
				config.WorkerCount = intVal
			}
		}
	case "RetentionHours":
		if config.RetentionHours == 0 {
			if intVal, err := strconv.Atoi(value); err == nil {
				config.RetentionHours = intVal
			}
		}
	case "RateLimit":
		if config.RateLimit == 0 {
			if intVal, err := strconv.Atoi(value); err == nil {
				config.RateLimit = intVal
			}
		}
	}
}

// setStringFromEnv sets a string field from environment variable if not already set
func (cm *ConfigManager) setStringFromEnv(config *Config, field, value string) {
	if value == "" {
		return
	}

	switch field {
	case "PrivateKey":
		if config.PrivateKey == "" {
			config.PrivateKey = value
		}
	case "APIBaseURL":
		if config.APIBaseURL == "" {
			config.APIBaseURL = value
		}
	case "ServerPort":
		if config.ServerPort == "" {
			config.ServerPort = value
		}
	case "TempPatterns":
		if config.TempPatterns == "" {
			config.TempPatterns = value
		}
	}
}

// setDefaults sets default values for configuration fields
func (cm *ConfigManager) setDefaults(config *Config) {
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
			cm.setIntDefault(config, key, v)
		case string:
			cm.setStringDefault(config, key, v)
		}
	}
}

// setIntDefault sets an integer default value if not already set
func (cm *ConfigManager) setIntDefault(config *Config, key string, value int) {
	switch key {
	case "worker_count":
		if config.WorkerCount == 0 {
			config.WorkerCount = value
		}
	case "retention_hours":
		if config.RetentionHours == 0 {
			config.RetentionHours = value
		}
	case "rate_limit":
		if config.RateLimit == 0 {
			config.RateLimit = value
		}
	}
}

// setStringDefault sets a string default value if not already set
func (cm *ConfigManager) setStringDefault(config *Config, key string, value string) {
	switch key {
	case "port":
		if config.ServerPort == "" {
			config.ServerPort = value
		}
	case "temp_patterns":
		if config.TempPatterns == "" {
			config.TempPatterns = value
		}
	case "api_base_url":
		if config.APIBaseURL == "" {
			config.APIBaseURL = value
		}
	}
}

// NewServerBuilder creates a new server builder
func NewServerBuilder() *ServerBuilder {
	return &ServerBuilder{
		logger: log.With().Str("component", "server-builder").Logger(),
	}
}

// Build creates and configures a new server instance
func (sb *ServerBuilder) Build() (*Server, error) {
	// Set up logging
	if err := sb.setupLogging(); err != nil {
		return nil, fmt.Errorf("failed to setup logging: %w", err)
	}

	// Initialize Vault client
	vaultClient, err := sb.initializeVault()
	if err != nil {
		sb.logger.Warn().Err(err).Msg("Failed to initialize Vault client, falling back to environment variables")
	}

	// Load configuration
	configManager := NewConfigManager()
	config, err := configManager.LoadConfiguration(vaultClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Build server
	server, err := sb.buildServer(config, vaultClient)
	if err != nil {
		return nil, fmt.Errorf("failed to build server: %w", err)
	}

	return server, nil
}

// setupLogging configures the logging system
func (sb *ServerBuilder) setupLogging() error {
	zerolog.TimeFieldFormat = time.RFC3339

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	})

	return nil
}

// initializeVault creates and initializes the Vault client
func (sb *ServerBuilder) initializeVault() (*vault.VaultClient, error) {
	return vault.NewClient()
}

// buildServer constructs the server with all components
func (sb *ServerBuilder) buildServer(config *Config, vaultClient *vault.VaultClient) (*Server, error) {
	// Initialize Gin router
	router := sb.initializeRouter()

	// Create server instance
	server := &Server{
		Router:               router,
		Logger:               log.With().Str("component", "server").Logger(),
		Jobs:                 make(map[string]*Job),
		JobQueue:             make(chan *Job, 100),
		JobMutex:             sync.RWMutex{},
		RateLimiter:          rate.NewLimiter(rate.Every(time.Second), config.RateLimit),
		GithubAppID:          config.AppID,
		GithubInstallationID: config.InstallationID,
		GithubPrivateKey:     config.PrivateKey,
		GithubAPIBaseURL:     config.APIBaseURL,
		VaultClient:          vaultClient,
		Config:               config,
	}

	// Initialize components
	server.JobProcessor = NewJobProcessor(server)
	server.registerRoutes()

	// Start background processes
	go server.JobProcessor.ProcessJobs()

	return server, nil
}

// initializeRouter creates and configures the Gin router
func (sb *ServerBuilder) initializeRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	return router
}

// NewRequestValidator creates a new request validator
func NewRequestValidator() *RequestValidator {
	v := validator.New()

	// Register custom validators
	httpsGitRegex := regexp.MustCompile(`^https://[\w.@:/\-~]+\.git$`)
	v.RegisterValidation("httpsgit", func(fl validator.FieldLevel) bool {
		gitURL := fl.Field().String()
		return httpsGitRegex.MatchString(gitURL)
	})

	return &RequestValidator{
		validator: v,
	}
}

// ValidatePlaybookRequest validates a playbook execution request
func (rv *RequestValidator) ValidatePlaybookRequest(req *PlaybookRequest) error {
	return rv.validator.Struct(req)
}

// Legacy function for backward compatibility
func New() (*Server, error) {
	builder := NewServerBuilder()
	return builder.Build()
}

// Server methods
func (s *Server) registerRoutes() {
	r := s.Router

	// Add request logging middleware
	r.Use(s.requestLogger())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.GET("/api/health", s.handleHealth)
	r.POST("/api/playbook/run", s.handlePlaybookRun)
	r.GET("/api/jobs", s.handleJobs)
	r.GET("/api/jobs/:job_id", s.handleJobStatus)
	r.POST("/api/jobs/:job_id/retry", s.handleJobRetry)
}

// requestLogger middleware logs all HTTP requests with structured data
func (s *Server) requestLogger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Create structured log entry
		s.Logger.Info().
			Str("method", param.Method).
			Str("path", param.Path).
			Str("remote_addr", param.ClientIP).
			Str("user_agent", param.Request.UserAgent()).
			Int("status", param.StatusCode).
			Int("body_size", param.BodySize).
			Dur("latency", param.Latency).
			Str("error", param.ErrorMessage).
			Msg("HTTP request")

		// Return empty string since we're handling logging ourselves
		return ""
	})
}

func (s *Server) handleHealth(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		s.Logger.Debug().
			Str("endpoint", "/api/health").
			Str("method", c.Request.Method).
			Str("remote_addr", c.ClientIP()).
			Dur("duration", duration).
			Msg("Health check completed")
	}()

	c.JSON(200, gin.H{"status": "healthy", "version": "1.0.0"})
}

func (s *Server) handlePlaybookRun(c *gin.Context) {
	startTime := time.Now()
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	// Create a logger with request context
	reqLogger := s.Logger.With().
		Str("request_id", requestID).
		Str("endpoint", "/api/playbook/run").
		Str("method", c.Request.Method).
		Str("remote_addr", c.ClientIP()).
		Str("user_agent", c.Request.UserAgent()).
		Logger()

	defer func() {
		duration := time.Since(startTime)
		reqLogger.Info().
			Dur("duration", duration).
			Msg("Playbook run request completed")
	}()

	reqLogger.Info().Msg("Received playbook run request")

	if !s.RateLimiter.Allow() {
		reqLogger.Warn().Msg("Rate limit exceeded")
		c.JSON(429, gin.H{"error": "Too many requests"})
		return
	}

	var req PlaybookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		reqLogger.Error().
			Err(err).
			Str("content_type", c.GetHeader("Content-Type")).
			Int64("content_length", c.Request.ContentLength).
			Msg("Invalid request body")
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}

	reqLogger.Info().
		Str("repository_url", req.RepositoryURL).
		Str("playbook_path", req.PlaybookPath).
		Str("target_hosts", req.TargetHosts).
		Int("inventory_groups", len(req.Inventory)).
		Msg("Request validation passed")

	// Validate request
	validator := NewRequestValidator()
	if err := validator.ValidatePlaybookRequest(&req); err != nil {
		reqLogger.Error().
			Err(err).
			Str("repository_url", req.RepositoryURL).
			Str("playbook_path", req.PlaybookPath).
			Msg("Request validation failed")
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Create and queue job
	job := s.createJob(&req)
	s.queueJob(job)

	reqLogger.Info().
		Str("job_id", job.ID).
		Str("repository_url", req.RepositoryURL).
		Str("playbook_path", req.PlaybookPath).
		Msg("Job queued successfully")

	c.JSON(202, gin.H{"status": "queued", "job_id": job.ID})
}

func (s *Server) createJob(req *PlaybookRequest) *Job {
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	s.Logger.Debug().
		Str("job_id", jobID).
		Str("repository_url", req.RepositoryURL).
		Str("playbook_path", req.PlaybookPath).
		Msg("Creating new job")

	return &Job{
		ID:            jobID,
		Status:        "queued",
		StartTime:     time.Now(),
		RepositoryURL: req.RepositoryURL,
		PlaybookPath:  req.PlaybookPath,
		TargetHosts:   req.TargetHosts,
		Inventory:     req.Inventory,
	}
}

func (s *Server) queueJob(job *Job) {
	s.JobMutex.Lock()
	s.Jobs[job.ID] = job
	jobCount := len(s.Jobs)
	s.JobMutex.Unlock()

	s.JobQueue <- job

	s.Logger.Debug().
		Str("job_id", job.ID).
		Int("total_jobs", jobCount).
		Int("queue_size", len(s.JobQueue)).
		Msg("Job added to queue")
}

func (s *Server) handleJobs(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		s.Logger.Debug().
			Str("endpoint", "/api/jobs").
			Str("method", c.Request.Method).
			Str("remote_addr", c.ClientIP()).
			Dur("duration", duration).
			Msg("Jobs list request completed")
	}()

	s.JobMutex.RLock()
	jobs := s.Jobs
	jobCount := len(jobs)
	s.JobMutex.RUnlock()

	s.Logger.Debug().
		Int("job_count", jobCount).
		Msg("Retrieved jobs list")

	c.JSON(200, jobs)
}

func (s *Server) handleJobStatus(c *gin.Context) {
	jobID := c.Param("job_id")
	startTime := time.Now()

	reqLogger := s.Logger.With().
		Str("job_id", jobID).
		Str("endpoint", "/api/jobs/:job_id").
		Str("method", c.Request.Method).
		Str("remote_addr", c.ClientIP()).
		Logger()

	defer func() {
		duration := time.Since(startTime)
		reqLogger.Debug().Dur("duration", duration).Msg("Job status request completed")
	}()

	reqLogger.Debug().Msg("Job status request received")

	s.JobMutex.RLock()
	job, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()

	if !exists {
		reqLogger.Warn().Msg("Job not found")
		c.JSON(404, gin.H{"error": "Job not found"})
		return
	}

	reqLogger.Debug().
		Str("job_status", job.Status).
		Time("start_time", job.StartTime).
		Msg("Job status retrieved")

	c.JSON(200, job)
}

func (s *Server) handleJobRetry(c *gin.Context) {
	jobID := c.Param("job_id")
	startTime := time.Now()

	reqLogger := s.Logger.With().
		Str("original_job_id", jobID).
		Str("endpoint", "/api/jobs/:job_id/retry").
		Str("method", c.Request.Method).
		Str("remote_addr", c.ClientIP()).
		Logger()

	defer func() {
		duration := time.Since(startTime)
		reqLogger.Debug().Dur("duration", duration).Msg("Job retry request completed")
	}()

	reqLogger.Info().Msg("Job retry request received")

	s.JobMutex.RLock()
	origJob, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()

	if !exists {
		reqLogger.Warn().Msg("Original job not found for retry")
		c.JSON(404, gin.H{"error": "Job not found"})
		return
	}

	reqLogger.Info().
		Str("original_status", origJob.Status).
		Int("original_retry_count", origJob.RetryCount).
		Msg("Creating retry job")

	newJob := s.createRetryJob(origJob)
	s.queueJob(newJob)

	reqLogger.Info().
		Str("new_job_id", newJob.ID).
		Int("new_retry_count", newJob.RetryCount).
		Msg("Retry job created and queued")

	c.JSON(202, gin.H{"status": "queued", "job_id": newJob.ID, "retry_of": jobID})
}

func (s *Server) createRetryJob(origJob *Job) *Job {
	newJobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	s.Logger.Debug().
		Str("original_job_id", origJob.ID).
		Str("new_job_id", newJobID).
		Int("retry_count", origJob.RetryCount+1).
		Msg("Creating retry job")

	newJob := *origJob
	newJob.ID = newJobID
	newJob.Status = "queued"
	newJob.StartTime = time.Now()
	newJob.EndTime = time.Time{}
	newJob.Output = ""
	newJob.Error = ""
	newJob.RetryCount = origJob.RetryCount + 1

	return &newJob
}

func (s *Server) Start() error {
	s.Logger.Info().Str("addr", ":"+s.Config.ServerPort).Msg("Starting server")
	return s.Router.Run(":" + s.Config.ServerPort)
}

func (s *Server) Stop() error {
	if s.VaultClient != nil {
		s.VaultClient = nil
	}
	return nil
}
