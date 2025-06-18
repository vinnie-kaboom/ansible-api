package server

import (
	"ansible-api/internal/githubapp"
	"fmt"
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

		p.server.Logger.Info().Msg("Authenticating with GitHub App")

		token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
			AppID:          p.server.GithubAppID,
			InstallationID: p.server.GithubInstallationID,
			PrivateKey:     p.server.GithubPrivateKey,
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

		// Log only the repository path, not the full URL with token
		p.server.Logger.Info().
			Str("repository", repoPath).
			Str("clone_url", maskTokenInURL(cloneURL)).
			Msg("Cloning repository")

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

		inventoryFilePath := filepath.Join(tmpDir, "inventory", "hosts.ini")
		fallbackInventoryFilePath := filepath.Join("inventory.ini")
		if job.Inventory == nil {
			if _, err := os.Stat(inventoryFilePath); os.IsNotExist(err) {
				if _, err := os.Stat(fallbackInventoryFilePath); os.IsNotExist(err) {
					p.updateJobStatus(job, "failed", "", "No inventory file found in repository and no inventory provided in request")
					err := os.RemoveAll(tmpDir)
					if err != nil {
						p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
						return
					}
					continue
				} else {
					inventoryFilePath = fallbackInventoryFilePath
				}
			}
		} else {
			// Create inventory file from request
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
			defer inventoryFile.Close()

			// Write inventory content
			for group, hosts := range job.Inventory {
				fmt.Fprintf(inventoryFile, "[%s]\n", group)
				for host, vars := range hosts {
					fmt.Fprintf(inventoryFile, "%s %s\n", host, vars)
				}
				fmt.Fprintf(inventoryFile, "\n")
			}
		}

		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		if job.TargetHosts != "" {
			ansibleCmd.Args = append(ansibleCmd.Args, "--limit", job.TargetHosts)
		}
		ansibleCmd.Dir = tmpDir

		ansibleOutput := &ansibleOutputWriter{logger: p.server.Logger.With().Str("component", "ansible").Logger()}
		ansibleCmd.Stdout = ansibleOutput
		ansibleCmd.Stderr = ansibleOutput

		p.server.Logger.Info().
			Str("playbook", job.PlaybookPath).
			Str("inventory", inventoryFilePath).
			Msg("Executing Ansible playbook")

		if err := ansibleCmd.Run(); err != nil {
			p.updateJobStatus(job, "failed", ansibleOutput.GetOutput(), err.Error())
			// Record failed state
			_ = UpdatePlaybookState(job.PlaybookPath, job.RepositoryURL, "failed")
			if job.Inventory != nil {
			}
			err := os.RemoveAll(tmpDir)
			if err != nil {
				p.server.Logger.Error().Err(err).Msg("Failed to remove temporary directory")
				return
			}
			continue
		}

		p.server.Logger.Info().
			Str("playbook", job.PlaybookPath).
			Msg("Ansible playbook execution completed")

		p.updateJobStatus(job, "completed", ansibleOutput.GetOutput(), "")
		// Record completed state
		_ = UpdatePlaybookState(job.PlaybookPath, job.RepositoryURL, "completed")
		// Only close inventoryFile if it was created (job.Inventory != nil)
		// No action needed if already closed by defer
		// Remove temporary directory
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

// ansibleOutputWriter is a custom writer to capture and format Ansible output
type ansibleOutputWriter struct {
	logger zerolog.Logger
	buffer strings.Builder
}

func (w *ansibleOutputWriter) Write(p []byte) (n int, err error) {
	// Store the output for job status
	w.buffer.Write(p)

	// Log each line separately for better readability
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Determine log level based on content
			if strings.Contains(line, "ERROR") || strings.Contains(line, "fatal:") {
				w.logger.Error().Str("output", line).Msg("Ansible execution")
			} else if strings.Contains(line, "WARNING") {
				w.logger.Warn().Str("output", line).Msg("Ansible execution")
			} else if strings.Contains(line, "TASK") || strings.Contains(line, "PLAY") {
				w.logger.Info().Str("output", line).Msg("Ansible execution")
			} else {
				w.logger.Debug().Str("output", line).Msg("Ansible execution")
			}
		}
	}
	return len(p), nil
}

func (w *ansibleOutputWriter) GetOutput() string {
	return w.buffer.String()
}
