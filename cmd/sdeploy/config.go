package main

import (
	"fmt"
	"os"
	"strings"

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
	EnvVariables    []string `yaml:"env_variables"`
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

	// Apply defaults and validate, using the shared finalize step so this path
	// cannot drift from LoadMergedConfig.
	if err := finalizeConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadMergedConfig loads and merges multiple YAML configuration files into a
// single in-memory Config. Files are processed in the order given; for any
// mapping key, later files override earlier ones (last-write-wins). Nested maps
// are merged recursively, while slices and scalar values are replaced wholly.
//
// Non-existent or unreadable files are skipped silently so that optional
// overlays (for example a secrets file) may be absent without breaking startup.
// An error is returned only when no valid configuration could be loaded at all.
//
// When a single path is supplied this behaves equivalently to LoadConfig for
// the existing single-file workflow.
func LoadMergedConfig(paths []string) (*Config, error) {
	merged := make(map[string]interface{})

	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		data, err := os.ReadFile(p)
		if err != nil {
			// Skip files that don't exist so optional overlays can be omitted.
			continue
		}

		var partial map[string]interface{}
		if err := yaml.Unmarshal(data, &partial); err != nil {
			return nil, fmt.Errorf("failed to parse config YAML %q: %w", p, err)
		}

		mergeMaps(merged, partial)
	}

	if len(merged) == 0 {
		return nil, fmt.Errorf("no valid configuration files found in: %v", paths)
	}

	mergedData, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to merge configuration: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(mergedData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse merged config: %w", err)
	}

	// Apply defaults and validate via the same shared step used by LoadConfig
	// so the two load paths behave identically for the single-file case.
	if err := finalizeConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// finalizeConfig applies default values and validates a parsed configuration.
// It is shared by LoadConfig and LoadMergedConfig so the two load paths cannot
// drift apart (e.g. a new default added in one place but not the other).
func finalizeConfig(cfg *Config) error {
	if cfg.ListenPort == 0 {
		cfg.ListenPort = Defaults.Port
	}
	return validateConfig(cfg)
}

// mergeMaps merges override into base for the top-level mapping. Nested maps are
// merged recursively (see mergeValue). The `projects` list is handled specially
// by mergeProjects so that projects can be split across files: a project in the
// override that shares a webhook_path with an existing project is patched, while
// a project with a new webhook_path is appended.
func mergeMaps(base, override map[string]interface{}) {
	for key, val := range override {
		switch key {
		case "projects":
			base[key] = mergeProjects(base[key], val)
		default:
			if baseVal, ok := base[key]; ok {
				base[key] = mergeValue(baseVal, val)
			} else {
				base[key] = val
			}
		}
	}
}

// mergeProjects merges two `projects` lists. Projects are matched by their
// `webhook_path` (which is unique per validation). A project in the override
// with a webhook_path already present is deep-merged (patched) into the base
// project; a project with a new webhook_path is appended. This allows different
// files to contribute entirely separate projects while still letting a later
// file override individual fields of an existing project.
func mergeProjects(baseVal, overrideVal interface{}) interface{} {
	baseList, ok1 := baseVal.([]interface{})
	overrideList, ok2 := overrideVal.([]interface{})
	if !ok1 || !ok2 {
		// Not both project lists: fall back to replacing (override wins,
		// or base is kept if the override is not a list).
		if ok2 {
			return overrideVal
		}
		return baseVal
	}

	// Index base projects by webhook_path.
	byPath := make(map[string]int, len(baseList))
	merged := make([]interface{}, len(baseList))
	copy(merged, baseList)
	for i, p := range baseList {
		if m, ok := p.(map[string]interface{}); ok {
			if k, ok := m["webhook_path"].(string); ok && k != "" {
				byPath[k] = i
			}
		}
	}

	for _, p := range overrideList {
		m, ok := p.(map[string]interface{})
		if !ok {
			merged = append(merged, p)
			continue
		}
		if k, ok := m["webhook_path"].(string); ok {
			if i, exists := byPath[k]; exists {
				// Patch existing project (deep merge).
				merged[i] = mergeValue(merged[i], m)
				continue
			}
		}
		// Append new project.
		merged = append(merged, m)
	}

	return merged
}

// mergeValue recursively merges two YAML-derived values.
//   - maps: keys are merged, with nested values merged recursively
//   - slices (including slices of maps, except `projects` which is routed
//     through mergeProjects): replaced wholesale by the override
//   - scalars: the override replaces the base
func mergeValue(base, override interface{}) interface{} {
	switch o := override.(type) {
	case map[string]interface{}:
		if b, ok := base.(map[string]interface{}); ok {
			merged := make(map[string]interface{}, len(b)+len(o))
			for k, v := range b {
				merged[k] = v
			}
			for k, v := range o {
				if existing, ok := merged[k]; ok {
					merged[k] = mergeValue(existing, v)
				} else {
					merged[k] = v
				}
			}
			return merged
		}
		return override
	case []interface{}:
		// Lists (other than projects, handled separately) are replaced
		// wholesale by the override rather than merged element-wise.
		return override
	default:
		return override
	}
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
