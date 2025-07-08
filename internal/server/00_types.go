package server

import (
	"ansible-api/internal/ansible"
	"ansible-api/internal/vault"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

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
	// Drift detection settings
	DriftCheckOnlyOnRepoChange bool `json:"drift_check_only_on_repo_change"`
	DriftIgnoreDynamicContent  bool `json:"drift_ignore_dynamic_content"`
}

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
	AnsibleClient        *ansible.Client
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

type PlaybookState struct {
	Repo                  string   `json:"repo"`
	LastRun               string   `json:"last_run"`
	LastHash              string   `json:"last_hash"`
	LastStatus            string   `json:"last_status"`
	LastCheckOutput       string   `json:"last_check_output"`
	LastRemediation       string   `json:"last_remediation"`
	LastRemediationStatus string   `json:"last_remediation_status"`
	DriftDetected         bool     `json:"drift_detected"`
	LastTargets           []string `json:"last_targets"`
	PlaybookCommit        string   `json:"playbook_commit"`
	TargetHosts           string   `json:"target_hosts"`
}

// StateFile represents the state of all playbooks
type StateFile map[string]PlaybookState

// DriftDetector handles drift detection operations
type DriftDetector struct {
	server    *Server
	stateFile string
	logger    zerolog.Logger
}

// ConfigManager handles configuration loading and management
type ConfigManager struct {
	logger zerolog.Logger
}

// ServerBuilder handles server construction and initialization
type ServerBuilder struct {
	logger zerolog.Logger
}

// RequestValidator handles request validation
type RequestValidator struct {
	validator *validator.Validate
}

// JobProcessor handles job processing and execution
type JobProcessor struct {
	server *Server
}
