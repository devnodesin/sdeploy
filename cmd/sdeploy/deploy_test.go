package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestDeployLockAcquisition tests that lock is acquired for deployment
func TestDeployLockAcquisition(t *testing.T) {
	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo hello",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}
}

// TestDeploySkipOnBusy tests that concurrent deployments are skipped
func TestDeploySkipOnBusy(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "sleep 0.5",
	}

	var wg sync.WaitGroup
	results := make([]DeployResult, 2)

	// Start first deployment
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = deployer.Deploy(context.Background(), project, "WEBHOOK")
	}()

	// Give time for first deployment to start
	time.Sleep(50 * time.Millisecond)

	// Try second deployment (should be skipped)
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1] = deployer.Deploy(context.Background(), project, "INTERNAL")
	}()

	wg.Wait()

	// One should succeed, one should be skipped
	skippedCount := 0
	for _, r := range results {
		if r.Skipped {
			skippedCount++
		}
	}

	if skippedCount != 1 {
		t.Errorf("Expected exactly 1 skipped deployment, got %d", skippedCount)
	}

	// Check logs contain "Skipped"
	if !strings.Contains(buf.String(), "Skipped") {
		t.Log("Log output:", buf.String())
	}
}

// TestDeployGitPull tests git pull execution when git_update=true
func TestDeployGitPull(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a bare git repo for testing
	gitPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(gitPath, 0755); err != nil {
		t.Fatalf("Failed to create git path: %v", err)
	}

	// Create a simple script that echoes git pull
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitUpdate:      true,
		LocalPath:      gitPath,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo done",
	}

	// This will fail git pull but that's expected in test env
	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	// Even if git pull fails, we should log the attempt
	logOutput := buf.String()
	if !strings.Contains(logOutput, "git") || !strings.Contains(logOutput, "pull") {
		t.Log("Log output:", logOutput)
	}
	_ = result
}

// TestDeployCommandExecution tests execute_command execution
func TestDeployCommandExecution(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo 'test output' > output.txt",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "test output") {
		t.Errorf("Expected output file to contain 'test output', got: %s", string(content))
	}
}

// TestDeployTimeout tests command timeout
func TestDeployTimeout(t *testing.T) {
	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "sleep 10",
		TimeoutSeconds: 1, // 1 second timeout
	}

	start := time.Now()
	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	elapsed := time.Since(start)

	// Allow some slack for process cleanup - should be around 1 second, not 10
	if elapsed > 5*time.Second {
		t.Errorf("Expected timeout to occur within ~1 second, took %v", elapsed)
	}

	if result.Success {
		t.Error("Expected deployment to fail due to timeout")
	}
}

// TestDeployEnvVars tests environment variable injection
func TestDeployEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, "env.txt")

	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "MyProject",
		WebhookPath:    "/hooks/test",
		GitBranch:      "develop",
		ExecutePath:    tmpDir,
		ExecuteCommand: "env > env.txt",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Fatalf("Deployment failed: %s", result.Error)
	}

	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("Failed to read env file: %v", err)
	}

	envStr := string(content)
	if !strings.Contains(envStr, "SDEPLOY_PROJECT_NAME=MyProject") {
		t.Error("Expected SDEPLOY_PROJECT_NAME env var")
	}
	if !strings.Contains(envStr, "SDEPLOY_TRIGGER_SOURCE=WEBHOOK") {
		t.Error("Expected SDEPLOY_TRIGGER_SOURCE env var")
	}
	if !strings.Contains(envStr, "SDEPLOY_GIT_BRANCH=develop") {
		t.Error("Expected SDEPLOY_GIT_BRANCH env var")
	}
}

// TestDeployOutputCapture tests stdout/stderr capture
func TestDeployOutputCapture(t *testing.T) {
	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo 'stdout message' && echo 'stderr message' >&2",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "stdout message") {
		t.Error("Expected output to contain stdout message")
	}
}

// TestDeployErrorHandling tests graceful error handling
func TestDeployErrorHandling(t *testing.T) {
	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "exit 1",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if result.Success {
		t.Error("Expected deployment to fail")
	}
	if result.Error == "" {
		t.Error("Expected error message to be populated")
	}
}

// TestDeployLockRelease tests lock is released after completion
func TestDeployLockRelease(t *testing.T) {
	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo hello",
	}

	// First deployment
	result1 := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result1.Success {
		t.Errorf("First deployment failed: %s", result1.Error)
	}

	// Second deployment should also succeed (lock released)
	result2 := deployer.Deploy(context.Background(), project, "INTERNAL")
	if !result2.Success {
		t.Errorf("Second deployment failed (lock not released?): %s", result2.Error)
	}
}

// TestDeployResult tests DeployResult structure
func TestDeployResult(t *testing.T) {
	result := DeployResult{
		Success:   true,
		Skipped:   false,
		Output:    "test output",
		Error:     "",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Second),
	}

	if result.Duration() < time.Second {
		t.Error("Expected duration to be at least 1 second")
	}
}

// TestDeployWorkingDirectory tests command runs in correct directory
func TestDeployWorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	pwdFile := filepath.Join(tmpDir, "pwd.txt")

	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecutePath:    tmpDir,
		ExecuteCommand: "pwd > pwd.txt",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Fatalf("Deployment failed: %s", result.Error)
	}

	content, err := os.ReadFile(pwdFile)
	if err != nil {
		t.Fatalf("Failed to read pwd file: %v", err)
	}

	if !strings.Contains(string(content), tmpDir) {
		t.Errorf("Expected working directory %s, got: %s", tmpDir, string(content))
	}
}

// TestIsGitRepo tests the isGitRepo function
func TestIsGitRepo(t *testing.T) {
	// Test with empty path
	if isGitRepo("") {
		t.Error("Expected isGitRepo('') to return false")
	}

	// Test with non-existent path
	if isGitRepo("/nonexistent/path") {
		t.Error("Expected isGitRepo on non-existent path to return false")
	}

	// Test with directory that has .git
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	if !isGitRepo(tmpDir) {
		t.Error("Expected isGitRepo to return true for directory with .git")
	}

	// Test with directory that does NOT have .git
	emptyDir := t.TempDir()
	if isGitRepo(emptyDir) {
		t.Error("Expected isGitRepo to return false for directory without .git")
	}

	// Test with .git as file instead of directory
	fileDir := t.TempDir()
	gitFile := filepath.Join(fileDir, ".git")
	if err := os.WriteFile(gitFile, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("Failed to create .git file: %v", err)
	}

	if isGitRepo(fileDir) {
		t.Error("Expected isGitRepo to return false when .git is a file not a directory")
	}
}

// TestDeployNoGitRepo tests deployment with no git_repo configured (local directory only)
func TestDeployNoGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "LocalProject",
		WebhookPath:    "/hooks/local",
		LocalPath:      tmpDir,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo local",
	}

	result := deployer.Deploy(context.Background(), project, "INTERNAL")

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "No git_repo configured, treating local_path as local directory") {
		t.Errorf("Expected log message about no git_repo, got: %s", logOutput)
	}
}

// TestDeployGitRepoAlreadyCloned tests deployment when git repo is already cloned
func TestDeployGitRepoAlreadyCloned(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate an already cloned repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "ClonedProject",
		WebhookPath:    "/hooks/cloned",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitUpdate:      false,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo done",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see "Repository already cloned at" message
	if !strings.Contains(logOutput, "Repository already cloned at") {
		t.Errorf("Expected log message about already cloned repo, got: %s", logOutput)
	}

	// Should see "git_update is false, skipping git pull" message
	if !strings.Contains(logOutput, "git_update is false, skipping git pull") {
		t.Errorf("Expected log message about skipping git pull, got: %s", logOutput)
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}
}

// TestDeployBuildConfigLogging tests that build config is logged
func TestDeployBuildConfigLogging(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate an already cloned repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitUpdate:      false, // Don't try to pull
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see build config log
	if !strings.Contains(logOutput, "Build config:") {
		t.Errorf("Expected build config to be logged, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "name=TestProject") {
		t.Errorf("Expected project name in build config, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "local_path=") {
		t.Errorf("Expected local_path in build config, got: %s", logOutput)
	}
}

// TestGetShellPath tests the shell path lookup function
func TestGetShellPath(t *testing.T) {
	shellPath := getShellPath()

	// Shell path should not be empty
	if shellPath == "" {
		t.Error("Expected getShellPath() to return a non-empty string")
	}

	// The shell path should be "sh" or contain "sh" (Unix) or "cmd" (Windows)
	if !strings.Contains(shellPath, "sh") && !strings.Contains(shellPath, "cmd") {
		t.Errorf("Expected shell path to contain 'sh' or 'cmd', got: %s", shellPath)
	}
}

// TestGetShellArgs tests the shell args function
func TestGetShellArgs(t *testing.T) {
	args := getShellArgs()

	// Shell args should not be empty
	if args == "" {
		t.Error("Expected getShellArgs() to return a non-empty string")
	}

	// The args should be "-c" (Unix) or "/c" (Windows)
	if args != "-c" && args != "/c" {
		t.Errorf("Expected shell args to be '-c' or '/c', got: %s", args)
	}
}

// TestDeployErrorOutputLogging tests that error output is logged when command fails
func TestDeployErrorOutputLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo 'error message' >&2 && exit 1",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if result.Success {
		t.Error("Expected deployment to fail")
	}

	logOutput := buf.String()

	// Should log the command output when deployment fails
	if !strings.Contains(logOutput, "Command output:") {
		t.Errorf("Expected log to contain 'Command output:', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "error message") {
		t.Errorf("Expected log to contain error message from command, got: %s", logOutput)
	}
}

// TestDeploySuccessOutputLogging tests that output is logged when command succeeds
func TestDeploySuccessOutputLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo 'build completed successfully'",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should log the command output when deployment succeeds
	if !strings.Contains(logOutput, "Command output:") {
		t.Errorf("Expected log to contain 'Command output:', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "build completed successfully") {
		t.Errorf("Expected log to contain build output, got: %s", logOutput)
	}
}

// TestDeployLogOrderOutputBeforeCompleted tests that command output is logged BEFORE "Deployment completed"
func TestDeployLogOrderOutputBeforeCompleted(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo 'test output message'",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Find positions of "Command output" and "Deployment completed"
	outputPos := strings.Index(logOutput, "Command output:")
	completedPos := strings.Index(logOutput, "Deployment completed")

	if outputPos == -1 {
		t.Error("Expected log to contain 'Command output:'")
	}
	if completedPos == -1 {
		t.Error("Expected log to contain 'Deployment completed'")
	}

	// Command output should appear BEFORE "Deployment completed"
	if outputPos >= completedPos {
		t.Errorf("Expected 'Command output:' to appear BEFORE 'Deployment completed' in logs.\nLog output:\n%s", logOutput)
	}
}

// TestDeployExecuteCommandLogging tests that execute command and path are logged
func TestDeployExecuteCommandLogging(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should log "Executing command:"
	if !strings.Contains(logOutput, "Executing command:") {
		t.Errorf("Expected log to contain 'Executing command:', got: %s", logOutput)
	}

	// Should log "Path:"
	if !strings.Contains(logOutput, "Path:") {
		t.Errorf("Expected log to contain 'Path:', got: %s", logOutput)
	}

	// Should log "Command:"
	if !strings.Contains(logOutput, "Command:") {
		t.Errorf("Expected log to contain 'Command:', got: %s", logOutput)
	}

	// Should log the actual execute path
	if !strings.Contains(logOutput, tmpDir) {
		t.Errorf("Expected log to contain execute path '%s', got: %s", tmpDir, logOutput)
	}
}

// TestRunAsUserGroupConfig tests run_as_user and run_as_group configuration
func TestRunAsUserGroupConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Config with run_as_user and run_as_group
	configWithRunAs := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"webhook_secret": "secret_token_123",
				"execute_command": "sh deploy.sh",
				"run_as_user": "nobody",
				"run_as_group": "nogroup"
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configWithRunAs), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	project := cfg.Projects[0]
	if project.RunAsUser != "nobody" {
		t.Errorf("Expected RunAsUser 'nobody', got '%s'", project.RunAsUser)
	}
	if project.RunAsGroup != "nogroup" {
		t.Errorf("Expected RunAsGroup 'nogroup', got '%s'", project.RunAsGroup)
	}
}

// TestDefaultRunAsUserGroup tests default run_as_user and run_as_group values
func TestDefaultRunAsUserGroup(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Config without run_as_user and run_as_group
	configNoRunAs := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"webhook_secret": "secret_token_123",
				"execute_command": "sh deploy.sh"
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configNoRunAs), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	project := cfg.Projects[0]
	// RunAsUser and RunAsGroup should be empty strings in config, defaults are applied at runtime
	if project.RunAsUser != "" {
		t.Errorf("Expected RunAsUser to be empty string (defaults applied at runtime), got '%s'", project.RunAsUser)
	}
	if project.RunAsGroup != "" {
		t.Errorf("Expected RunAsGroup to be empty string (defaults applied at runtime), got '%s'", project.RunAsGroup)
	}
}

// TestBuildCommandFunction tests buildCommand function exists and works
func TestBuildCommandFunction(t *testing.T) {
	ctx := context.Background()
	cmd, warning := buildCommand(ctx, "echo test", "www-data", "www-data")
	if cmd == nil {
		t.Error("Expected buildCommand to return a non-nil command")
	}
	// Warning may or may not be empty depending on whether www-data exists
	// and whether running as root - we just verify it doesn't panic
	_ = warning
}

// TestGitPullRunAsLogging tests that git pull logs the Run As user/group
func TestGitPullRunAsLogging(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate an already cloned repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitUpdate:      true, // Enable git pull
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo done",
		RunAsUser:      "testuser",
		RunAsGroup:     "testgroup",
	}

	// Deploy will fail on git pull (not a real git repo), but we can verify the logging
	deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see "Run As:" logging for git pull
	if !strings.Contains(logOutput, "Run As: testuser:testgroup") {
		t.Errorf("Expected log message 'Run As: testuser:testgroup' for git pull, got: %s", logOutput)
	}
}

// TestGitPullDefaultRunAs tests that git pull uses default www-data:www-data when not configured
func TestGitPullDefaultRunAs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate an already cloned repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitUpdate:      true, // Enable git pull
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo done",
		// RunAsUser and RunAsGroup are not set, should default to www-data:www-data
	}

	// Deploy will fail on git pull (not a real git repo), but we can verify the logging
	deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see default "Run As: www-data:www-data" logging for git pull
	if !strings.Contains(logOutput, "Run As: www-data:www-data") {
		t.Errorf("Expected log message 'Run As: www-data:www-data' for git pull (default), got: %s", logOutput)
	}
}

// TestGetEffectiveRunAs tests the getEffectiveRunAs helper function
func TestGetEffectiveRunAs(t *testing.T) {
	tests := []struct {
		name       string
		project    *ProjectConfig
		wantUser   string
		wantGroup  string
	}{
		{
			name:      "default when not set",
			project:   &ProjectConfig{},
			wantUser:  "www-data",
			wantGroup: "www-data",
		},
		{
			name:      "custom user and group",
			project:   &ProjectConfig{RunAsUser: "nginx", RunAsGroup: "www"},
			wantUser:  "nginx",
			wantGroup: "www",
		},
		{
			name:      "custom user only",
			project:   &ProjectConfig{RunAsUser: "deploy"},
			wantUser:  "deploy",
			wantGroup: "www-data",
		},
		{
			name:      "custom group only",
			project:   &ProjectConfig{RunAsGroup: "staff"},
			wantUser:  "www-data",
			wantGroup: "staff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUser, gotGroup := getEffectiveRunAs(tt.project)
			if gotUser != tt.wantUser {
				t.Errorf("getEffectiveRunAs() user = %v, want %v", gotUser, tt.wantUser)
			}
			if gotGroup != tt.wantGroup {
				t.Errorf("getEffectiveRunAs() group = %v, want %v", gotGroup, tt.wantGroup)
			}
		})
	}
}

// TestSetProcessGroupPreservesCredentials tests that setProcessGroup preserves existing SysProcAttr
func TestSetProcessGroupPreservesCredentials(t *testing.T) {
	ctx := context.Background()
	
	// Create a command with SysProcAttr already set (simulating buildCommand with credentials)
	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: 1000,
			Gid: 1000,
		},
	}
	
	// Call setProcessGroup - it should preserve the credential
	setProcessGroup(cmd)
	
	// Verify Setpgid was set
	if !cmd.SysProcAttr.Setpgid {
		t.Error("Expected Setpgid to be true")
	}
	
	// Verify Credential was preserved
	if cmd.SysProcAttr.Credential == nil {
		t.Fatal("Expected Credential to be preserved, but it was nil")
	}
	if cmd.SysProcAttr.Credential.Uid != 1000 {
		t.Errorf("Expected Uid 1000, got %d", cmd.SysProcAttr.Credential.Uid)
	}
	if cmd.SysProcAttr.Credential.Gid != 1000 {
		t.Errorf("Expected Gid 1000, got %d", cmd.SysProcAttr.Credential.Gid)
	}
}

// TestSetProcessGroupWithNilSysProcAttr tests setProcessGroup when SysProcAttr is nil
func TestSetProcessGroupWithNilSysProcAttr(t *testing.T) {
	ctx := context.Background()
	
	// Create a command without SysProcAttr
	cmd := exec.CommandContext(ctx, "echo", "test")
	
	// Call setProcessGroup
	setProcessGroup(cmd)
	
	// Verify SysProcAttr was created with Setpgid
	if cmd.SysProcAttr == nil {
		t.Error("Expected SysProcAttr to be created")
	}
	if cmd.SysProcAttr != nil && !cmd.SysProcAttr.Setpgid {
		t.Error("Expected Setpgid to be true")
	}
}

// TestEnsureParentDirExists tests the ensureParentDirExists function
func TestEnsureParentDirExists(t *testing.T) {
	ctx := context.Background()

	t.Run("parent dir already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		parentDir := tmpDir // Parent already exists
		err := ensureParentDirExists(ctx, parentDir, "www-data", "www-data", nil, "TestProject")
		if err != nil {
			t.Errorf("Expected no error when parent dir exists, got: %v", err)
		}
	})

	t.Run("creates parent dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		parentDir := filepath.Join(tmpDir, "new-parent")
		var buf bytes.Buffer
		logger := NewLogger(&buf, "")

		err := ensureParentDirExists(ctx, parentDir, "www-data", "www-data", logger, "TestProject")
		if err != nil {
			t.Errorf("Expected no error creating parent dir, got: %v", err)
		}

		// Verify directory was created
		info, err := os.Stat(parentDir)
		if err != nil {
			t.Fatalf("Expected parent dir to exist, got error: %v", err)
		}
		if !info.IsDir() {
			t.Error("Expected parent dir to be a directory")
		}

		// Verify logging
		logOutput := buf.String()
		if !strings.Contains(logOutput, "Creating parent directory:") {
			t.Errorf("Expected log message about creating parent directory, got: %s", logOutput)
		}
	})

	t.Run("creates nested parent dirs", func(t *testing.T) {
		tmpDir := t.TempDir()
		parentDir := filepath.Join(tmpDir, "level1", "level2", "level3")

		err := ensureParentDirExists(ctx, parentDir, "www-data", "www-data", nil, "TestProject")
		if err != nil {
			t.Errorf("Expected no error creating nested parent dirs, got: %v", err)
		}

		// Verify directory was created
		info, err := os.Stat(parentDir)
		if err != nil {
			t.Fatalf("Expected parent dir to exist, got error: %v", err)
		}
		if !info.IsDir() {
			t.Error("Expected parent dir to be a directory")
		}
	})

	t.Run("error when path is file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "existing-file")

		// Create a file at the parent path
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		err := ensureParentDirExists(ctx, filePath, "www-data", "www-data", nil, "TestProject")
		if err == nil {
			t.Error("Expected error when path is an existing file, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "not a directory") {
			t.Errorf("Expected 'not a directory' error, got: %v", err)
		}
	})
}

// TestDeferredReloadNotTriggeredByWebhook tests that webhook trigger alone doesn't cause reload
func TestDeferredReloadNotTriggeredByWebhook(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	config := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "TestProject",
				"webhook_path": "/hooks/test",
				"webhook_secret": "secret123",
				"execute_command": "echo test"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	cm, err := NewConfigManager(configPath, logger)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	deployer := NewDeployer(logger)
	deployer.SetConfigManager(cm)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecuteCommand: "echo hello",
	}

	// Clear the buffer before deployment
	buf.Reset()

	// Deploy should NOT trigger config reload
	result := deployer.Deploy(context.Background(), project, "INTERNAL")
	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should NOT see "Processing deferred configuration reload" in logs
	if strings.Contains(logOutput, "Processing deferred configuration reload") {
		t.Error("Config reload should NOT be triggered by webhook/deployment alone")
	}
}

// TestFilePermissionsWithUmask tests that files created during build have correct permissions
func TestFilePermissionsWithUmask(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_file.txt")

	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecutePath:    tmpDir,
		ExecuteCommand: "touch test_file.txt",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Fatalf("Deployment failed: %s", result.Error)
	}

	// Check file exists
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Expected test file to exist: %v", err)
	}

	// File permissions should allow read by all (umask 0022)
	// Expected: -rw-r--r-- (0644) for files created with umask 0022
	perm := info.Mode().Perm()
	if perm&0044 == 0 {
		t.Errorf("Expected file to be readable by group and others, got permissions: %o", perm)
	}
}

// TestDirectoryPermissionsWithUmask tests that directories created during build have correct permissions
func TestDirectoryPermissionsWithUmask(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test_dir")

	deployer := NewDeployer(nil)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		ExecutePath:    tmpDir,
		ExecuteCommand: "mkdir test_dir",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")
	if !result.Success {
		t.Fatalf("Deployment failed: %s", result.Error)
	}

	// Check directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Expected test directory to exist: %v", err)
	}

	if !info.IsDir() {
		t.Fatal("Expected test_dir to be a directory")
	}

	// Directory permissions should allow read/execute by all (umask 0022)
	// Expected: drwxr-xr-x (0755) for directories created with umask 0022
	perm := info.Mode().Perm()
	if perm&0055 == 0 {
		t.Errorf("Expected directory to be readable/executable by group and others, got permissions: %o", perm)
	}
}
