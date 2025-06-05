package service_model

import (
	"time"
)

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
	RepositoryURL string                       `json:"repository_url" validate:"required,httpsgit"`
	PlaybookPath  string                       `json:"playbook_path" validate:"required"`
	Inventory     map[string]map[string]string `json:"inventory" validate:"required,min=1"`
	Environment   map[string]string            `json:"environment"`
	Secrets       map[string]string            `json:"secrets"`
}
