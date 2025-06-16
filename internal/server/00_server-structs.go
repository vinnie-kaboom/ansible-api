package server

import (
	"ansible-api/internal/vault"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// Config holds the server configuration
type Config struct {
	AppID          int    `json:"app_id"`
	InstallationID int    `json:"installation_id"`
	PrivateKey     string `json:"private_key"`
	APIBaseURL     string `json:"api_base_url"`
	ServerPort     string `json:"port"`
	WorkerCount    int    `json:"worker_count"`
	RetentionHours int    `json:"retention_hours"`
	TempPatterns   string `json:"temp_patterns"`
	RateLimit      int    `json:"rate_limit"`
}

// Server represents the main API server and its dependencies.
type Server struct {
	Router               *gin.Engine
	Logger               zerolog.Logger
	Jobs                 map[string]*Job
	JobMutex             sync.RWMutex
	JobQueue             chan *Job
	RateLimiter          *rate.Limiter
	GithubAppID          int
	GithubInstallationID int
	GithubPrivateKey     string
	GithubAPIBaseURL     string
	VaultClient          *vault.VaultClient
	JobProcessor         *JobProcessor
	Config               *Config
}

// PlaybookRequest represents a request to run an Ansible playbook.
type PlaybookRequest struct {
	RepositoryURL string                       `json:"repository_url" validate:"required,httpsgit"`
	PlaybookPath  string                       `json:"playbook_path" validate:"required"`
	Inventory     map[string]map[string]string `json:"inventory"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
	TargetHosts   string                       `json:"target_hosts"`
}

// Job represents a playbook execution job.
type Job struct {
	ID            string                       `json:"id"`
	Status        string                       `json:"status"`
	StartTime     time.Time                    `json:"start_time"`
	EndTime       time.Time                    `json:"end_time"`
	Output        string                       `json:"output"`
	Error         string                       `json:"error"`
	RepositoryURL string                       `json:"repository_url"`
	PlaybookPath  string                       `json:"playbook_path"`
	RetryCount    int                          `json:"retry_count"`
	TargetHosts   string                       `json:"target_hosts"`
	Inventory     map[string]map[string]string `json:"inventory"`
}
