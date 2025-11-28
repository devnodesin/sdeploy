package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigValidFile tests loading a valid JSON config file
func TestLoadConfigValidFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	validConfig := `{
		"listen_port": 8080,
		"log_filepath": "/var/log/sdeploy/daemon.log",
		"email_config": {
			"smtp_host": "smtp.sendgrid.net",
			"smtp_port": 587,
			"smtp_user": "apikey",
			"smtp_pass": "SG.xxxxxxxxxxxx",
			"email_sender": "sdeploy@example.com"
		},
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"webhook_secret": "secret_token_123",
				"git_repo": "git@github.com:myorg/frontend.git",
				"git_path": "/var/repo/frontend",
				"execute_path": "/var/www/site",
				"git_branch": "main",
				"execute_command": "sh /var/www/site/deploy.sh",
				"git_update": true,
				"email_recipients": ["team@example.com"]
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ListenPort != 8080 {
		t.Errorf("Expected ListenPort 8080, got %d", cfg.ListenPort)
	}

	if cfg.LogFilepath != "/var/log/sdeploy/daemon.log" {
		t.Errorf("Expected LogFilepath '/var/log/sdeploy/daemon.log', got '%s'", cfg.LogFilepath)
	}

	if cfg.EmailConfig.SMTPHost != "smtp.sendgrid.net" {
		t.Errorf("Expected SMTPHost 'smtp.sendgrid.net', got '%s'", cfg.EmailConfig.SMTPHost)
	}

	if len(cfg.Projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(cfg.Projects))
	}

	if cfg.Projects[0].Name != "Frontend" {
		t.Errorf("Expected project name 'Frontend', got '%s'", cfg.Projects[0].Name)
	}
}

// TestLoadConfigMissingFile tests error handling for missing config file
func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error for missing config file, got nil")
	}
}

// TestLoadConfigInvalidJSON tests error handling for invalid JSON
func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	invalidJSON := `{invalid json`
	err := os.WriteFile(configPath, []byte(invalidJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

// TestLoadConfigMissingRequiredFields tests validation of required fields
func TestLoadConfigMissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Config missing webhook_secret
	configMissingSecret := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"git_branch": "main",
				"execute_command": "sh deploy.sh"
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configMissingSecret), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for missing webhook_secret, got nil")
	}
}

// TestLoadConfigDuplicateWebhookPath tests validation for unique webhook_path
func TestLoadConfigDuplicateWebhookPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configDuplicatePath := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/myapp",
				"webhook_secret": "secret1",
				"git_branch": "main",
				"execute_command": "sh deploy1.sh"
			},
			{
				"name": "Backend",
				"webhook_path": "/hooks/myapp",
				"webhook_secret": "secret2",
				"git_branch": "main",
				"execute_command": "sh deploy2.sh"
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configDuplicatePath), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for duplicate webhook_path, got nil")
	}
}

// TestLoadConfigDefaultPort tests default listen port
func TestLoadConfigDefaultPort(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Config without listen_port (should default to 8080)
	configNoPort := `{
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"webhook_secret": "secret_token_123",
				"git_branch": "main",
				"execute_command": "sh deploy.sh"
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configNoPort), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ListenPort != 8080 {
		t.Errorf("Expected default ListenPort 8080, got %d", cfg.ListenPort)
	}
}

// TestFindConfigFile tests the config file search order
func TestFindConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test 1: Explicit path provided
	explicitPath := filepath.Join(tmpDir, "explicit_config.json")
	err := os.WriteFile(explicitPath, []byte(`{"projects":[]}`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	found := FindConfigFile(explicitPath)
	if found != explicitPath {
		t.Errorf("Expected '%s', got '%s'", explicitPath, found)
	}

	// Test 2: Empty path falls back to search order
	// This test would need to check /etc/sdeploy/config.json and ./config.json
	// For unit testing, we'll just verify it returns empty if nothing found
	found = FindConfigFile("")
	// If we're running tests from a directory without config.json, this should be empty
	// or point to an existing config.json - we just verify it doesn't panic
	_ = found
}

// TestProjectConfigOptionalFields tests optional fields in project config
func TestProjectConfigOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Config with all optional fields
	configWithOptional := `{
		"listen_port": 8080,
		"projects": [
			{
				"name": "Frontend",
				"webhook_path": "/hooks/frontend",
				"webhook_secret": "secret_token_123",
				"git_repo": "git@github.com:myorg/frontend.git",
				"git_path": "/var/repo/frontend",
				"execute_path": "/var/www/site",
				"git_branch": "main",
				"execute_command": "sh deploy.sh",
				"git_update": true,
				"timeout_seconds": 300,
				"email_recipients": ["team@example.com", "admin@example.com"]
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(configWithOptional), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	project := cfg.Projects[0]
	if project.TimeoutSeconds != 300 {
		t.Errorf("Expected TimeoutSeconds 300, got %d", project.TimeoutSeconds)
	}

	if len(project.EmailRecipients) != 2 {
		t.Errorf("Expected 2 email recipients, got %d", len(project.EmailRecipients))
	}

	if !project.GitUpdate {
		t.Error("Expected GitUpdate to be true")
	}
}
