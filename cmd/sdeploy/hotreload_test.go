package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestConfigManagerCreation tests creating a ConfigManager
func TestConfigManagerCreation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	validConfig := `
listen_port: 8080
projects:
  - name: TestProject
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	cfg := cm.GetConfig()
	if cfg.ListenPort != 8080 {
		t.Errorf("Expected ListenPort 8080, got %d", cfg.ListenPort)
	}
}

// TestConfigManagerGetProject tests getting a project by webhook path
func TestConfigManagerGetProject(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	validConfig := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret1
    execute_command: echo frontend
  - name: Backend
    webhook_path: /hooks/backend
    webhook_secret: secret2
    execute_command: echo backend
`

	if err := os.WriteFile(configPath, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	// Test existing project
	project := cm.GetProject("/hooks/frontend")
	if project == nil {
		t.Error("Expected to find /hooks/frontend project")
	}
	if project != nil && project.Name != "Frontend" {
		t.Errorf("Expected project name 'Frontend', got '%s'", project.Name)
	}

	// Test non-existing project
	project = cm.GetProject("/hooks/nonexistent")
	if project != nil {
		t.Error("Expected nil for non-existent project")
	}
}

// TestConfigManagerHotReload tests hot reload functionality
func TestConfigManagerHotReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	initialConfig := `
listen_port: 8080
projects:
  - name: Initial
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo initial
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	cm, err := NewConfigManager(configPath, logger)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	// Start watcher
	if err := cm.StartWatcher(); err != nil {
		t.Fatalf("StartWatcher failed: %v", err)
	}

	// Verify initial config
	project := cm.GetProject("/hooks/test")
	if project == nil || project.Name != "Initial" {
		t.Fatal("Initial project not found or has wrong name")
	}

	// Update config file
	updatedConfig := `
listen_port: 8080
projects:
  - name: Updated
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo updated
`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config file: %v", err)
	}

	// Wait for hot reload (debounce + processing time)
	time.Sleep(800 * time.Millisecond)

	// Verify config was reloaded
	project = cm.GetProject("/hooks/test")
	if project == nil {
		t.Fatal("Project not found after reload")
	}
	if project.Name != "Updated" {
		t.Errorf("Expected project name 'Updated', got '%s'", project.Name)
	}

	// Verify logging
	logOutput := buf.String()
	if !strings.Contains(logOutput, "Configuration reloaded successfully") {
		t.Errorf("Expected reload success log message, got: %s", logOutput)
	}
}

// TestConfigManagerInvalidReload tests that invalid config is rejected
func TestConfigManagerInvalidReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	initialConfig := `
listen_port: 8080
projects:
  - name: Initial
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo initial
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	cm, err := NewConfigManager(configPath, logger)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	if err := cm.StartWatcher(); err != nil {
		t.Fatalf("StartWatcher failed: %v", err)
	}

	// Write invalid YAML
	invalidConfig := `listen_port: 8080
projects:
  - name: "unclosed string
    webhook_path: /test`
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Wait for hot reload attempt
	time.Sleep(800 * time.Millisecond)

	// Verify original config is still intact
	project := cm.GetProject("/hooks/test")
	if project == nil || project.Name != "Initial" {
		t.Error("Expected original config to be preserved after invalid reload")
	}

	// Verify error was logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "Failed to reload configuration") {
		t.Errorf("Expected reload failure log message, got: %s", logOutput)
	}
}

// TestConfigManagerPortChangeWarning tests warning for listen_port change
func TestConfigManagerPortChangeWarning(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	initialConfig := `
listen_port: 8080
projects:
  - name: Test
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	cm, err := NewConfigManager(configPath, logger)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	if err := cm.StartWatcher(); err != nil {
		t.Fatalf("StartWatcher failed: %v", err)
	}

	// Change listen_port
	updatedConfig := `
listen_port: 9090
projects:
  - name: Test
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config file: %v", err)
	}

	// Wait for hot reload
	time.Sleep(800 * time.Millisecond)

	// Verify warning was logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "listen_port changed") {
		t.Errorf("Expected listen_port change warning, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "Restart required") {
		t.Errorf("Expected restart required message, got: %s", logOutput)
	}
}

// TestConfigManagerReloadCallback tests the onReload callback
func TestConfigManagerReloadCallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	initialConfig := `
listen_port: 8080
projects:
  - name: Initial
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo initial
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	// Set callback
	var callbackCfg *Config
	var callbackMu sync.Mutex
	cm.SetOnReload(func(cfg *Config) {
		callbackMu.Lock()
		callbackCfg = cfg
		callbackMu.Unlock()
	})

	if err := cm.StartWatcher(); err != nil {
		t.Fatalf("StartWatcher failed: %v", err)
	}

	// Update config file
	updatedConfig := `
listen_port: 8080
projects:
  - name: Callback Test
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo callback
`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config file: %v", err)
	}

	// Wait for hot reload
	time.Sleep(800 * time.Millisecond)

	// Verify callback was called with new config
	callbackMu.Lock()
	defer callbackMu.Unlock()
	if callbackCfg == nil {
		t.Error("Expected callback to be called")
	}
	if callbackCfg != nil && len(callbackCfg.Projects) > 0 && callbackCfg.Projects[0].Name != "Callback Test" {
		t.Errorf("Expected callback config project name 'Callback Test', got '%s'", callbackCfg.Projects[0].Name)
	}
}

// TestConfigManagerDeferredReload tests deferring reload during active builds
func TestConfigManagerDeferredReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	initialConfig := `
listen_port: 8080
projects:
  - name: Initial
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo initial
`

	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "", false)
	cm, err := NewConfigManager(configPath, logger)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	// Set reload as pending (simulating active build)
	cm.SetReloadPending(true)

	// Verify reload is pending
	if !cm.IsReloadPending() {
		t.Error("Expected reload to be pending")
	}

	// Clear pending and process
	cm.SetReloadPending(false)
	cm.ProcessPendingReload()

	// Should not process since we cleared the pending flag
	logOutput := buf.String()
	if strings.Contains(logOutput, "Processing deferred configuration reload") {
		t.Error("Should not process reload when pending flag is cleared")
	}

	// Test actual deferred processing
	buf.Reset()
	cm.SetReloadPending(true)

	// Write updated config
	updatedConfig := `
listen_port: 8080
projects:
  - name: Deferred
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo deferred
`

	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("Failed to update test config file: %v", err)
	}

	// Process pending reload
	cm.ProcessPendingReload()

	// Verify deferred reload was processed
	logOutput = buf.String()
	if !strings.Contains(logOutput, "Processing deferred configuration reload") {
		t.Errorf("Expected deferred reload processing log, got: %s", logOutput)
	}

	// Verify config was updated
	project := cm.GetProject("/hooks/test")
	if project == nil || project.Name != "Deferred" {
		t.Error("Expected config to be updated after deferred reload")
	}
}

// TestWebhookHandlerWithConfigManager tests webhook handler with hot reload
func TestWebhookHandlerWithConfigManager(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	config := `
listen_port: 8080
projects:
  - name: TestProject
    webhook_path: /hooks/test
    webhook_secret: secret123
    git_branch: main
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	handler := NewWebhookHandlerWithConfigManager(cm, nil)

	// Test getProject
	project := handler.getProject("/hooks/test")
	if project == nil {
		t.Error("Expected to find project /hooks/test")
	}
	if project != nil && project.Name != "TestProject" {
		t.Errorf("Expected project name 'TestProject', got '%s'", project.Name)
	}

	// Test non-existent project
	project = handler.getProject("/hooks/nonexistent")
	if project != nil {
		t.Error("Expected nil for non-existent project")
	}
}

// TestDeployerWithConfigManager tests deployer integration with ConfigManager
func TestDeployerWithConfigManager(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	config := `
listen_port: 8080
projects:
  - name: TestProject
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	deployer := NewDeployer(nil)
	deployer.SetConfigManager(cm)

	// Verify no active builds initially
	if deployer.HasActiveBuilds() {
		t.Error("Expected no active builds initially")
	}
}

// TestConfigManagerThreadSafety tests thread-safe access to config
func TestConfigManagerThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	config := `
listen_port: 8080
projects:
  - name: TestProject
    webhook_path: /hooks/test
    webhook_secret: secret123
    execute_command: echo test
`

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm, err := NewConfigManager(configPath, nil)
	if err != nil {
		t.Fatalf("NewConfigManager failed: %v", err)
	}
	defer cm.Stop()

	// Concurrent access test
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.GetConfig()
			_ = cm.GetProject("/hooks/test")
		}()
	}
	wg.Wait()
}
