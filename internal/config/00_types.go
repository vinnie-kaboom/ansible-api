package config

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
