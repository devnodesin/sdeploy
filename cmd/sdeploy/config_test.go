package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMergedConfigSeparateProjectsInSeparateFiles verifies that projects
// split across multiple files are appended (not index-merged), matching on
// webhook_path. This mirrors a real layout where one file holds email_config,
// another holds some projects, and a third holds additional projects.
func TestLoadMergedConfigSeparateProjectsInSeparateFiles(t *testing.T) {
	tmpDir := t.TempDir()

	emailPath := filepath.Join(tmpDir, "email.conf")
	emailConfig := `
email_config:
  smtp_host: smtp.resend.com
  smtp_port: 2587
  smtp_user: resend
  smtp_pass: secret_smtp_pass
  email_sender: sdeploy@asensar.in
`
	if err := os.WriteFile(emailPath, []byte(emailConfig), 0644); err != nil {
		t.Fatalf("Failed to create email config: %v", err)
	}

	sitesPath := filepath.Join(tmpDir, "sites.conf")
	sitesConfig := `
projects:
  - name: fd.asensar.in
    webhook_path: /wh/site-stage-fd-home
    webhook_secret: StageHome
    git_repo: git@github.com:asensar/site-fastdep.git
    git_branch: main
    git_update: true
    local_path: /opt/sdeploy/site-stage-fd-home
    execute_command: sh build.sh
    email_recipients:
      - contact@asensar.com
  - name: fd.asensar.in.info
    webhook_path: /wh/site-stage-fd-info
    webhook_secret: StageInfo
    git_repo: git@github.com:asensar/site-fastdep.git
    git_branch: main
    git_update: true
    local_path: /opt/sdeploy/site-stage-fd-info
    execute_command: sh build.sh
    email_recipients:
      - contact@asensar.com
`
	if err := os.WriteFile(sitesPath, []byte(sitesConfig), 0644); err != nil {
		t.Fatalf("Failed to create sites config: %v", err)
	}

	extraPath := filepath.Join(tmpDir, "extra.conf")
	extraConfig := `
projects:
  - name: deltaware.in
    webhook_path: /wh/deltaware-in
    webhook_secret: DeltawareIN
    git_repo: git@github.com:asensar/site-deltaware.git
    git_branch: main
    git_update: true
    local_path: /opt/sdeploy/site-live-deltaware-home
    execute_command: sh build.sh
`
	if err := os.WriteFile(extraPath, []byte(extraConfig), 0644); err != nil {
		t.Fatalf("Failed to create extra config: %v", err)
	}

	cfg, err := LoadMergedConfig([]string{emailPath, sitesPath, extraPath})
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}

	// All three projects should be present (2 from sites + 1 appended from extra).
	if len(cfg.Projects) != 3 {
		t.Fatalf("Expected 3 projects, got %d", len(cfg.Projects))
	}

	// email_config preserved from the first file.
	if cfg.EmailConfig == nil || cfg.EmailConfig.SMTPHost != "smtp.resend.com" {
		t.Fatalf("Expected email_config preserved, got %+v", cfg.EmailConfig)
	}

	// The appended project keeps its own fields, not merged into index 0.
	delta := cfg.Projects[2]
	if delta.Name != "deltaware.in" {
		t.Errorf("Expected appended project name 'deltaware.in', got '%s'", delta.Name)
	}
	if delta.GitRepo != "git@github.com:asensar/site-deltaware.git" {
		t.Errorf("Expected deltaware git_repo, got '%s'", delta.GitRepo)
	}
	if delta.WebhookPath != "/wh/deltaware-in" {
		t.Errorf("Expected deltaware webhook_path, got '%s'", delta.WebhookPath)
	}

	// Original first project must remain intact (not overwritten by deltaware).
	first := cfg.Projects[0]
	if first.Name != "fd.asensar.in" {
		t.Errorf("Expected first project name 'fd.asensar.in', got '%s'", first.Name)
	}
	if first.GitRepo != "git@github.com:asensar/site-fastdep.git" {
		t.Errorf("Expected first project git_repo preserved, got '%s'", first.GitRepo)
	}
	if len(first.EmailRecipients) != 1 || first.EmailRecipients[0] != "contact@asensar.com" {
		t.Errorf("Expected first project email_recipients preserved, got %v", first.EmailRecipients)
	}
}

// TestLoadMergedConfigPatchProjectByWebhookPath verifies that a later file can
// patch an existing project (same webhook_path) without rewriting the whole list.
func TestLoadMergedConfigPatchProjectByWebhookPath(t *testing.T) {
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "base.conf")
	baseConfig := `
projects:
  - name: app
    webhook_path: /wh/app
    webhook_secret: base_secret
    git_repo: git@github.com:org/app.git
    execute_command: sh build.sh
    email_recipients:
      - a@example.com
`
	if err := os.WriteFile(basePath, []byte(baseConfig), 0644); err != nil {
		t.Fatalf("Failed to create base config: %v", err)
	}

	secretPath := filepath.Join(tmpDir, "secret.conf")
	secretConfig := `
projects:
  - webhook_path: /wh/app
    webhook_secret: overridden_secret
`
	if err := os.WriteFile(secretPath, []byte(secretConfig), 0644); err != nil {
		t.Fatalf("Failed to create secret config: %v", err)
	}

	cfg, err := LoadMergedConfig([]string{basePath, secretPath})
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("Expected 1 project after patch, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0].WebhookSecret != "overridden_secret" {
		t.Errorf("Expected patched webhook_secret, got '%s'", cfg.Projects[0].WebhookSecret)
	}
	// Unset fields from the overlay are inherited from the base project.
	if cfg.Projects[0].GitRepo != "git@github.com:org/app.git" {
		t.Errorf("Expected inherited git_repo, got '%s'", cfg.Projects[0].GitRepo)
	}
	if len(cfg.Projects[0].EmailRecipients) != 1 {
		t.Errorf("Expected inherited email_recipients, got %v", cfg.Projects[0].EmailRecipients)
	}
}

// TestLoadMergedConfigTestsMultipleFiles tests loading and merging multiple config files
func TestLoadMergedConfigTestsMultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Base config file
	basePath := filepath.Join(tmpDir, "sdeploy.conf")
	baseConfig := `
listen_port: 8080
log_path: /var/log/sdeploy
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: base_secret
    execute_command: sh deploy.sh
`
	if err := os.WriteFile(basePath, []byte(baseConfig), 0644); err != nil {
		t.Fatalf("Failed to create base config file: %v", err)
	}

	// Overlay config file (later overrides earlier). To patch the existing
	// project, the overlay must include the matching webhook_path.
	overlayPath := filepath.Join(tmpDir, "secrets.conf")
	overlayConfig := `
listen_port: 9090
projects:
  - webhook_path: /hooks/frontend
    webhook_secret: overridden_secret
`
	if err := os.WriteFile(overlayPath, []byte(overlayConfig), 0644); err != nil {
		t.Fatalf("Failed to create overlay config file: %v", err)
	}

	cfg, err := LoadMergedConfig([]string{basePath, overlayPath})
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}

	// listen_port overridden by the second file
	if cfg.ListenPort != 9090 {
		t.Errorf("Expected ListenPort 9090 (overridden), got %d", cfg.ListenPort)
	}

	if len(cfg.Projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(cfg.Projects))
	}

	// webhook_secret overridden, execute_command inherited from base
	if cfg.Projects[0].WebhookSecret != "overridden_secret" {
		t.Errorf("Expected WebhookSecret 'overridden_secret', got '%s'", cfg.Projects[0].WebhookSecret)
	}
	if cfg.Projects[0].ExecuteCommand != "sh deploy.sh" {
		t.Errorf("Expected ExecuteCommand 'sh deploy.sh' (inherited), got '%s'", cfg.Projects[0].ExecuteCommand)
	}
}

// TestLoadMergedConfigSkipsMissingFiles tests that missing files are skipped
func TestLoadMergedConfigSkipsMissingFiles(t *testing.T) {
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "sdeploy.conf")
	baseConfig := `
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_branch: main
    execute_command: sh deploy.sh
`
	if err := os.WriteFile(basePath, []byte(baseConfig), 0644); err != nil {
		t.Fatalf("Failed to create base config file: %v", err)
	}

	// A non-existent overlay should be skipped; the base config still loads
	cfg, err := LoadMergedConfig([]string{basePath, filepath.Join(tmpDir, "missing.conf")})
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(cfg.Projects))
	}
}

// TestLoadMergedConfigSingleFileEquivalent tests that a single file behaves like LoadConfig
func TestLoadMergedConfigSingleFileEquivalent(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "sdeploy.conf")
	config := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_branch: main
    execute_command: sh deploy.sh
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	merged, err := LoadMergedConfig([]string{configPath})
	if err != nil {
		t.Fatalf("LoadMergedConfig failed: %v", err)
	}

	single, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if merged.ListenPort != single.ListenPort {
		t.Errorf("ListenPort mismatch: merged=%d single=%d", merged.ListenPort, single.ListenPort)
	}
	if len(merged.Projects) != len(single.Projects) {
		t.Errorf("Projects length mismatch: merged=%d single=%d", len(merged.Projects), len(single.Projects))
	}
}

// TestLoadMergedConfigNoValidFiles tests error when no valid config is found
func TestLoadMergedConfigNoValidFiles(t *testing.T) {
	_, err := LoadMergedConfig([]string{"/nonexistent/a.conf", "/nonexistent/b.conf"})
	if err == nil {
		t.Error("Expected error when no valid config files found, got nil")
	}
}

// TestLoadConfigValidFile tests loading a valid YAML config file
func TestLoadConfigValidFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	validConfig := `
listen_port: 8080
log_path: /var/log/sdeploy
email_config:
  smtp_host: smtp.sendgrid.net
  smtp_port: 587
  smtp_user: apikey
  smtp_pass: SG.xxxxxxxxxxxx
  email_sender: sdeploy@example.com
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_repo: git@github.com:myorg/frontend.git
    local_path: /var/repo/frontend
    execute_path: /var/www/site
    git_branch: main
    execute_command: sh /var/www/site/deploy.sh
    git_update: true
    email_recipients:
      - team@example.com
`

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

	if cfg.LogPath != "/var/log/sdeploy" {
		t.Errorf("Expected LogPath '/var/log/sdeploy', got '%s'", cfg.LogPath)
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
	_, err := LoadConfig("/nonexistent/path/sdeploy.conf")
	if err == nil {
		t.Error("Expected error for missing config file, got nil")
	}
}

// TestLoadConfigInvalidYAML tests error handling for invalid YAML
func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	invalidYAML := `listen_port: 8080
projects:
  - name: "unclosed string
    webhook_path: /hooks/test`
	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

// TestLoadConfigMissingRequiredFields tests validation of required fields
func TestLoadConfigMissingRequiredFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	// Config missing webhook_secret
	configMissingSecret := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    git_branch: main
    execute_command: sh deploy.sh
`

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
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	configDuplicatePath := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/myapp
    webhook_secret: secret1
    git_branch: main
    execute_command: sh deploy1.sh
  - name: Backend
    webhook_path: /hooks/myapp
    webhook_secret: secret2
    git_branch: main
    execute_command: sh deploy2.sh
`

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
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	// Config without listen_port (should default to 8080)
	configNoPort := `
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_branch: main
    execute_command: sh deploy.sh
`

	err := os.WriteFile(configPath, []byte(configNoPort), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ListenPort != Defaults.Port {
		t.Errorf("Expected default ListenPort %d, got %d", Defaults.Port, cfg.ListenPort)
	}
}

// TestFindConfigFile tests the config file search order
func TestFindConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test 1: Explicit path provided
	explicitPath := filepath.Join(tmpDir, "explicit_config.conf")
	err := os.WriteFile(explicitPath, []byte("projects: []"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	found := FindConfigFile(explicitPath)
	if found != explicitPath {
		t.Errorf("Expected '%s', got '%s'", explicitPath, found)
	}

	// Test 2: Empty path falls back to search order
	// This test would need to check /etc/sdeploy.conf and ./sdeploy.conf
	// For unit testing, we'll just verify it returns empty if nothing found
	found = FindConfigFile("")
	// If we're running tests from a directory without sdeploy.conf, this should be empty
	// or point to an existing sdeploy.conf - we just verify it doesn't panic
	_ = found
}

// TestProjectConfigOptionalFields tests optional fields in project config
func TestProjectConfigOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	// Config with all optional fields
	configWithOptional := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_repo: git@github.com:myorg/frontend.git
    local_path: /var/repo/frontend
    execute_path: /var/www/site
    git_branch: main
    execute_command: sh deploy.sh
    git_update: true
    timeout_seconds: 300
    email_recipients:
      - team@example.com
      - admin@example.com
`

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

// TestIsEmailConfigValid tests the IsEmailConfigValid function
func TestIsEmailConfigValid(t *testing.T) {
	tests := []struct {
		name     string
		config   *EmailConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "valid config",
			config: &EmailConfig{
				SMTPHost:    "smtp.example.com",
				SMTPPort:    587,
				SMTPUser:    "user",
				SMTPPass:    "pass",
				EmailSender: "sender@example.com",
			},
			expected: true,
		},
		{
			name: "missing smtp_host",
			config: &EmailConfig{
				SMTPHost:    "",
				SMTPPort:    587,
				SMTPUser:    "user",
				SMTPPass:    "pass",
				EmailSender: "sender@example.com",
			},
			expected: false,
		},
		{
			name: "missing smtp_port (0)",
			config: &EmailConfig{
				SMTPHost:    "smtp.example.com",
				SMTPPort:    0,
				SMTPUser:    "user",
				SMTPPass:    "pass",
				EmailSender: "sender@example.com",
			},
			expected: false,
		},
		{
			name: "missing smtp_user",
			config: &EmailConfig{
				SMTPHost:    "smtp.example.com",
				SMTPPort:    587,
				SMTPUser:    "",
				SMTPPass:    "pass",
				EmailSender: "sender@example.com",
			},
			expected: false,
		},
		{
			name: "missing smtp_pass",
			config: &EmailConfig{
				SMTPHost:    "smtp.example.com",
				SMTPPort:    587,
				SMTPUser:    "user",
				SMTPPass:    "",
				EmailSender: "sender@example.com",
			},
			expected: false,
		},
		{
			name: "missing email_sender",
			config: &EmailConfig{
				SMTPHost:    "smtp.example.com",
				SMTPPort:    587,
				SMTPUser:    "user",
				SMTPPass:    "pass",
				EmailSender: "",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsEmailConfigValid(tc.config)
			if result != tc.expected {
				t.Errorf("IsEmailConfigValid() = %v, expected %v", result, tc.expected)
			}
		})
	}
}

// TestDefaultGitBranch tests that git_branch defaults to Defaults.GitBranch when empty
func TestDefaultGitBranch(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	// Config without git_branch
	configNoGitBranch := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    execute_command: sh deploy.sh
`

	err := os.WriteFile(configPath, []byte(configNoGitBranch), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Projects[0].GitBranch != Defaults.GitBranch {
		t.Errorf("Expected default GitBranch '%s', got '%s'", Defaults.GitBranch, cfg.Projects[0].GitBranch)
	}
}

// TestGitBranchNotOverwritten tests that a set git_branch is not overwritten
func TestGitBranchNotOverwritten(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	// Config with explicit git_branch
	configWithGitBranch := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_branch: develop
    execute_command: sh deploy.sh
`

	err := os.WriteFile(configPath, []byte(configWithGitBranch), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Projects[0].GitBranch != "develop" {
		t.Errorf("Expected GitBranch 'develop', got '%s'", cfg.Projects[0].GitBranch)
	}
}

// TestLoadConfigWithGitSSHKeyPath tests loading config with git_ssh_key_path
func TestLoadConfigWithGitSSHKeyPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")
	keyPath := filepath.Join(tmpDir, "test-key")

	// Create a dummy SSH key file
	err := os.WriteFile(keyPath, []byte("dummy-key-content"), 0600)
	if err != nil {
		t.Fatalf("Failed to create test SSH key file: %v", err)
	}

	configWithSSHKey := fmt.Sprintf(`
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_repo: git@github.com:myorg/repo.git
    git_ssh_key_path: %s
    execute_command: sh deploy.sh
`, keyPath)

	err = os.WriteFile(configPath, []byte(configWithSSHKey), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Projects[0].GitSSHKeyPath != keyPath {
		t.Errorf("Expected GitSSHKeyPath '%s', got '%s'", keyPath, cfg.Projects[0].GitSSHKeyPath)
	}
}

// TestValidateSSHKeyPath tests SSH key path validation
func TestValidateSSHKeyPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid key file", func(t *testing.T) {
		keyPath := filepath.Join(tmpDir, "valid-key")
		err := os.WriteFile(keyPath, []byte("dummy-key"), 0600)
		if err != nil {
			t.Fatalf("Failed to create test key file: %v", err)
		}

		err = validateSSHKeyPath(keyPath)
		if err != nil {
			t.Errorf("Expected no error for valid key file, got: %v", err)
		}
	})

	t.Run("non-existent key file", func(t *testing.T) {
		keyPath := filepath.Join(tmpDir, "nonexistent-key")
		err := validateSSHKeyPath(keyPath)
		if err == nil {
			t.Error("Expected error for non-existent key file, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Expected 'does not exist' error, got: %v", err)
		}
	})

	t.Run("directory instead of file", func(t *testing.T) {
		dirPath := filepath.Join(tmpDir, "key-dir")
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		err = validateSSHKeyPath(dirPath)
		if err == nil {
			t.Error("Expected error for directory, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "must be a file") {
			t.Errorf("Expected 'must be a file' error, got: %v", err)
		}
	})

	t.Run("unreadable key file", func(t *testing.T) {
		// Create a file with no read permissions
		keyPath := filepath.Join(tmpDir, "unreadable-key")
		err := os.WriteFile(keyPath, []byte("dummy-key"), 0000)
		if err != nil {
			t.Fatalf("Failed to create test key file: %v", err)
		}

		err = validateSSHKeyPath(keyPath)
		if err == nil {
			t.Error("Expected error for unreadable key file, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "not readable") {
			t.Errorf("Expected 'not readable' error, got: %v", err)
		}
	})
}

// TestLoadConfigInvalidSSHKeyPath tests validation error for invalid SSH key path
func TestLoadConfigInvalidSSHKeyPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	configWithInvalidKey := `
listen_port: 8080
projects:
  - name: Frontend
    webhook_path: /hooks/frontend
    webhook_secret: secret_token_123
    git_repo: git@github.com:myorg/repo.git
    git_ssh_key_path: /nonexistent/key/path
    execute_command: sh deploy.sh
`

	err := os.WriteFile(configPath, []byte(configWithInvalidKey), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid SSH key path, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "git_ssh_key_path") {
		t.Errorf("Expected error message to mention git_ssh_key_path, got: %v", err)
	}
}

// TestValidateGitBranchValid tests validateGitBranch with valid branch names
func TestValidateGitBranchValid(t *testing.T) {
	validBranches := []string{
		"main",
		"master",
		"develop",
		"feature/my-feature",
		"release/v1.0.0",
		"hotfix-123",
		"user_branch",
		"feature.test",
		"123-numeric",
	}

	for _, branch := range validBranches {
		t.Run(branch, func(t *testing.T) {
			err := validateGitBranch(branch)
			if err != nil {
				t.Errorf("Expected branch '%s' to be valid, got error: %v", branch, err)
			}
		})
	}
}

// TestValidateGitBranchInvalid tests validateGitBranch with invalid branch names
func TestValidateGitBranchInvalid(t *testing.T) {
	invalidBranches := []string{
		"",                   // empty
		"branch with spaces", // spaces
		"branch;rm -rf /",    // semicolon (command injection attempt)
		"branch`whoami`",     // backticks (command injection attempt)
		"branch$(whoami)",    // command substitution
		"branch&&echo hi",    // shell operators
		"branch|cat",         // pipe
		"branch>file",        // redirection
		"branch<file",        // redirection
		"branch\necho hi",    // newline
		"branch\techo hi",    // tab
		"branch*",            // wildcard
		"branch?",            // wildcard
		"branch[",            // bracket
		"branch]",            // bracket
		"branch^",            // caret
		"branch~",            // tilde
		"branch:",            // colon
	}

	for _, branch := range invalidBranches {
		t.Run(fmt.Sprintf("invalid_%s", branch), func(t *testing.T) {
			err := validateGitBranch(branch)
			if err == nil {
				t.Errorf("Expected branch '%s' to be invalid, but got no error", branch)
			}
		})
	}
}

// TestLoadConfigInvalidBranchName tests that config loading rejects invalid branch names
func TestLoadConfigInvalidBranchName(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	configWithInvalidBranch := `
listen_port: 8080
projects:
  - name: TestProject
    webhook_path: /hooks/test
    webhook_secret: secret_token_123
    git_repo: git@github.com:myorg/repo.git
    git_branch: "invalid;branch"
    execute_command: sh deploy.sh
`

	err := os.WriteFile(configPath, []byte(configWithInvalidBranch), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid git_branch, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "git_branch contains invalid character") {
		t.Errorf("Expected error message about invalid git_branch character, got: %v", err)
	}
}

// TestLoadConfigEnvVariables tests that env_variables are parsed from YAML config
func TestLoadConfigEnvVariables(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sdeploy.conf")

	configWithEnvVars := `
listen_port: 8080
projects:
  - name: EnvVarProject
    webhook_path: /hooks/envtest
    webhook_secret: secret_token_123
    execute_command: sh build.sh
    env_variables:
      - BUILD_DIR=/tmp/mybuild
      - DEPLOY_DIR=/var/www/html/
      - VITE_API_BASE_URL=https://api.example.com/
`

	err := os.WriteFile(configPath, []byte(configWithEnvVars), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	project := cfg.Projects[0]
	if len(project.EnvVariables) != 3 {
		t.Errorf("Expected 3 env_variables, got %d", len(project.EnvVariables))
	}
	if len(project.EnvVariables) > 0 && project.EnvVariables[0] != "BUILD_DIR=/tmp/mybuild" {
		t.Errorf("Expected first env_variable to be 'BUILD_DIR=/tmp/mybuild', got '%s'", project.EnvVariables[0])
	}
	if len(project.EnvVariables) > 1 && project.EnvVariables[1] != "DEPLOY_DIR=/var/www/html/" {
		t.Errorf("Expected second env_variable to be 'DEPLOY_DIR=/var/www/html/', got '%s'", project.EnvVariables[1])
	}
	if len(project.EnvVariables) > 2 && project.EnvVariables[2] != "VITE_API_BASE_URL=https://api.example.com/" {
		t.Errorf("Expected third env_variable to be 'VITE_API_BASE_URL=https://api.example.com/', got '%s'", project.EnvVariables[2])
	}
}
