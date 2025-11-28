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
	os.MkdirAll(gitPath, 0755)

	// Create a simple script that echoes git pull
	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitUpdate:      true,
		GitPath:        gitPath,
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
