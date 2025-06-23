package server

import (
	"ansible-api/internal/githubapp"
	"bytes"
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

func NewJobProcessor(server *Server) *JobProcessor {
	return &JobProcessor{
		server: server,
	}
}

// ProcessJobs continuously processes jobs from the queue
func (p *JobProcessor) ProcessJobs() {
	for job := range p.server.JobQueue {
		// Create a logger with job context for this entire job execution
		jobLogger := p.server.Logger.With().
			Str("job_id", job.ID).
			Str("repository", job.RepositoryURL).
			Str("playbook", job.PlaybookPath).
			Str("target_hosts", job.TargetHosts).
			Logger()

		jobLogger.Info().Msg("Starting job processing")

		p.server.JobMutex.Lock()
		job.Status = "running"
		p.server.JobMutex.Unlock()

		// Track job duration
		startTime := time.Now()
		defer func() {
			duration := time.Since(startTime)
			jobLogger.Info().
				Dur("duration", duration).
				Str("final_status", job.Status).
				Msg("Job processing completed")
		}()

		tmpDir, err := os.MkdirTemp("", "repo")
		if err != nil {
			jobLogger.Error().Err(err).Msg("Failed to create temporary directory")
			p.updateJobStatus(job, "failed", "", err.Error())
			continue
		}
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				jobLogger.Error().Err(err).Str("tmp_dir", tmpDir).Msg("Failed to remove temporary directory")
			} else {
				jobLogger.Debug().Str("tmp_dir", tmpDir).Msg("Temporary directory cleaned up")
			}
		}()

		jobLogger.Info().Msg("Authenticating with GitHub App")

		token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
			AppID:          p.server.GithubAppID,
			InstallationID: p.server.GithubInstallationID,
			PrivateKey:     p.server.GithubPrivateKey,
			APIBaseURL:     p.server.GithubAPIBaseURL,
		})
		if err != nil {
			jobLogger.Error().
				Err(err).
				Int("app_id", p.server.GithubAppID).
				Int("installation_id", p.server.GithubInstallationID).
				Str("api_base_url", p.server.GithubAPIBaseURL).
				Msg("Failed to authenticate with GitHub")
			p.updateJobStatus(job, "failed", "", "GitHub App authentication failed: "+err.Error())
			continue
		}

		jobLogger.Info().Msg("GitHub authentication successful")

		repoPath := extractRepoPath(job.RepositoryURL)
		host := extractHost(job.RepositoryURL)
		cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

		jobLogger.Info().
			Str("repository", repoPath).
			Str("host", host).
			Str("clone_url", maskTokenInURL(cloneURL)).
			Str("tmp_dir", tmpDir).
			Msg("Cloning repository")

		gitOutput := &gitOutputWriter{logger: jobLogger.With().Str("component", "git").Logger()}
		_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL:      cloneURL,
			Progress: gitOutput,
		})
		if err != nil {
			jobLogger.Error().
				Err(err).
				Str("repository", repoPath).
				Str("clone_url", maskTokenInURL(cloneURL)).
				Msg("Failed to clone repository")
			p.updateJobStatus(job, "failed", "", err.Error())
			continue
		}

		jobLogger.Info().Str("repository", repoPath).Msg("Repository cloned successfully")

		// Inventory handling with detailed logging
		inventoryFilePath := filepath.Join(tmpDir, "inventory", "hosts.ini")

		if job.Inventory == nil {
			jobLogger.Debug().
				Str("inventory_path", inventoryFilePath).
				Msg("No inventory provided in request, checking repository")

			if _, err := os.Stat(inventoryFilePath); os.IsNotExist(err) {
				jobLogger.Error().
					Str("inventory_path", inventoryFilePath).
					Msg("No inventory file found in repository and no inventory provided in request")
				p.updateJobStatus(job, "failed", "", "No inventory file found in repository and no inventory provided in request")
				continue
			} else {
				jobLogger.Info().Str("inventory_path", inventoryFilePath).Msg("Using repository inventory file")
			}
		} else {
			// Create inventory directory if it doesn't exist
			inventoryDir := filepath.Dir(inventoryFilePath)
			if err := os.MkdirAll(inventoryDir, 0755); err != nil {
				jobLogger.Error().Err(err).Str("inventory_dir", inventoryDir).Msg("Failed to create inventory directory")
				p.updateJobStatus(job, "failed", "", err.Error())
				continue
			}

			jobLogger.Info().
				Int("inventory_groups", len(job.Inventory)).
				Str("inventory_path", inventoryFilePath).
				Msg("Creating inventory file from request")

			inventoryFile, err := os.Create(inventoryFilePath)
			if err != nil {
				jobLogger.Error().Err(err).Str("inventory_path", inventoryFilePath).Msg("Failed to create inventory file")
				p.updateJobStatus(job, "failed", "", err.Error())
				continue
			}
			defer inventoryFile.Close()

			for group, hosts := range job.Inventory {
				jobLogger.Debug().
					Str("group", group).
					Int("hosts", len(hosts)).
					Msg("Writing inventory group")
				fmt.Fprintf(inventoryFile, "[%s]\n", group)
				for host, vars := range hosts {
					fmt.Fprintf(inventoryFile, "%s %s\n", host, vars)
				}
				fmt.Fprintf(inventoryFile, "\n")
			}
			jobLogger.Info().Msg("Inventory file created successfully")
		}

		playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
		ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
		if job.TargetHosts != "" {
			ansibleCmd.Args = append(ansibleCmd.Args, "--limit", job.TargetHosts)
		}
		ansibleCmd.Dir = tmpDir

		// Load SSH key from Vault for authentication
		sshKeyPath := ""
		if sshKey, err := p.server.VaultClient.GetSecret("ansible/ssh"); err == nil {
			if privateKey, exists := sshKey["private_key"]; exists {
				if privateKeyStr, ok := privateKey.(string); ok {
					// Create temporary SSH key file
					tmpKeyFile, err := os.CreateTemp("", "ansible-ssh-key-*")
					if err == nil {
						defer os.Remove(tmpKeyFile.Name())

						if err := tmpKeyFile.Chmod(0600); err == nil {
							if _, err := tmpKeyFile.WriteString(privateKeyStr); err == nil {
								tmpKeyFile.Close()
								sshKeyPath = tmpKeyFile.Name()
								jobLogger.Debug().Str("ssh_key_path", sshKeyPath).Msg("SSH key loaded from Vault")
							}
						}
					}
				}
			}
		}

		// Set environment variables to eliminate warnings
		ansibleCmd.Env = append(os.Environ(),
			"ANSIBLE_PYTHON_INTERPRETER="+p.getPythonInterpreter(),
			"ANSIBLE_HOST_KEY_CHECKING=False",
		)

		// Add SSH key to Ansible command if available
		if sshKeyPath != "" {
			ansibleCmd.Args = append(ansibleCmd.Args, "--private-key", sshKeyPath)
		}

		// Capture output
		var stdout, stderr bytes.Buffer
		ansibleCmd.Stdout = &stdout
		ansibleCmd.Stderr = &stderr

		jobLogger.Info().Msg("Executing Ansible playbook")
		err = ansibleCmd.Run()

		// Capture the raw output
		rawOutput := stdout.String()
		rawError := stderr.String()

		// Create structured output
		structuredOutput := p.createStructuredOutput(rawOutput, rawError, err)

		job.EndTime = time.Now()
		duration := job.EndTime.Sub(job.StartTime)

		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			jobLogger.Error().
				Err(err).
				Str("raw_output", rawOutput).
				Str("raw_error", rawError).
				Dur("duration", duration).
				Msg("Ansible playbook execution failed")
		} else {
			job.Status = "completed"
			jobLogger.Info().
				Dur("duration", duration).
				Msg("Ansible playbook execution completed successfully")
		}

		job.Output = structuredOutput

		// Record completed state
		logicalPlaybookPath := job.PlaybookPath
		if updateErr := UpdatePlaybookState(p.server, logicalPlaybookPath, playbookPath, job.RepositoryURL, job.Status, job.TargetHosts); updateErr != nil {
			jobLogger.Error().Err(updateErr).Msg("Failed to update playbook state")
		}
	}
}

func (p *JobProcessor) updateJobStatus(job *Job, status, output, errMsg string) {
	p.server.JobMutex.Lock()
	defer p.server.JobMutex.Unlock()

	job.Status = status
	job.Output = output
	if errMsg != "" {
		job.Error = errMsg
	}
	job.EndTime = time.Now()
}

func extractRepoPath(fullURL string) string {
	u, err := url.Parse(fullURL)
	if err != nil {
		return fullURL // fallback
	}
	return u.Path[1:] // remove leading slash
}

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
		return "github.com"
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
	w.buffer.Write(p)

	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
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

func (p *JobProcessor) createStructuredOutput(rawOutput, rawError string, err error) string {
	var result strings.Builder

	// Add header
	result.WriteString("=== ANSIBLE PLAYBOOK EXECUTION REPORT ===\n\n")

	// Parse and structure the output
	lines := strings.Split(rawOutput, "\n")

	// Extract playbook name
	playbookName := "Unknown"
	for _, line := range lines {
		if strings.Contains(line, "PLAY [") {
			playbookName = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "PLAY ["))
			break
		}
	}

	result.WriteString(fmt.Sprintf("📋 Playbook: %s\n", playbookName))
	result.WriteString(fmt.Sprintf("⏱️  Execution Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	result.WriteString(fmt.Sprintf("🔧 Status: %s\n\n", map[bool]string{true: "❌ FAILED", false: "✅ SUCCESS"}[err != nil]))

	// Extract and structure tasks
	result.WriteString("📝 TASK EXECUTION SUMMARY:\n")
	result.WriteString("─" + strings.Repeat("─", 50) + "\n")

	taskResults := p.parseTaskResults(rawOutput)
	for _, task := range taskResults {
		status := "✅"
		if task.Failed {
			status = "❌"
		} else if task.Changed {
			status = "🔄"
		} else if task.Skipped {
			status = "⏭️"
		}

		result.WriteString(fmt.Sprintf("%s %s\n", status, task.Name))
		if task.Host != "" {
			result.WriteString(fmt.Sprintf("   Host: %s\n", task.Host))
		}
		if task.Error != "" {
			result.WriteString(fmt.Sprintf("   Error: %s\n", task.Error))
		}
		result.WriteString("\n")
	}

	// Extract play recap
	result.WriteString("📊 PLAY RECAP:\n")
	result.WriteString("─" + strings.Repeat("─", 50) + "\n")

	recap := p.parsePlayRecap(rawOutput)
	for host, stats := range recap {
		result.WriteString(fmt.Sprintf("🏠 %s:\n", host))
		result.WriteString(fmt.Sprintf("   ✅ OK: %d\n", stats.Ok))
		result.WriteString(fmt.Sprintf("   🔄 Changed: %d\n", stats.Changed))
		result.WriteString(fmt.Sprintf("   ❌ Failed: %d\n", stats.Failed))
		result.WriteString(fmt.Sprintf("   ⏭️ Skipped: %d\n", stats.Skipped))
		result.WriteString(fmt.Sprintf("   🚫 Unreachable: %d\n", stats.Unreachable))
		result.WriteString("\n")
	}

	// Add error details if any
	if err != nil {
		result.WriteString("🚨 ERROR DETAILS:\n")
		result.WriteString("─" + strings.Repeat("─", 50) + "\n")
		result.WriteString(fmt.Sprintf("Error: %s\n", err.Error()))

		if rawError != "" {
			result.WriteString(fmt.Sprintf("Stderr: %s\n", rawError))
		}
	}

	// Add troubleshooting tips for common errors
	if strings.Contains(rawOutput, "Connection refused") {
		result.WriteString("\n💡 TROUBLESHOOTING TIP:\n")
		result.WriteString("SSH connection refused. Check:\n")
		result.WriteString("• SSH service is running on target host\n")
		result.WriteString("• SSH keys are properly configured\n")
		result.WriteString("• Firewall allows SSH connections\n")
		result.WriteString("• Target host is reachable\n")
	}

	return result.String()
}

type TaskResult struct {
	Name    string
	Host    string
	Failed  bool
	Changed bool
	Skipped bool
	Error   string
}

type PlayRecap struct {
	Ok          int
	Changed     int
	Failed      int
	Skipped     int
	Unreachable int
}

func (p *JobProcessor) parseTaskResults(output string) []TaskResult {
	var tasks []TaskResult
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if strings.Contains(line, "TASK [") {
			taskName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "TASK ["))
			task := TaskResult{Name: taskName}

			// Look for task result in subsequent lines
			for j := i + 1; j < len(lines) && j < i+10; j++ {
				resultLine := lines[j]
				if strings.Contains(resultLine, "fatal:") {
					task.Failed = true
					task.Error = strings.TrimSpace(strings.TrimPrefix(resultLine, "fatal:"))
					break
				} else if strings.Contains(resultLine, "changed:") {
					task.Changed = true
					break
				} else if strings.Contains(resultLine, "skipping:") {
					task.Skipped = true
					break
				} else if strings.Contains(resultLine, "ok:") {
					break
				}
			}
			tasks = append(tasks, task)
		}
	}

	return tasks
}

func (p *JobProcessor) parsePlayRecap(output string) map[string]PlayRecap {
	recap := make(map[string]PlayRecap)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "PLAY RECAP") {
			continue
		}

		// Parse lines like: localhost                  : ok=0    changed=0    unreachable=1    failed=0
		if strings.Contains(line, ":") && (strings.Contains(line, "ok=") || strings.Contains(line, "failed=")) {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				host := strings.TrimSpace(parts[0])
				stats := parts[1]

				var playRecap PlayRecap
				fmt.Sscanf(stats, "ok=%d changed=%d unreachable=%d failed=%d",
					&playRecap.Ok, &playRecap.Changed, &playRecap.Unreachable, &playRecap.Failed)

				recap[host] = playRecap
			}
		}
	}

	return recap
}

// getPythonInterpreter gets the Python interpreter, using config override if available, otherwise auto-detecting
func (p *JobProcessor) getPythonInterpreter() string {
	// Check if explicitly configured
	if p.server.Config != nil && p.server.Config.PythonInterpreter != "" {
		return p.server.Config.PythonInterpreter
	}

	// Check environment variable override
	if envInterpreter := os.Getenv("ANSIBLE_PYTHON_INTERPRETER_OVERRIDE"); envInterpreter != "" {
		return envInterpreter
	}

	// Auto-detect
	return detectPythonInterpreter()
}

// detectPythonInterpreter finds the best available Python interpreter (Linux-focused)
func detectPythonInterpreter() string {
	// List of Python interpreters to try, in order of preference
	pythonCandidates := []string{
		"python3",          // Most common, should work on most systems
		"/usr/bin/python3", // Standard location on Linux
		"python",           // Fallback to python (might be Python 2 or 3)
		"/usr/bin/python",  // Standard location fallback
		"python3.9",        // Specific versions if needed
		"python3.10",
		"python3.11",
		"python3.12",
		"python3.13",
	}

	for _, candidate := range pythonCandidates {
		// Use 'which' to check if command exists on Linux, then test version
		if cmd := exec.Command("which", candidate); cmd.Run() == nil {
			// Test if it's actually Python 3
			if testCmd := exec.Command(candidate, "--version"); testCmd.Run() == nil {
				if output, err := testCmd.Output(); err == nil {
					version := string(output)
					if strings.Contains(version, "Python 3") {
						return candidate
					}
				}
			}
		}
	}

	// If nothing else works, fallback to python3
	return "python3"
}
