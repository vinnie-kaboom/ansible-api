package githubapp

type AuthConfig struct {
	AppID          int
	InstallationID int
	PrivateKey     string
	APIBaseURL     string
}

type AuthError struct {
	Op  string
	Err error
}
