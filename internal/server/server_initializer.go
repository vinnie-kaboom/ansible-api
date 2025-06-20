package server

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	// "sync"
	"time"

	"ansible-api/internal/vault"

	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

var httpsGitRegex = regexp.MustCompile(`^https://[\w.@:/\-~]+\.git$`)

func httpsGitURLValidator(fl validator.FieldLevel) bool {
	gitURL := fl.Field().String()
	return httpsGitRegex.MatchString(gitURL)
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

func loadConfigFromVault(vaultClient *vault.VaultClient) (*Config, error) {
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
		log.Info().Err(err).Msg("GitHub configuration not found in Vault, will use environment variables")
	}

	// Load API configuration
	if apiConfig, err := vaultClient.GetSecret("ansible/api"); err == nil {
		for key, value := range apiConfig {
			config.setIntValue(key, value)
			config.setStringValue(key, value)
		}
	} else {
		log.Info().Err(err).Msg("API configuration not found in Vault, will use environment variables")
	}

	return config, nil
}

func loadConfigFromEnv(config *Config) {
	envMap := map[string]string{
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
	if config.PrivateKey == "" {
		config.PrivateKey = envMap["GITHUB_PRIVATE_KEY"]
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

// Server struct is defined in 00_server-structs.go

func New() (*Server, error) {
	// Set up logging
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

	// Initialize Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	s := &Server{
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

	s.JobProcessor = NewJobProcessor(s)
	s.registerRoutes()

	go s.JobProcessor.ProcessJobs()

	return s, nil
}

func (s *Server) registerRoutes() {
	r := s.Router
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.GET("/api/health", s.handleHealth)
	r.POST("/api/playbook/run", s.handlePlaybookRun)
	r.GET("/api/jobs", s.handleJobs)
	r.GET("/api/jobs/:job_id", s.handleJobStatus)
	r.POST("/api/jobs/:job_id/retry", s.handleJobRetry)
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(200, gin.H{"status": "healthy", "version": "1.0.0"})
}

func (s *Server) handlePlaybookRun(c *gin.Context) {
	if !s.RateLimiter.Allow() {
		c.JSON(429, gin.H{"error": "Too many requests"})
		return
	}
	var req PlaybookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.Logger.Error().Err(err).Msg("Invalid request body")
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}
	validate := validator.New()
	err := validate.RegisterValidation("httpsgit", httpsGitURLValidator)
	if err != nil {
		s.Logger.Error().Err(err).Msg("Failed to register httpsgit validator")
		c.JSON(500, gin.H{"error": "Internal server error"})
		return
	}
	if err := validate.Struct(req); err != nil {
		s.Logger.Error().Err(err).Msg("Validation failed")
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	job := &Job{
		ID:            fmt.Sprintf("job-%d", time.Now().UnixNano()),
		Status:        "queued",
		StartTime:     time.Now(),
		RepositoryURL: req.RepositoryURL,
		PlaybookPath:  req.PlaybookPath,
		TargetHosts:   req.TargetHosts,
		Inventory:     req.Inventory,
	}
	s.JobMutex.Lock()
	s.Jobs[job.ID] = job
	s.JobMutex.Unlock()

	s.JobQueue <- job

	c.JSON(202, gin.H{"status": "queued", "job_id": job.ID})
}

func (s *Server) handleJobs(c *gin.Context) {
	s.JobMutex.RLock()
	defer s.JobMutex.RUnlock()
	c.JSON(200, s.Jobs)
}

func (s *Server) handleJobStatus(c *gin.Context) {
	jobID := c.Param("job_id")
	s.JobMutex.RLock()
	job, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()
	if !exists {
		c.JSON(404, gin.H{"error": "Job not found"})
		return
	}
	c.JSON(200, job)
}

func (s *Server) handleJobRetry(c *gin.Context) {
	jobID := c.Param("job_id")
	s.JobMutex.RLock()
	origJob, exists := s.Jobs[jobID]
	s.JobMutex.RUnlock()
	if !exists {
		c.JSON(404, gin.H{"error": "Job not found"})
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

	c.JSON(202, gin.H{"status": "queued", "job_id": newJob.ID, "retry_of": jobID})
}

func (s *Server) Start() error {
	s.Logger.Info().Str("addr", ":"+s.Config.ServerPort).Msg("Starting server")
	return s.Router.Run(":" + s.Config.ServerPort)
}

func (s *Server) Stop() error {
	// Stop the Vault client
	if s.VaultClient != nil {
		s.VaultClient = nil
	}
	return nil
}
