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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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
	logger := NewLogger(&buf, "", false)
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

// TestBuildCommandFunction tests buildCommand function exists and works
func TestBuildCommandFunction(t *testing.T) {
	ctx := context.Background()
	cmd := buildCommand(ctx, "echo test")
	if cmd == nil {
		t.Error("Expected buildCommand to return a non-nil command")
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
		err := ensureParentDirExists(ctx, parentDir, nil, "TestProject")
		if err != nil {
			t.Errorf("Expected no error when parent dir exists, got: %v", err)
		}
	})

	t.Run("creates parent dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		parentDir := filepath.Join(tmpDir, "new-parent")
		var buf bytes.Buffer
		logger := NewLogger(&buf, "", false)

		err := ensureParentDirExists(ctx, parentDir, logger, "TestProject")
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

		err := ensureParentDirExists(ctx, parentDir, nil, "TestProject")
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

		err := ensureParentDirExists(ctx, filePath, nil, "TestProject")
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
	logger := NewLogger(&buf, "", false)
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

// TestDeployWithSSHKey tests deployment with git_ssh_key_path configured
func TestDeployWithSSHKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a dummy SSH key file
	keyPath := filepath.Join(tmpDir, "test-key")
	err := os.WriteFile(keyPath, []byte("dummy-ssh-key"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test SSH key file: %v", err)
	}

	// Create a .git directory to simulate an already cloned repo
	repoPath := filepath.Join(tmpDir, "repo")
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "git@github.com:example/repo.git",
		LocalPath:      repoPath,
		GitBranch:      "main",
		GitUpdate:      false, // Don't try to pull
		GitSSHKeyPath:  keyPath,
		ExecutePath:    repoPath,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see SSH key usage logged (without revealing the path)
	if !strings.Contains(logOutput, "Using SSH key for git operations") {
		t.Errorf("Expected log to contain 'Using SSH key for git operations', got: %s", logOutput)
	}

	// Build config should show SSH key is configured
	if !strings.Contains(logOutput, "git_ssh_key=configured") {
		t.Errorf("Expected build config to show git_ssh_key=configured, got: %s", logOutput)
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}
}

// TestDeployWithoutSSHKey tests deployment without git_ssh_key_path (public repo)
func TestDeployWithoutSSHKey(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate an already cloned repo
	repoPath := filepath.Join(tmpDir, "repo")
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/public-repo.git",
		LocalPath:      repoPath,
		GitBranch:      "main",
		GitUpdate:      false,
		ExecutePath:    repoPath,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should NOT see SSH key usage logged
	if strings.Contains(logOutput, "Using SSH key:") {
		t.Errorf("Expected log to NOT contain 'Using SSH key:', got: %s", logOutput)
	}

	// Build config should show SSH key is none
	if !strings.Contains(logOutput, "git_ssh_key=none") {
		t.Errorf("Expected build config to show git_ssh_key=none, got: %s", logOutput)
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}
}

// TestDeploySSHKeyValidationError tests deployment fails when SSH key is invalid
func TestDeploySSHKeyValidationError(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "git@github.com:example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitSSHKeyPath:  "/nonexistent/key/path",
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	if result.Success {
		t.Error("Expected deployment to fail with invalid SSH key path")
	}

	if !strings.Contains(result.Error, "SSH key validation failed") {
		t.Errorf("Expected error message about SSH key validation, got: %s", result.Error)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "SSH key validation failed") {
		t.Errorf("Expected log to contain SSH key validation error, got: %s", logOutput)
	}
}

// TestDeploySSHKeyMissingFile tests deployment fails when SSH key file doesn't exist
func TestDeploySSHKeyMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	deployer := NewDeployer(nil)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "git@github.com:example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitSSHKeyPath:  filepath.Join(tmpDir, "missing-key"),
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	if result.Success {
		t.Error("Expected deployment to fail with missing SSH key file")
	}

	if !strings.Contains(result.Error, "does not exist") {
		t.Errorf("Expected error message about missing file, got: %s", result.Error)
	}
}

// TestDeploySSHKeyBadPermissions tests deployment fails when SSH key has wrong permissions
func TestDeploySSHKeyBadPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test-key")

	// Create a key file with no read permissions
	err := os.WriteFile(keyPath, []byte("dummy-key"), 0000)
	if err != nil {
		t.Fatalf("Failed to create test SSH key file: %v", err)
	}

	deployer := NewDeployer(nil)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "git@github.com:example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitSSHKeyPath:  keyPath,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(context.Background(), project, "WEBHOOK")

	if result.Success {
		t.Error("Expected deployment to fail with unreadable SSH key file")
	}

	if !strings.Contains(result.Error, "not readable") {
		t.Errorf("Expected error message about unreadable file, got: %s", result.Error)
	}
}

// TestGetCurrentBranch tests the getCurrentBranch function
func TestGetCurrentBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create initial commit (required to have a branch)
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create a dummy file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Test getting current branch
	ctx := context.Background()
	branch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("getCurrentBranch failed: %v", err)
	}

	// Branch should be either "main" or "master" depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("Expected branch to be 'main' or 'master', got: %s", branch)
	}
}

// TestGetCurrentBranchNonRepo tests getCurrentBranch on non-repo directory
func TestGetCurrentBranchNonRepo(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()
	_, err := getCurrentBranch(ctx, tmpDir)
	if err == nil {
		t.Error("Expected getCurrentBranch to fail on non-repo directory")
	}
}

// TestEnsureCorrectBranchSameBranch tests ensureCorrectBranch when already on correct branch
func TestEnsureCorrectBranchSameBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Get current branch
	ctx := context.Background()
	currentBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:      "TestProject",
		LocalPath: tmpDir,
		GitBranch: currentBranch, // Same as current branch
	}

	// Should succeed without doing anything
	err = deployer.ensureCorrectBranch(ctx, project)
	if err != nil {
		t.Errorf("ensureCorrectBranch failed: %v", err)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Already on correct branch") {
		t.Errorf("Expected log to contain 'Already on correct branch', got: %s", logOutput)
	}
}

// TestEnsureCorrectBranchDifferentBranch tests ensureCorrectBranch when on different branch
func TestEnsureCorrectBranchDifferentBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Create a new branch
	cmd = exec.Command("git", "checkout", "-b", "develop")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create develop branch: %v", err)
	}

	// Get current branch (should be "develop")
	ctx := context.Background()
	currentBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	if currentBranch != "develop" {
		t.Fatalf("Expected current branch to be 'develop', got: %s", currentBranch)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	// Determine the main branch name
	cmd = exec.Command("git", "checkout", "-")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout previous branch: %v", err)
	}

	mainBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get main branch: %v", err)
	}

	// Switch back to develop
	cmd = exec.Command("git", "checkout", "develop")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout develop: %v", err)
	}

	project := &ProjectConfig{
		Name:      "TestProject",
		LocalPath: tmpDir,
		GitBranch: mainBranch, // Different from current branch
	}

	// Should checkout the configured branch
	err = deployer.ensureCorrectBranch(ctx, project)
	if err != nil {
		t.Errorf("ensureCorrectBranch failed: %v", err)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "Checking out branch") {
		t.Errorf("Expected log to contain 'Checking out branch', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "Successfully checked out branch") {
		t.Errorf("Expected log to contain 'Successfully checked out branch', got: %s", logOutput)
	}

	// Verify we're now on the correct branch
	finalBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get final branch: %v", err)
	}

	if finalBranch != mainBranch {
		t.Errorf("Expected to be on branch '%s', but on '%s'", mainBranch, finalBranch)
	}
}

// TestEnsureCorrectBranchNonExistentBranch tests ensureCorrectBranch with non-existent branch
func TestEnsureCorrectBranchNonExistentBranch(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	ctx := context.Background()
	deployer := NewDeployer(nil)

	project := &ProjectConfig{
		Name:      "TestProject",
		LocalPath: tmpDir,
		GitBranch: "nonexistent-branch",
	}

	// Should fail
	err := deployer.ensureCorrectBranch(ctx, project)
	if err == nil {
		t.Error("Expected ensureCorrectBranch to fail with non-existent branch")
	}

	if !strings.Contains(err.Error(), "failed to checkout branch") {
		t.Errorf("Expected error about failed checkout, got: %v", err)
	}
}

// TestDeployWithBranchCheckout tests full deployment with branch checkout
func TestDeployWithBranchCheckout(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Get the initial branch name
	ctx := context.Background()
	initialBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get initial branch: %v", err)
	}

	// Create and switch to a different branch
	cmd = exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "dummy-repo", // Set git_repo to trigger git operations
		LocalPath:      tmpDir,
		GitBranch:      initialBranch, // Configure to use initial branch
		GitUpdate:      false,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo 'deployed'",
	}

	result := deployer.Deploy(ctx, project, "WEBHOOK")

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should see branch checkout logging
	if !strings.Contains(logOutput, "Checking out branch") {
		t.Errorf("Expected log to contain 'Checking out branch', got: %s", logOutput)
	}

	// Verify we're now on the correct branch
	finalBranch, err := getCurrentBranch(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Failed to get final branch: %v", err)
	}

	if finalBranch != initialBranch {
		t.Errorf("Expected to be on branch '%s', but on '%s'", initialBranch, finalBranch)
	}
}

// TestBranchLoggedInBuildConfig tests that current branch is logged in build config
func TestBranchLoggedInBuildConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate a git repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "develop",
		GitUpdate:      false,
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo test",
	}

	deployer.Deploy(context.Background(), project, "WEBHOOK")

	logOutput := buf.String()

	// Should see branch in build config
	if !strings.Contains(logOutput, "git_branch=develop") {
		t.Errorf("Expected build config to contain 'git_branch=develop', got: %s", logOutput)
	}
}

// TestDeployWithCloneAndBranchCheckout tests that branch is verified after git clone
func TestDeployWithCloneAndBranchCheckout(t *testing.T) {
	// Create a source repository
	sourceDir := t.TempDir()
	
	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit on master/main
	testFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Get the initial branch name
	ctx := context.Background()
	initialBranch, err := getCurrentBranch(ctx, sourceDir)
	if err != nil {
		t.Fatalf("Failed to get initial branch: %v", err)
	}

	// Create a different branch
	cmd = exec.Command("git", "checkout", "-b", "develop")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create develop branch: %v", err)
	}

	// Add a commit to develop
	developFile := filepath.Join(sourceDir, "develop.txt")
	if err := os.WriteFile(developFile, []byte("develop"), 0644); err != nil {
		t.Fatalf("Failed to create develop file: %v", err)
	}

	cmd = exec.Command("git", "add", "develop.txt")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add develop: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Add develop file")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit develop: %v", err)
	}

	// Switch back to initial branch as the default
	cmd = exec.Command("git", "checkout", initialBranch)
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout initial branch: %v", err)
	}

	// Now test SDeploy cloning and ensuring correct branch
	cloneDir := t.TempDir()
	
	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	// Configure to clone to 'develop' branch (different from default)
	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        fmt.Sprintf("file://%s", sourceDir),
		LocalPath:      filepath.Join(cloneDir, "repo"),
		GitBranch:      "develop",
		GitUpdate:      false,
		ExecutePath:    filepath.Join(cloneDir, "repo"),
		ExecuteCommand: "echo test",
	}

	result := deployer.Deploy(ctx, project, "WEBHOOK")

	if !result.Success {
		t.Fatalf("Expected deployment to succeed, got error: %s\nLogs:\n%s", result.Error, buf.String())
	}

	logOutput := buf.String()

	// Should see messages about branch verification after clone
	if !strings.Contains(logOutput, "Cloned repository") {
		t.Errorf("Expected log to contain 'Cloned repository', got: %s", logOutput)
	}
	
	if !strings.Contains(logOutput, "configured branch: develop") {
		t.Errorf("Expected log to show configured branch, got: %s", logOutput)
	}

	// Verify we're on the correct branch
	finalBranch, err := getCurrentBranch(ctx, project.LocalPath)
	if err != nil {
		t.Fatalf("Failed to get final branch: %v", err)
	}

	if finalBranch != "develop" {
		t.Errorf("Expected to be on 'develop' branch after clone, but on '%s'", finalBranch)
	}

	// Verify develop.txt exists (from develop branch)
	developFilePath := filepath.Join(project.LocalPath, "develop.txt")
	if _, err := os.Stat(developFilePath); err != nil {
		t.Errorf("Expected develop.txt to exist on develop branch, but got error: %v", err)
	}
}

// TestGetCurrentCommitSHA tests the getCurrentCommitSHA function
func TestGetCurrentCommitSHA(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Test getting current commit SHA
	ctx := context.Background()
	sha, err := getCurrentCommitSHA(ctx, tmpDir)
	if err != nil {
		t.Fatalf("getCurrentCommitSHA failed: %v", err)
	}

	// SHA should be 40 characters (hex)
	if len(sha) != 40 {
		t.Errorf("Expected SHA to be 40 characters, got %d: %s", len(sha), sha)
	}
}

// TestGetCurrentCommitSHANonRepo tests getCurrentCommitSHA on non-repo directory
func TestGetCurrentCommitSHANonRepo(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := context.Background()
	_, err := getCurrentCommitSHA(ctx, tmpDir)
	if err == nil {
		t.Error("Expected getCurrentCommitSHA to fail on non-repo directory")
	}
}

// TestDeployNoChangesDetection tests that build is skipped when no changes detected
func TestDeployNoChangesDetection(t *testing.T) {
	// Create a source repository (remote)
	sourceDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init bare git repo: %v", err)
	}

	// Create a working directory to make commits
	workDir := t.TempDir()
	cmd = exec.Command("git", "clone", sourceDir, workDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(workDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	cmd = exec.Command("git", "push", "origin", "HEAD")
	cmd.Dir = workDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git push: %v", err)
	}

	// Get the actual branch name
	ctx := context.Background()
	actualBranch, err := getCurrentBranch(ctx, workDir)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	// Clone to another directory (this will be the deployment target)
	cloneDir := t.TempDir()
	targetPath := filepath.Join(cloneDir, "repo")

	cmd = exec.Command("git", "clone", "--branch", actualBranch, sourceDir, targetPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone target repo: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        fmt.Sprintf("file://%s", sourceDir),
		LocalPath:      targetPath,
		GitBranch:      actualBranch,
		GitUpdate:      true, // Enable git pull
		ExecutePath:    targetPath,
		ExecuteCommand: "echo 'deployed'",
	}

	// First deployment - should be skipped since there are no new changes
	result := deployer.Deploy(ctx, project, "WEBHOOK")

	// Since there are no changes (repo is already up to date),
	// the build should be skipped
	if !result.Skipped {
		t.Errorf("Expected build to be skipped when no changes, got skipped=%v, success=%v, error=%s", result.Skipped, result.Success, result.Error)
	}

	logOutput := buf.String()

	// Should see message about no changes
	if !strings.Contains(logOutput, "No changes detected") || !strings.Contains(logOutput, "Build ignored: no changes in the configured branch") {
		t.Errorf("Expected log to contain message about no changes, got: %s", logOutput)
	}
}

// TestDeployWithChangesDetection tests that build runs when changes are detected
func TestDeployWithChangesDetection(t *testing.T) {
	// Create a source repository
	sourceDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Clone to a target directory
	cloneDir := t.TempDir()
	targetPath := filepath.Join(cloneDir, "repo")

	cmd = exec.Command("git", "clone", sourceDir, targetPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone repo: %v", err)
	}

	// Add a new commit to source repo
	testFile2 := filepath.Join(sourceDir, "test2.txt")
	if err := os.WriteFile(testFile2, []byte("test2"), 0644); err != nil {
		t.Fatalf("Failed to create test2 file: %v", err)
	}

	cmd = exec.Command("git", "add", "test2.txt")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add test2: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Add test2")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit test2: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	// Get current branch name
	ctx := context.Background()
	branch, err := getCurrentBranch(ctx, sourceDir)
	if err != nil {
		t.Fatalf("Failed to get branch: %v", err)
	}

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        fmt.Sprintf("file://%s", sourceDir),
		LocalPath:      targetPath,
		GitBranch:      branch,
		GitUpdate:      true, // Enable git pull
		ExecutePath:    targetPath,
		ExecuteCommand: "echo 'deployed'",
	}

	// Deploy - should detect changes and run build
	result := deployer.Deploy(ctx, project, "WEBHOOK")

	if result.Skipped {
		t.Errorf("Expected build to run when changes detected, but it was skipped")
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should see message about changes detected
	if !strings.Contains(logOutput, "Changes detected") {
		t.Errorf("Expected log to contain 'Changes detected', got: %s", logOutput)
	}
}

// TestDeployNoGitUpdateNoChangeDetection tests that build runs when git_update is false (no change detection)
func TestDeployNoGitUpdateNoChangeDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .git directory to simulate a git repo
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        "https://github.com/example/repo.git",
		LocalPath:      tmpDir,
		GitBranch:      "main",
		GitUpdate:      false, // Disable git pull - no change detection
		ExecutePath:    tmpDir,
		ExecuteCommand: "echo 'deployed'",
	}

	ctx := context.Background()
	result := deployer.Deploy(ctx, project, "WEBHOOK")

	// Build should run (not skipped) since we're not doing git pull / change detection
	if result.Skipped {
		t.Errorf("Expected build to run when git_update is false (no change detection), but it was skipped")
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should see message about skipping git pull
	if !strings.Contains(logOutput, "git_update is false, skipping git pull") {
		t.Errorf("Expected log to contain 'git_update is false', got: %s", logOutput)
	}
}

// TestDeployCloneAlwaysHasChanges tests that cloning always proceeds with build (considered as having changes)
func TestDeployCloneAlwaysHasChanges(t *testing.T) {
	// Create a source repository
	sourceDir := t.TempDir()

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git user name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(sourceDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	cloneDir := t.TempDir()
	targetPath := filepath.Join(cloneDir, "repo")

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	deployer := NewDeployer(logger)

	ctx := context.Background()
	branch, err := getCurrentBranch(ctx, sourceDir)
	if err != nil {
		t.Fatalf("Failed to get branch: %v", err)
	}

	project := &ProjectConfig{
		Name:           "TestProject",
		WebhookPath:    "/hooks/test",
		GitRepo:        fmt.Sprintf("file://%s", sourceDir),
		LocalPath:      targetPath,
		GitBranch:      branch,
		GitUpdate:      true,
		ExecutePath:    targetPath,
		ExecuteCommand: "echo 'deployed'",
	}

	// Deploy with clone - should always proceed with build
	result := deployer.Deploy(ctx, project, "WEBHOOK")

	if result.Skipped {
		t.Errorf("Expected build to run after cloning (clone always has changes), but it was skipped")
	}

	if !result.Success {
		t.Errorf("Expected deployment to succeed, got error: %s", result.Error)
	}

	logOutput := buf.String()

	// Should see message about cloning
	if !strings.Contains(logOutput, "Cloned repository") {
		t.Errorf("Expected log to contain 'Cloned repository', got: %s", logOutput)
	}
}
