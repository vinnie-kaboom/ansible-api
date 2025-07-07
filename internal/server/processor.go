package server

import (
	"ansible-api/internal/githubapp"
	"bytes"
	"fmt"
	"io"
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
		p.processJob(job)
	}
}

func (p *JobProcessor) processJob(job *Job) {
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
		return
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
		return
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
		return
	}

	jobLogger.Info().Str("repository", repoPath).Msg("Repository cloned successfully")

	// Inventory handling with detailed logging
	inventoryFilePath := filepath.Join(tmpDir, "inventory", "hosts.ini")
	fallbackInventoryFilePath := filepath.Join("inventory.ini")

	if job.Inventory == nil {
		jobLogger.Debug().
			Str("primary_inventory", inventoryFilePath).
			Str("fallback_inventory", fallbackInventoryFilePath).
			Msg("No inventory provided in request, checking repository")

		if _, err := os.Stat(inventoryFilePath); os.IsNotExist(err) {
			if _, err := os.Stat(fallbackInventoryFilePath); os.IsNotExist(err) {
				jobLogger.Error().
					Str("primary_inventory", inventoryFilePath).
					Str("fallback_inventory", fallbackInventoryFilePath).
					Msg("No inventory file found in repository and no inventory provided in request")
				p.updateJobStatus(job, "failed", "", "No inventory file found in repository and no inventory provided in request")
				return
			} else {
				inventoryFilePath = fallbackInventoryFilePath
				jobLogger.Info().Str("inventory_path", inventoryFilePath).Msg("Using fallback inventory file")
			}
		} else {
			jobLogger.Info().Str("inventory_path", inventoryFilePath).Msg("Using repository inventory file")
		}
	} else {
		jobLogger.Info().
			Int("inventory_groups", len(job.Inventory)).
			Str("inventory_path", inventoryFilePath).
			Msg("Creating inventory file from request")

		inventoryFile, err := os.Create(inventoryFilePath)
		if err != nil {
			jobLogger.Error().Err(err).Str("inventory_path", inventoryFilePath).Msg("Failed to create inventory file")
			p.updateJobStatus(job, "failed", "", err.Error())
			return
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

	// Auto-create ansible.cfg from example if needed for better role resolution
	exampleConfigPath := filepath.Join(tmpDir, "ansible.cfg.example")
	configPath := filepath.Join(tmpDir, "ansible.cfg")
	if _, err := os.Stat(exampleConfigPath); err == nil {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if err := copyFile(exampleConfigPath, configPath); err != nil {
				jobLogger.Warn().Err(err).Msg("Failed to copy ansible.cfg.example, proceeding without it")
			} else {
				jobLogger.Info().Msg("Created ansible.cfg from example for optimal role resolution")
			}
		}
	}

	// Install Ansible collections if requirements file exists
	collectionsRequirementsPath := filepath.Join(tmpDir, "collections", "requirements.yml")
	if _, err := os.Stat(collectionsRequirementsPath); err == nil {
		jobLogger.Info().Str("requirements_file", collectionsRequirementsPath).Msg("Installing Ansible collections")

		collectionsCmd := exec.Command("ansible-galaxy", "collection", "install", "-r", collectionsRequirementsPath, "--force")
		collectionsCmd.Dir = tmpDir

		// Set same environment variables as main ansible command
		collectionsCmd.Env = append(os.Environ(),
			"ANSIBLE_HOST_KEY_CHECKING=False",
		)

		if collectionsOutput, err := collectionsCmd.CombinedOutput(); err != nil {
			jobLogger.Warn().
				Err(err).
				Str("output", string(collectionsOutput)).
				Msg("Failed to install collections, proceeding anyway")
		} else {
			jobLogger.Info().
				Str("output", string(collectionsOutput)).
				Msg("Collections installed successfully")
		}
	} else {
		jobLogger.Debug().Msg("No collections requirements file found")
	}

	// Get SSH key from pre-created AnsibleClient (restores original design)
	sshKeyPath := ""
	if p.server.AnsibleClient != nil && p.server.AnsibleClient.SSHKeyPath != "" {
		sshKeyPath = p.server.AnsibleClient.SSHKeyPath
		jobLogger.Info().Str("ssh_key_path", sshKeyPath).Msg("Using SSH key from AnsibleClient")
	} else {
		jobLogger.Warn().
			Bool("ansible_client_exists", p.server.AnsibleClient != nil).
			Str("vault_client_status", map[bool]string{true: "available", false: "nil"}[p.server.VaultClient != nil]).
			Msg("No SSH key available from AnsibleClient, trying fallback methods")

		// Fallback: Try to get SSH key directly from Vault if available
		if p.server.VaultClient != nil {
			if sshKey, err := p.server.VaultClient.GetSSHKey(); err == nil {
				// Create temporary SSH key file as fallback
				if tmpSSHFile, err := os.CreateTemp("", "ansible-ssh-fallback-*"); err == nil {
					defer os.Remove(tmpSSHFile.Name()) // Clean up
					if err := tmpSSHFile.Chmod(0600); err == nil {
						if _, err := tmpSSHFile.WriteString(sshKey); err == nil {
							tmpSSHFile.Close()
							sshKeyPath = tmpSSHFile.Name()
							jobLogger.Info().Str("ssh_key_path", sshKeyPath).Msg("Using fallback SSH key from Vault")
						}
					}
				}
			} else {
				jobLogger.Warn().Err(err).Msg("Failed to get SSH key from Vault as fallback")
			}
		}

		// Final fallback: Check for environment variable
		if sshKeyPath == "" {
			if envSSHKey := os.Getenv("ANSIBLE_SSH_PRIVATE_KEY_FILE"); envSSHKey != "" {
				if _, err := os.Stat(envSSHKey); err == nil {
					sshKeyPath = envSSHKey
					jobLogger.Info().Str("ssh_key_path", sshKeyPath).Msg("Using SSH key from environment variable ANSIBLE_SSH_PRIVATE_KEY_FILE")
				} else {
					jobLogger.Warn().Str("ssh_key_path", envSSHKey).Err(err).Msg("SSH key file from environment variable not found")
				}
			}
		}

		if sshKeyPath == "" {
			jobLogger.Warn().Msg("No SSH key available from any source - ansible-playbook will use default SSH authentication")
		}
	}

	playbookPath := filepath.Join(tmpDir, job.PlaybookPath)
	ansibleCmd := exec.Command("ansible-playbook", playbookPath, "-i", inventoryFilePath)
	if job.TargetHosts != "" {
		ansibleCmd.Args = append(ansibleCmd.Args, "--limit", job.TargetHosts)
	}

	// Add SSH key if available (fallback option)
	if sshKeyPath != "" {
		ansibleCmd.Args = append(ansibleCmd.Args, "--private-key", sshKeyPath)
		jobLogger.Info().Str("ssh_key_path", sshKeyPath).Msg("Added SSH private key to ansible-playbook command")
	} else {
		jobLogger.Info().Msg("No SSH key available - using password authentication")
	}
	ansibleCmd.Dir = tmpDir

	// Log the full command being executed
	jobLogger.Info().
		Strs("command_args", ansibleCmd.Args).
		Str("working_dir", ansibleCmd.Dir).
		Msg("Executing ansible-playbook command")

	// Set environment variables to eliminate warnings and fix role paths
	ansibleCmd.Env = append(os.Environ(),
		"ANSIBLE_PYTHON_INTERPRETER=/usr/bin/python3.13",
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_ROLES_PATH=./roles:./playbooks/roles:~/.ansible/roles:/usr/share/ansible/roles:/etc/ansible/roles",
	)

	// Pass SSH credentials from Vault via environment variables (air-gapped friendly)
	if p.server.VaultClient != nil {
		if credentials, err := p.server.VaultClient.GetSecret("ansible/credentials"); err == nil {
			if username, ok := credentials["username"]; ok {
				ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_SSH_USER="+username.(string))
				jobLogger.Info().Str("username", username.(string)).Msg("Set ANSIBLE_SSH_USER from Vault")
			}
			if password, ok := credentials["password"]; ok {
				ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_SSH_PASSWORD="+password.(string))
				jobLogger.Info().Msg("Set ANSIBLE_SSH_PASSWORD from Vault (length: " + fmt.Sprintf("%d", len(password.(string))) + ")")
			}
			if sudoPassword, ok := credentials["sudo_password"]; ok {
				ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_BECOME_PASSWORD="+sudoPassword.(string))
				jobLogger.Info().Msg("Set ANSIBLE_BECOME_PASSWORD from Vault (length: " + fmt.Sprintf("%d", len(sudoPassword.(string))) + ")")
			}
		} else {
			jobLogger.Warn().Err(err).Msg("Failed to get credentials from Vault, checking environment")
		}
	}

	// Fallback: Use existing environment variables if Vault unavailable
	if sshUser := os.Getenv("ANSIBLE_SSH_USER"); sshUser != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_SSH_USER="+sshUser)
		jobLogger.Info().Str("username", sshUser).Msg("Using ANSIBLE_SSH_USER from environment")
	}
	if sshPassword := os.Getenv("ANSIBLE_SSH_PASSWORD"); sshPassword != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_SSH_PASSWORD="+sshPassword)
		jobLogger.Info().Msg("Using ANSIBLE_SSH_PASSWORD from environment (length: " + fmt.Sprintf("%d", len(sshPassword)) + ")")
	}
	if becomePassword := os.Getenv("ANSIBLE_BECOME_PASSWORD"); becomePassword != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_BECOME_PASSWORD="+becomePassword)
		jobLogger.Info().Msg("Using ANSIBLE_BECOME_PASSWORD from environment (length: " + fmt.Sprintf("%d", len(becomePassword)) + ")")
	}

	// Debug: Log ALL environment variables being passed to ansible
	jobLogger.Info().Int("total_env_vars", len(ansibleCmd.Env)).Msg("Total environment variables for ansible-playbook")
	for _, envVar := range ansibleCmd.Env {
		if strings.HasPrefix(envVar, "ANSIBLE_SSH_") || strings.HasPrefix(envVar, "ANSIBLE_BECOME_") {
			// Mask the passwords but show the variables are set
			if strings.HasPrefix(envVar, "ANSIBLE_SSH_PASSWORD=") {
				jobLogger.Debug().Str("env_var", "ANSIBLE_SSH_PASSWORD=***MASKED***").Msg("Environment variable set")
			} else if strings.HasPrefix(envVar, "ANSIBLE_BECOME_PASSWORD=") {
				jobLogger.Debug().Str("env_var", "ANSIBLE_BECOME_PASSWORD=***MASKED***").Msg("Environment variable set")
			} else {
				jobLogger.Debug().Str("env_var", envVar).Msg("Environment variable set")
			}
		}
	}

	// Add Kerberos environment variables if available
	if krb5Config := os.Getenv("KRB5_CONFIG"); krb5Config != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "KRB5_CONFIG="+krb5Config)
		jobLogger.Info().Str("krb5_config", krb5Config).Msg("Using KRB5_CONFIG environment variable")
	}
	if krb5CCName := os.Getenv("KRB5CCNAME"); krb5CCName != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "KRB5CCNAME="+krb5CCName)
		jobLogger.Info().Str("krb5_ccname", krb5CCName).Msg("Using KRB5CCNAME environment variable")
	}
	if kerberosUser := os.Getenv("ANSIBLE_REMOTE_USER"); kerberosUser != "" {
		ansibleCmd.Env = append(ansibleCmd.Env, "ANSIBLE_REMOTE_USER="+kerberosUser)
		jobLogger.Info().Str("kerberos_user", kerberosUser).Msg("Using ANSIBLE_REMOTE_USER environment variable")
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

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// copyDir function removed - using environment variable approach instead
