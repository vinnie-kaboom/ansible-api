package server

import (
	"ansible-api/internal/githubapp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/src-d/go-git.v4"
)

// JobProcessor handles the processing of Ansible playbook jobs
type JobProcessor struct {
	server *Server
}

// NewJobProcessor creates a new JobProcessor instance
func NewJobProcessor(server *Server) *JobProcessor {
	return &JobProcessor{
		server: server,
	}
}

// ProcessJobs continuously processes jobs from the queue
func (p *JobProcessor) ProcessJobs() {
	for job := range p.server.JobQueue {
		p.server.JobMutex.Lock()
		job.Status = "running"
		p.server.JobMutex.Unlock()

		tmpDir, err := os.MkdirTemp("", "repo")
		if err != nil {
			p.updateJobStatus(job, "failed", "", err.Error())
			continue
		}

		p.server.Logger.Info().Msg("Attempting to authenticate with GitHub")

		token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
			AppID:          p.server.GithubAppID,
			InstallationID: p.server.GithubInstallationID,
			PrivateKeyPath: p.server.GithubPrivateKeyPath,
			APIBaseURL:     p.server.GithubAPIBaseURL,
		})
		if err != nil {
			p.server.Logger.Error().Err(err).Msg("Failed to authenticate with GitHub")
			p.updateJobStatus(job, "failed", "", "GitHub App authentication failed: "+err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		repoPath := extractRepoPath(job.RepositoryURL)
		host := extractHost(job.RepositoryURL)
		cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

		maskedCloneURL := maskTokenInURL(cloneURL)
		p.server.Logger.Info().Str("clone_url", maskedCloneURL).Msg("Cloning repository")

		// Create a custom writer to capture and format Git output
		gitOutput := &gitOutputWriter{logger: p.server.Logger.With().Str("component", "git").Logger()}
		_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL:      cloneURL,
			Progress: gitOutput,
		})
		if err != nil {
			p.updateJobStatus(job, "failed", "", err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		inventoryFilePath := filepath.Join(tmpDir, "inventory.ini")
		inventoryFile, err := os.Create(inventoryFilePath)
		if err != nil {
			p.updateJobStatus(job, "failed", "", err.Error())
			err := os.RemoveAll(tmpDir)
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		ansibleCmd.Dir = tmpDir
		if output, err := ansibleCmd.CombinedOutput(); err != nil {
			p.updateJobStatus(job, "failed", string(output), err.Error())
			err := inventoryFile.Close()
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to close inventory file")
				return
			}
			err = os.RemoveAll(tmpDir)
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		p.updateJobStatus(job, "completed", "", "")
		err = inventoryFile.Close()
		if err != nil {
			p.server.Logger.Error().Err(err).Msg("Failed to close inventory file")
			return
		}
		err = os.RemoveAll(tmpDir)
		if err != nil {
			p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
			return
		}
	}
}

// updateJobStatus updates the status of a job
func (p *JobProcessor) updateJobStatus(job *Job, status, output, errMsg string) {
	p.server.JobMutex.Lock()
	defer p.server.JobMutex.Unlock()

	job.Status = status
	job.Output = output
	job.Error = errMsg
	job.EndTime = time.Now()
}

// extractRepoPath extracts the repository path from a full URL
func extractRepoPath(fullURL string) string {
	u, err := url.Parse(fullURL)
	if err != nil {
		return fullURL // fallback
	}
	return u.Path[1:] // remove leading slash
}

// maskTokenInURL masks the token in a URL for logging
func maskTokenInURL(cloneURL string) string {
	u, err := url.Parse(cloneURL)
	if err != nil || u.User == nil {
		return cloneURL
	}
	username := u.User.Username()
	if _, hasToken := u.User.Password(); hasToken {
		u.User = url.UserPassword(username, "****")
		return u.String()
	}
	return cloneURL
}

// extractHost extracts the host from a repository URL
func extractHost(repoURL string) string {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "github.com" // fallback to github.com if parsing fails
	}
	return u.Host
}

// gitOutputWriter is a custom writer to capture and format Git output
type gitOutputWriter struct {
	logger zerolog.Logger
}

func (w *gitOutputWriter) Write(p []byte) (n int, err error) {
	// Convert Git's progress output to a single info line
	output := strings.TrimSpace(string(p))
	if output != "" {
		w.logger.Info().Str("progress", output).Msg("Git clone progress")
	}
	return len(p), nil
}
