package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// EmailConfig holds global email/SMTP configuration
type EmailConfig struct {
	SMTPHost    string `json:"smtp_host"`
	SMTPPort    int    `json:"smtp_port"`
	SMTPUser    string `json:"smtp_user"`
	SMTPPass    string `json:"smtp_pass"`
	EmailSender string `json:"email_sender"`
}

// ProjectConfig holds configuration for a single project
type ProjectConfig struct {
	Name            string   `json:"name"`
	WebhookPath     string   `json:"webhook_path"`
	WebhookSecret   string   `json:"webhook_secret"`
	GitRepo         string   `json:"git_repo"`
	GitPath         string   `json:"git_path"`
	ExecutePath     string   `json:"execute_path"`
	GitBranch       string   `json:"git_branch"`
	ExecuteCommand  string   `json:"execute_command"`
	GitUpdate       bool     `json:"git_update"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	EmailRecipients []string `json:"email_recipients"`
}

// Config holds the complete SDeploy configuration
type Config struct {
	ListenPort  int             `json:"listen_port"`
	LogFilepath string          `json:"log_filepath"`
	EmailConfig *EmailConfig    `json:"email_config"`
	Projects    []ProjectConfig `json:"projects"`
}

// LoadConfig loads and validates a configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Set default listen port if not specified
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 8080
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

	for i, project := range cfg.Projects {
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
	}

	return nil
}

// FindConfigFile finds a config file based on the search order:
// 1. Explicit path from -c flag
// 2. /etc/sdeploy/config.json
// 3. ./config.json
func FindConfigFile(explicitPath string) string {
	// If explicit path is provided, use it
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err == nil {
			return explicitPath
		}
		return ""
	}

	// Search order for config file
	searchPaths := []string{
		"/etc/sdeploy/config.json",
		"./config.json",
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
