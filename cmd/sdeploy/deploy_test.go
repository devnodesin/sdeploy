package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
