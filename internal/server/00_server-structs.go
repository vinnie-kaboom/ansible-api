package server

import (
	"ansible-api/internal/vault"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// Config holds application configuration loaded from Vault or environment variables.
type Config struct {
	AppID          int
	InstallationID int
	PrivateKey     string
	APIBaseURL     string
	ServerPort     string
	WorkerCount    int
	RetentionHours int
	TempPatterns   string
	RateLimit      int
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
	Inventory     map[string]map[string]string `json:"inventory" validate:"required,min=1"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
}

// Job represents a playbook execution job.
type Job struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	Output        string    `json:"output"`
	Error         string    `json:"error"`
	RepositoryURL string    `json:"repository_url"`
	PlaybookPath  string    `json:"playbook_path"`
	RetryCount    int       `json:"retry_count"`
}
