package githubapp

type AuthConfig struct {
	AppID          int    `json:"app_id"`
	InstallationID int    `json:"installation_id"`
	PrivateKey     string `json:"private_key"`
	APIBaseURL     string `json:"api_base_url"`
}

type AuthError struct {
	Op  string
	Err error
}
