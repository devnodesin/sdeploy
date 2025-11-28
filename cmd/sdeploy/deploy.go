package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
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
	logger    *Logger
	locks     map[string]*sync.Mutex
	locksMu   sync.Mutex
	notifier  *EmailNotifier
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
	defer lock.Unlock()

	if d.logger != nil {
		d.logger.Infof(project.Name, "Starting deployment (trigger: %s)", triggerSource)
	}

	// Git pull if configured
	if project.GitUpdate && project.GitPath != "" {
		if err := d.gitPull(ctx, project); err != nil {
			result.Error = fmt.Sprintf("git pull failed: %v", err)
			result.EndTime = time.Now()
			if d.logger != nil {
				d.logger.Errorf(project.Name, "Git pull failed: %v", err)
			}
			d.sendNotification(project, &result, triggerSource)
			return result
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
		}
	} else {
		result.Success = true
		if d.logger != nil {
			d.logger.Infof(project.Name, "Deployment completed in %v", result.Duration())
		}
	}

	d.sendNotification(project, &result, triggerSource)
	return result
}

// gitPull executes git pull in the project's git path
func (d *Deployer) gitPull(ctx context.Context, project *ProjectConfig) error {
	if d.logger != nil {
		d.logger.Infof(project.Name, "Executing git pull in %s", project.GitPath)
	}

	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = project.GitPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	if d.logger != nil {
		d.logger.Infof(project.Name, "Git pull output: %s", string(output))
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

	cmd := exec.CommandContext(ctx, "sh", "-c", project.ExecuteCommand)

	// Set process group so we can kill all child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
		if cmd.Process != nil {
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
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

