package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	defer func() {
		lock.Unlock()
		// Track active builds and process pending reload when all builds complete
		if atomic.AddInt32(&d.activeBuilds, -1) == 0 && d.configManager != nil {
			d.configManager.ProcessPendingReload()
		}
	}()

	// Increment active builds counter
	atomic.AddInt32(&d.activeBuilds, 1)

	if d.logger != nil {
		d.logger.Infof(project.Name, "Starting deployment (trigger: %s)", triggerSource)
	}

	// Log build config
	d.logBuildConfig(project)

	// Git operations (if git_repo is configured)
	if project.GitRepo != "" {
		if err := d.handleGitOperations(ctx, project); err != nil {
			result.Error = err.Error()
			result.EndTime = time.Now()
			d.sendNotification(project, &result, triggerSource)
			return result
		}
	} else {
		if d.logger != nil {
			d.logger.Infof(project.Name, "No git_repo configured, treating local_path as local directory")
		}
	}

	// Execute deployment command
	output, err := d.executeCommand(ctx, project, triggerSource)
	result.Output = output
	result.EndTime = time.Now()

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if d.logger != nil {
			d.logger.Errorf(project.Name, "Deployment failed: %v", err)
			d.logCommandOutput(project.Name, output, true)
		}
	} else {
		result.Success = true
		if d.logger != nil {
			// Log command output BEFORE "Deployment completed" message
			d.logCommandOutput(project.Name, output, false)
			d.logger.Infof(project.Name, "Deployment completed in %v", result.Duration())
		}
	}

	d.sendNotification(project, &result, triggerSource)
	return result
}

// logCommandOutput logs the command output if it's not empty
func (d *Deployer) logCommandOutput(projectName, output string, isError bool) {
	if d.logger == nil {
		return
	}
	if trimmedOutput := strings.TrimSpace(output); trimmedOutput != "" {
		if isError {
			d.logger.Errorf(projectName, "Command output: %s", trimmedOutput)
		} else {
			d.logger.Infof(projectName, "Command output: %s", trimmedOutput)
		}
	}
}

// logBuildConfig logs the project configuration at the start of a build
func (d *Deployer) logBuildConfig(project *ProjectConfig) {
	if d.logger == nil {
		return
	}
	d.logger.Infof(project.Name, "Build config: name=%s, local_path=%s, git_repo=%s, git_branch=%s, git_update=%t, execute_path=%s, execute_command=%s",
		project.Name,
		project.LocalPath,
		project.GitRepo,
		project.GitBranch,
		project.GitUpdate,
		project.ExecutePath,
		project.ExecuteCommand,
	)
}

// getEffectiveRunAs returns the effective run_as_user and run_as_group for a project
// If not configured in the project, defaults to www-data:www-data
func getEffectiveRunAs(project *ProjectConfig) (string, string) {
	runAsUser := project.RunAsUser
	if runAsUser == "" {
		runAsUser = "www-data"
	}
	runAsGroup := project.RunAsGroup
	if runAsGroup == "" {
		runAsGroup = "www-data"
	}
	return runAsUser, runAsGroup
}

// handleGitOperations handles git clone/pull based on configuration
func (d *Deployer) handleGitOperations(ctx context.Context, project *ProjectConfig) error {
	// Get effective user/group for running git commands (same as execute_command)
	runAsUser, runAsGroup := getEffectiveRunAs(project)

	// Check if local_path exists and is a git repo
	if !isGitRepo(project.LocalPath) {
		// Need to clone
		if err := d.gitClone(ctx, project.Name, project.GitRepo, project.LocalPath, project.GitBranch, runAsUser, runAsGroup); err != nil {
			if d.logger != nil {
				d.logger.Errorf(project.Name, "Git clone failed: %v", err)
			}
			return fmt.Errorf("git clone failed: %v", err)
		}
		if d.logger != nil {
			d.logger.Infof(project.Name, "Cloned repository to %s", project.LocalPath)
		}
	} else {
		if d.logger != nil {
			d.logger.Infof(project.Name, "Repository already cloned at %s", project.LocalPath)
		}
		// Check if we should do git pull
		if project.GitUpdate {
			if err := d.gitPull(ctx, project, runAsUser, runAsGroup); err != nil {
				if d.logger != nil {
					d.logger.Errorf(project.Name, "Git pull failed: %v", err)
				}
				return fmt.Errorf("git pull failed: %v", err)
			}
			if d.logger != nil {
				d.logger.Infof(project.Name, "Executed git pull")
			}
		} else {
			if d.logger != nil {
				d.logger.Infof(project.Name, "git_update is false, skipping git pull")
			}
		}
	}
	return nil
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

// gitClone clones a git repository to the specified local path
func (d *Deployer) gitClone(ctx context.Context, projectName, repoURL, localPath, branch, runAsUser, runAsGroup string) error {
	// Create parent directories if they don't exist (as root if running as root)
	// The parent directory needs to exist and be writable by the target user
	parentDir := filepath.Dir(localPath)
	if err := ensureParentDirExists(ctx, parentDir, runAsUser, runAsGroup, d.logger, projectName); err != nil {
		return fmt.Errorf("failed to create parent directory: %v", err)
	}

	gitCmd := fmt.Sprintf("git clone --branch %s %s %s", branch, repoURL, localPath)
	if d.logger != nil {
		d.logger.Infof(projectName, "Running: %s", gitCmd)
		d.logger.Infof(projectName, "Run As: %s:%s", runAsUser, runAsGroup)
	}

	// Build the command with user/group support
	cmd, warning := buildCommand(ctx, gitCmd, runAsUser, runAsGroup)
	if warning != "" && d.logger != nil {
		d.logger.Warnf(projectName, "%s", warning)
	}

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	output, err := cmd.CombinedOutput()
	
	if d.logger != nil && len(output) > 0 {
		d.logger.Infof(projectName, "Output: %s", strings.TrimSpace(string(output)))
	}
	
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// gitPull executes git pull in the project's local path
func (d *Deployer) gitPull(ctx context.Context, project *ProjectConfig, runAsUser, runAsGroup string) error {
	if d.logger != nil {
		d.logger.Infof(project.Name, "Running: git pull")
		d.logger.Infof(project.Name, "Path: %s", project.LocalPath)
		d.logger.Infof(project.Name, "Run As: %s:%s", runAsUser, runAsGroup)
	}

	// Build the command with user/group support
	cmd, warning := buildCommand(ctx, "git pull", runAsUser, runAsGroup)
	if warning != "" && d.logger != nil {
		d.logger.Warnf(project.Name, "%s", warning)
	}

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	cmd.Dir = project.LocalPath

	output, err := cmd.CombinedOutput()
	
	if d.logger != nil && len(output) > 0 {
		d.logger.Infof(project.Name, "Output: %s", strings.TrimSpace(string(output)))
	}
	
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// executeCommand runs the deployment command
func (d *Deployer) executeCommand(ctx context.Context, project *ProjectConfig, triggerSource string) (string, error) {
	// Create context with timeout if configured
	var cancel context.CancelFunc
	if project.TimeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(project.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	// Log the command being executed with path
	executePath := project.ExecutePath
	if executePath == "" {
		executePath = "."
	}
	if d.logger != nil {
		d.logger.Infof(project.Name, "Executing command:")
		d.logger.Infof(project.Name, "  Path: %s", executePath)
		d.logger.Infof(project.Name, "  Command: %s", project.ExecuteCommand)
	}

	// Get effective user/group for running the command
	runAsUser, runAsGroup := getEffectiveRunAs(project)

	// Build the command with user/group support
	cmd, warning := buildCommand(ctx, project.ExecuteCommand, runAsUser, runAsGroup)
	if warning != "" && d.logger != nil {
		d.logger.Warnf(project.Name, "%s", warning)
	}

	// Set process group so we can kill all child processes
	setProcessGroup(cmd)

	// Set working directory if configured
	if project.ExecutePath != "" {
		cmd.Dir = project.ExecutePath
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
