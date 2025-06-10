package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultPort         = 8081
	ansibleFolderName   = "ansible"
	playbooksFolderName = "playbooks"
	logsFolderName      = "logs"
	reposFileName       = "repos.json"
	templatesFolderName = "templates"
)

// Config holds the application configuration settings
type Config struct {
	// LogDir specifies the directory for storing application logs
	LogDir string `json:"log_dir"`
	// ReposFile specifies the path to the JSON file containing repository configurations
	ReposFile string `json:"repos_file"`
	// TemplateDir specifies the directory containing template files
	TemplateDir string `json:"template_dir"`
	// Port specifies the HTTP server port
	Port int `json:"port"`
}

// New creates a new Config instance with default values
func New() *Config {
	baseDir := getBaseDir()
	return &Config{
		LogDir:      filepath.Join(baseDir, logsFolderName),
		ReposFile:   filepath.Join(baseDir, reposFileName),
		TemplateDir: filepath.Join(baseDir, templatesFolderName),
		Port:        defaultPort,
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", c.Port)
	}

	dirs := []string{c.LogDir, filepath.Dir(c.ReposFile), c.TemplateDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// getBaseDir returns the base directory for the application
func getBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home, err = os.Getwd()
		if err != nil {
			home = "."
		}
	}
	return filepath.Join(home, ansibleFolderName, playbooksFolderName)
}
