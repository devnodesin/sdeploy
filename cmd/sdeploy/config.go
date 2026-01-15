package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Defaults holds all default configuration values in a single struct
// Access via: Defaults.Port, Defaults.LogPath, etc.
var Defaults = struct {
	Port      int
	LogPath   string
	GitBranch string
}{
	Port:      8080,
	LogPath:   "/var/log/sdeploy",
	GitBranch: "main",
}

// ConfigSearchPaths defines the search order for config files
var ConfigSearchPaths = []string{
	"/etc/sdeploy.conf",
	"./sdeploy.conf",
}

// EmailConfig holds global email/SMTP configuration
type EmailConfig struct {
	SMTPHost    string `yaml:"smtp_host"`
	SMTPPort    int    `yaml:"smtp_port"`
	SMTPUser    string `yaml:"smtp_user"`
	SMTPPass    string `yaml:"smtp_pass"`
	EmailSender string `yaml:"email_sender"`
}

// ProjectConfig holds configuration for a single project
type ProjectConfig struct {
	Name            string   `yaml:"name"`
	WebhookPath     string   `yaml:"webhook_path"`
	WebhookSecret   string   `yaml:"webhook_secret"`
	GitRepo         string   `yaml:"git_repo"`
	LocalPath       string   `yaml:"local_path"`
	ExecutePath     string   `yaml:"execute_path"`
	GitBranch       string   `yaml:"git_branch"`
	ExecuteCommand  string   `yaml:"execute_command"`
	GitUpdate       bool     `yaml:"git_update"`
	GitSSHKeyPath   string   `yaml:"git_ssh_key_path"`
	TimeoutSeconds  int      `yaml:"timeout_seconds"`
	EmailRecipients []string `yaml:"email_recipients"`
}

// Config holds the complete SDeploy configuration
type Config struct {
	ListenPort  int             `yaml:"listen_port"`
	LogPath     string          `yaml:"log_path"`
	EmailConfig *EmailConfig    `yaml:"email_config"`
	Projects    []ProjectConfig `yaml:"projects"`
}

// LoadConfig loads and validates a configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// Set default listen port if not specified in config
	if cfg.ListenPort == 0 {
		cfg.ListenPort = Defaults.Port
	}

	// Validate the configuration
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateConfig performs validation checks on the configuration
func validateConfig(cfg *Config) error {
	// Check for at least one project (optional, but need to validate projects if present)
	webhookPaths := make(map[string]bool)

	// Note: Using pointer to project (not range value) to allow modification of slice elements
	for i := range cfg.Projects {
		project := &cfg.Projects[i]

		// Validate required fields
		if project.WebhookPath == "" {
			return fmt.Errorf("project %d: webhook_path is required", i+1)
		}

		if project.WebhookSecret == "" {
			return fmt.Errorf("project %d (%s): webhook_secret is required", i+1, project.Name)
		}

		if project.ExecuteCommand == "" {
			return fmt.Errorf("project %d (%s): execute_command is required", i+1, project.Name)
		}

		// Check for duplicate webhook paths
		if webhookPaths[project.WebhookPath] {
			return fmt.Errorf("duplicate webhook_path: %s", project.WebhookPath)
		}
		webhookPaths[project.WebhookPath] = true

		// Default git_branch to Defaults.GitBranch if not set
		if project.GitBranch == "" {
			project.GitBranch = Defaults.GitBranch
		}

		// Validate git_branch format (basic validation to prevent command injection)
		if err := validateGitBranch(project.GitBranch); err != nil {
			return fmt.Errorf("project %d (%s): %v", i+1, project.Name, err)
		}

		// Validate git_ssh_key_path if provided
		if project.GitSSHKeyPath != "" {
			if err := validateSSHKeyPath(project.GitSSHKeyPath); err != nil {
				return fmt.Errorf("project %d (%s): %v", i+1, project.Name, err)
			}
		}
	}

	return nil
}

// validateGitBranch validates that a git branch name is safe to use
func validateGitBranch(branch string) error {
	if branch == "" {
		return fmt.Errorf("git_branch cannot be empty")
	}

	// Check for dangerous characters that could be used for command injection
	// Git branch names cannot contain: space, ~, ^, :, ?, *, [, \, and some others
	// We'll be restrictive and only allow alphanumeric, dash, underscore, slash, and dot
	for _, char := range branch {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '/' || char == '.') {
			return fmt.Errorf("git_branch contains invalid character '%c': branch names must only contain letters, numbers, dash, underscore, slash, or dot", char)
		}
	}

	return nil
}

// validateSSHKeyPath validates that the SSH key file exists and is readable
func validateSSHKeyPath(keyPath string) error {
	// Check if file exists
	info, err := os.Stat(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("git_ssh_key_path file does not exist: %s", keyPath)
		}
		return fmt.Errorf("git_ssh_key_path file error: %v", err)
	}

	// Check if it's a file (not a directory)
	if info.IsDir() {
		return fmt.Errorf("git_ssh_key_path must be a file, not a directory: %s", keyPath)
	}

	// Check if file is readable
	file, err := os.Open(keyPath)
	if err != nil {
		return fmt.Errorf("git_ssh_key_path file is not readable: %v", err)
	}
	file.Close()

	return nil
}

// FindConfigFile finds a config file based on the search order:
// 1. Explicit path from -c flag
// 2. Paths in ConfigSearchPaths (e.g., /etc/sdeploy.conf, ./sdeploy.conf)
func FindConfigFile(explicitPath string) string {
	// If explicit path is provided, use it
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err == nil {
			return explicitPath
		}
		return ""
	}

	// Search order for config file
	for _, path := range ConfigSearchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// IsEmailConfigValid checks if the email configuration is valid and complete
func IsEmailConfigValid(cfg *EmailConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.SMTPHost == "" {
		return false
	}
	if cfg.SMTPPort == 0 {
		return false
	}
	if cfg.SMTPUser == "" {
		return false
	}
	if cfg.SMTPPass == "" {
		return false
	}
	if cfg.EmailSender == "" {
		return false
	}
	return true
}
