package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DeployResult represents the result of a deployment
type DeployResult struct {
	Success   bool
	Skipped   bool
	Output    string
	Error     string
	StartTime time.Time
	EndTime   time.Time
}

// Duration returns the deployment duration
func (r *DeployResult) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// Deployer handles deployment execution with locking
type Deployer struct {
	logger        *Logger
	locks         map[string]*sync.Mutex
	locksMu       sync.Mutex
	notifier      *EmailNotifier
	configManager *ConfigManager
	activeBuilds  int32 // atomic counter for active builds
}

// NewDeployer creates a new deployer instance
func NewDeployer(logger *Logger) *Deployer {
	return &Deployer{
		logger: logger,
		locks:  make(map[string]*sync.Mutex),
	}
}

// SetNotifier sets the email notifier
func (d *Deployer) SetNotifier(notifier *EmailNotifier) {
	d.notifier = notifier
}

// SetConfigManager sets the config manager for deferred reload support
func (d *Deployer) SetConfigManager(cm *ConfigManager) {
	d.configManager = cm
}

// getProjectLock gets or creates a lock for a project
func (d *Deployer) getProjectLock(projectPath string) *sync.Mutex {
	d.locksMu.Lock()
	defer d.locksMu.Unlock()

	if lock, exists := d.locks[projectPath]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	d.locks[projectPath] = lock
	return lock
}

// HasActiveBuilds returns true if there are any active builds in progress
func (d *Deployer) HasActiveBuilds() bool {
	return atomic.LoadInt32(&d.activeBuilds) > 0
}

// Deploy executes a deployment for the given project
func (d *Deployer) Deploy(ctx context.Context, project *ProjectConfig, triggerSource string) DeployResult {
	result := DeployResult{
		StartTime: time.Now(),
	}

	// Get project lock
	lock := d.getProjectLock(project.WebhookPath)

	// Try to acquire lock (non-blocking)
	if !lock.TryLock() {
		result.Skipped = true
		result.EndTime = time.Now()
		if d.logger != nil {
			d.logger.Warnf(project.Name, "Skipped - deployment already in progress")
		}
		return result
	}
	
	// Create a build logger for this deployment
	var buildLogger *BuildLogger
	if d.logger != nil {
		buildLogger = d.logger.NewBuildLogger(project.Name)
	}
	
	defer func() {
		// Close the build logger with the result status
		if buildLogger != nil {
			buildLogger.Close(result.Success && !result.Skipped)
		}
		lock.Unlock()
		// Track active builds and process pending reload when all builds complete
		if atomic.AddInt32(&d.activeBuilds, -1) == 0 && d.configManager != nil {
			d.configManager.ProcessPendingReload()
		}
	}()

	// Increment active builds counter
	atomic.AddInt32(&d.activeBuilds, 1)

	// Log to both service logger and build logger
	if d.logger != nil {
		d.logger.Infof(project.Name, "Starting deployment (trigger: %s)", triggerSource)
	}
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Starting deployment (trigger: %s)", triggerSource)
	}

	// Log build config
	d.logBuildConfig(project, buildLogger)

	// Run preflight checks (directory existence, ownership, permissions)
	if err := runPreflightChecks(ctx, project, buildLogger); err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		if buildLogger != nil {
			buildLogger.Errorf(project.Name, "Preflight checks failed: %v", err)
		}
		d.sendNotification(project, &result, triggerSource)
		return result
	}

	// Git operations (if git_repo is configured)
	hasChanges := true // Default to true for non-git projects
	if project.GitRepo != "" {
		var err error
		hasChanges, err = d.handleGitOperations(ctx, project, buildLogger)
		if err != nil {
			result.Error = err.Error()
			result.EndTime = time.Now()
			d.sendNotification(project, &result, triggerSource)
			return result
		}
		
		// If no changes detected, skip build
		if !hasChanges {
			result.Skipped = true
			result.EndTime = time.Now()
			if buildLogger != nil {
				buildLogger.Infof(project.Name, "Build ignored: no changes in the configured branch")
			}
			// Per requirements: no notification should be sent for skipped builds due to no changes
			return result
		}
	} else {
		if buildLogger != nil {
			buildLogger.Infof(project.Name, "No git_repo configured, treating local_path as local directory")
		}
	}

	// Execute deployment command
	output, err := d.executeCommand(ctx, project, triggerSource, buildLogger)
	result.Output = output
	result.EndTime = time.Now()

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if buildLogger != nil {
			buildLogger.Errorf(project.Name, "Deployment failed: %v", err)
			d.logCommandOutput(project.Name, output, true, buildLogger)
		}
	} else {
		result.Success = true
		if buildLogger != nil {
			// Log command output BEFORE "Deployment completed" message
			d.logCommandOutput(project.Name, output, false, buildLogger)
			buildLogger.Infof(project.Name, "Deployment completed in %v", result.Duration())
		}
	}

	d.sendNotification(project, &result, triggerSource)
	return result
}

// logCommandOutput logs the command output if it's not empty
func (d *Deployer) logCommandOutput(projectName, output string, isError bool, buildLogger *BuildLogger) {
	if buildLogger == nil {
		return
	}
	if trimmedOutput := strings.TrimSpace(output); trimmedOutput != "" {
		if isError {
			buildLogger.Errorf(projectName, "Command output: %s", trimmedOutput)
		} else {
			buildLogger.Infof(projectName, "Command output: %s", trimmedOutput)
		}
	}
}

// logBuildConfig logs the project configuration at the start of a build
func (d *Deployer) logBuildConfig(project *ProjectConfig, buildLogger *BuildLogger) {
	if buildLogger == nil {
		return
	}
	// Don't log the actual SSH key path for security - just indicate if one is configured
	sshKeyStatus := "none"
	if project.GitSSHKeyPath != "" {
		sshKeyStatus = "configured"
	}
	buildLogger.Infof(project.Name, "Build config: name=%s, local_path=%s, git_repo=%s, git_branch=%s, git_update=%t, git_ssh_key=%s, execute_path=%s, execute_command=%s",
		project.Name,
		project.LocalPath,
		project.GitRepo,
		project.GitBranch,
		project.GitUpdate,
		sshKeyStatus,
		project.ExecutePath,
		project.ExecuteCommand,
	)
}

// handleGitOperations handles git clone/pull based on configuration
// Returns true if there were changes, false if no changes detected
func (d *Deployer) handleGitOperations(ctx context.Context, project *ProjectConfig, buildLogger *BuildLogger) (bool, error) {
	// Validate SSH key if configured
	if project.GitSSHKeyPath != "" {
		if err := validateSSHKeyPath(project.GitSSHKeyPath); err != nil {
			if buildLogger != nil {
				buildLogger.Errorf(project.Name, "SSH key validation failed: %v", err)
			}
			return false, fmt.Errorf("SSH key validation failed: %v", err)
		}
		if buildLogger != nil {
			buildLogger.Infof(project.Name, "Using SSH key for git operations")
		}
	}

	// Check if local_path exists and is a git repo
	if !isGitRepo(project.LocalPath) {
		// Need to clone
		if err := d.gitClone(ctx, project, buildLogger); err != nil {
			if buildLogger != nil {
				buildLogger.Errorf(project.Name, "Git clone failed: %v", err)
			}
			return false, fmt.Errorf("git clone failed: %v", err)
		}
		if buildLogger != nil {
			buildLogger.Infof(project.Name, "Cloned repository to %s", project.LocalPath)
		}
		
		// After cloning, verify we're on the correct branch
		// (the clone uses --branch flag, but we should verify)
		if err := d.ensureCorrectBranch(ctx, project, buildLogger); err != nil {
			if buildLogger != nil {
				buildLogger.Errorf(project.Name, "Failed to checkout configured branch after clone: %v", err)
			}
			return false, fmt.Errorf("failed to checkout configured branch after clone: %v", err)
		}
		// Clone always brings new code, so consider it as having changes
		return true, nil
	} else {
		if buildLogger != nil {
			buildLogger.Infof(project.Name, "Repository already cloned at %s", project.LocalPath)
		}
		
		// Ensure we're on the correct branch before pulling or executing commands
		if err := d.ensureCorrectBranch(ctx, project, buildLogger); err != nil {
			if buildLogger != nil {
				buildLogger.Errorf(project.Name, "Failed to checkout configured branch: %v", err)
			}
			return false, fmt.Errorf("failed to checkout configured branch: %v", err)
		}
		
		// Check if we should do git pull
		if project.GitUpdate {
			// Get current commit SHA before pull
			beforeSHA, err := getCurrentCommitSHA(ctx, project.LocalPath)
			if err != nil {
				if buildLogger != nil {
					buildLogger.Warnf(project.Name, "Failed to get commit SHA before pull: %v", err)
				}
				// Continue with pull even if we can't get SHA
				beforeSHA = ""
			}
			
			if err := d.gitPull(ctx, project, buildLogger); err != nil {
				if buildLogger != nil {
					buildLogger.Errorf(project.Name, "Git pull failed: %v", err)
				}
				return false, fmt.Errorf("git pull failed: %v", err)
			}
			if buildLogger != nil {
				buildLogger.Infof(project.Name, "Executed git pull")
			}
			
			// Get current commit SHA after pull
			afterSHA, err := getCurrentCommitSHA(ctx, project.LocalPath)
			if err != nil {
				if buildLogger != nil {
					buildLogger.Warnf(project.Name, "Failed to get commit SHA after pull: %v", err)
				}
				// If we can't determine, assume there were changes to be safe
				return true, nil
			}
			
			// Check if there were changes
			hasChanges := beforeSHA != afterSHA
			if buildLogger != nil {
				if hasChanges {
					buildLogger.Infof(project.Name, "Changes detected: %s -> %s", truncateSHA(beforeSHA), truncateSHA(afterSHA))
				} else {
					buildLogger.Infof(project.Name, "No changes detected (commit: %s)", truncateSHA(afterSHA))
				}
			}
			return hasChanges, nil
		} else {
			if buildLogger != nil {
				buildLogger.Infof(project.Name, "git_update is false, skipping git pull")
			}
			// If not pulling, assume there are changes (or at least proceed with build)
			return true, nil
		}
	}
}

// isGitRepo checks if the given path is a git repository
func isGitRepo(path string) bool {
	if path == "" {
		return false
	}
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// getCurrentBranch returns the current git branch for a repository
func getCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	// Use exec.Command directly with separate arguments for consistency and security
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(output))
	}

	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// getCurrentCommitSHA returns the current commit SHA for a repository
func getCurrentCommitSHA(ctx context.Context, repoPath string) (string, error) {
	// Use exec.Command directly with separate arguments for consistency and security
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, string(output))
	}

	sha := strings.TrimSpace(string(output))
	return sha, nil
}

// truncateSHA safely truncates a commit SHA to 8 characters for logging
func truncateSHA(sha string) string {
	if len(sha) < 8 {
		return sha
	}
	return sha[:8]
}

// isValidGitRepo checks if the path is a valid git repository by running a git command
func isValidGitRepo(ctx context.Context, repoPath string) bool {
	if !isGitRepo(repoPath) {
		return false
	}
	// Use a lighter git command to verify repository validity
	// Use exec.Command directly with separate arguments for consistency and security
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = repoPath
	err := cmd.Run()
	return err == nil
}

// ensureCorrectBranch verifies and checks out the configured branch if needed
func (d *Deployer) ensureCorrectBranch(ctx context.Context, project *ProjectConfig, buildLogger *BuildLogger) error {
	// Verify it's a valid git repository first
	if !isValidGitRepo(ctx, project.LocalPath) {
		if buildLogger != nil {
			buildLogger.Warnf(project.Name, "Directory has .git but is not a valid git repository, skipping branch checkout")
		}
		return nil
	}

	// Get current branch
	currentBranch, err := getCurrentBranch(ctx, project.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %v", err)
	}

	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Current branch: %s, configured branch: %s", currentBranch, project.GitBranch)
	}

	// If already on the correct branch, nothing to do
	if currentBranch == project.GitBranch {
		if buildLogger != nil {
			buildLogger.Infof(project.Name, "Already on correct branch: %s", currentBranch)
		}
		return nil
	}

	// Need to checkout the configured branch
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Checking out branch: %s", project.GitBranch)
	}

	if err := d.gitCheckout(ctx, project, buildLogger); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %v", project.GitBranch, err)
	}

	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Successfully checked out branch: %s", project.GitBranch)
	}

	return nil
}

// gitCheckout checks out the configured branch
func (d *Deployer) gitCheckout(ctx context.Context, project *ProjectConfig, buildLogger *BuildLogger) error {
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Running: git checkout %s", project.GitBranch)
	}

	// Use exec.Command directly with separate arguments to avoid shell injection
	// Even though branch name is validated, this is an extra layer of protection
	cmd := exec.CommandContext(ctx, "git", "checkout", project.GitBranch)
	setProcessGroup(cmd)
	cmd.Dir = project.LocalPath

	// Set GIT_SSH_COMMAND if git_ssh_key_path is configured
	if project.GitSSHKeyPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", buildGitSSHCommand(project.GitSSHKeyPath)))
	}

	output, err := cmd.CombinedOutput()

	if buildLogger != nil && len(output) > 0 {
		buildLogger.Infof(project.Name, "Output: %s", strings.TrimSpace(string(output)))
	}

	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// buildGitSSHCommand creates the SSH command string for git operations
func buildGitSSHCommand(sshKeyPath string) string {
	return fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new -o IdentitiesOnly=yes", sshKeyPath)
}

// gitClone clones a git repository to the specified local path
func (d *Deployer) gitClone(ctx context.Context, project *ProjectConfig, buildLogger *BuildLogger) error {
	// Create parent directories if they don't exist
	parentDir := filepath.Dir(project.LocalPath)
	if err := ensureParentDirExists(ctx, parentDir, buildLogger, project.Name); err != nil {
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	gitCmd := fmt.Sprintf("git clone --branch %s %s %s", project.GitBranch, project.GitRepo, project.LocalPath)
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Running: %s", gitCmd)
	}

	// Build the command
	cmd := buildCommand(ctx, gitCmd)

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	// Set GIT_SSH_COMMAND if git_ssh_key_path is configured
	if project.GitSSHKeyPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", buildGitSSHCommand(project.GitSSHKeyPath)))
	}

	output, err := cmd.CombinedOutput()

	if buildLogger != nil && len(output) > 0 {
		buildLogger.Infof(project.Name, "Output: %s", strings.TrimSpace(string(output)))
	}

	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// gitPull executes git pull in the project's local path
func (d *Deployer) gitPull(ctx context.Context, project *ProjectConfig, buildLogger *BuildLogger) error {
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Running: git pull")
		buildLogger.Infof(project.Name, "Path: %s", project.LocalPath)
	}

	// Build the command
	cmd := buildCommand(ctx, "git pull")

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	cmd.Dir = project.LocalPath

	// Set GIT_SSH_COMMAND if git_ssh_key_path is configured
	if project.GitSSHKeyPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", buildGitSSHCommand(project.GitSSHKeyPath)))
	}

	output, err := cmd.CombinedOutput()

	if buildLogger != nil && len(output) > 0 {
		buildLogger.Infof(project.Name, "Output: %s", strings.TrimSpace(string(output)))
	}

	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// executeCommand runs the deployment command
func (d *Deployer) executeCommand(ctx context.Context, project *ProjectConfig, triggerSource string, buildLogger *BuildLogger) (string, error) {
	// Create context with timeout if configured
	var cancel context.CancelFunc
	if project.TimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(project.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// Log the command being executed with path
	// Get effective execute_path (defaults to local_path if not set)
	executePath := getEffectiveExecutePath(project.LocalPath, project.ExecutePath)
	if executePath == "" {
		executePath = "."
	}
	if buildLogger != nil {
		buildLogger.Infof(project.Name, "Executing command:")
		buildLogger.Infof(project.Name, "  Path: %s", executePath)
		buildLogger.Infof(project.Name, "  Command: %s", project.ExecuteCommand)
	}

	// Build the command
	cmd := buildCommand(ctx, project.ExecuteCommand)

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	// Set working directory to effective execute_path
	if executePath != "." {
		cmd.Dir = executePath
	}

	// Set environment variables
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SDEPLOY_PROJECT_NAME=%s", project.Name),
		fmt.Sprintf("SDEPLOY_TRIGGER_SOURCE=%s", triggerSource),
		fmt.Sprintf("SDEPLOY_GIT_BRANCH=%s", project.GitBranch),
	)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Wait for command completion or context cancellation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Kill the entire process group
		killProcessGroup(cmd)
		<-done // Wait for the process to actually exit
		return stdout.String() + stderr.String(), fmt.Errorf("command timed out after %d seconds", project.TimeoutSeconds)
	case err := <-done:
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}
		return output, err
	}
}

// sendNotification sends email notification if configured
func (d *Deployer) sendNotification(project *ProjectConfig, result *DeployResult, triggerSource string) {
	if d.notifier == nil {
		return
	}

	if err := d.notifier.SendNotification(project, result, triggerSource); err != nil {
		if d.logger != nil {
			d.logger.Errorf(project.Name, "Failed to send email notification: %v", err)
		}
	}
}
