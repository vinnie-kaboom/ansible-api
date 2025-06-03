package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	LogDir      string `json:"log_dir"`
	ReposFile   string `json:"repos_file"`
	TemplateDir string `json:"template_dir"`
	Port        int    `json:"port"`
}

func getBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home, err = os.Getwd()
		if err != nil {
			home = "."
		}
	}
	return filepath.Join(home, "ansible", "playbooks")
}

var DefaultConfig = Config{
	LogDir:      filepath.Join(getBaseDir(), "logs"),
	ReposFile:   filepath.Join(getBaseDir(), "repos.json"),
	TemplateDir: filepath.Join(getBaseDir(), "templates"),
	Port:        8081,
}
