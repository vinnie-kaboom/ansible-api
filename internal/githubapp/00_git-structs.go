package githubapp

type AuthConfig struct {
	AppID          int
	InstallationID int
	PrivateKeyPath string
	APIBaseURL     string
}

type AuthError struct {
	Op  string
	Err error
}
