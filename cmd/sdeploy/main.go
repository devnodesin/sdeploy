package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
)

const (
	Version     = "v1.0"
	ServiceName = "SDeploy"
)

func main() {
	// Parse command line flags
	configPath := flag.String("c", "", "Path to config file")
	daemonMode := flag.Bool("d", false, "Run as daemon (background service)")
	showHelp := flag.Bool("h", false, "Show help")
	flag.Parse()

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	// Find config file
	cfgPath := FindConfigFile(*configPath)
	if cfgPath == "" {
		fmt.Fprintln(os.Stderr, "Error: No config file found")
		fmt.Fprintln(os.Stderr, "Searched: -c flag, /etc/sdeploy/config.json, ./config.json")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	var logger *Logger
	if *daemonMode && cfg.LogFilepath != "" {
		logger = NewLogger(nil, cfg.LogFilepath)
	} else {
		logger = NewLogger(os.Stdout, "")
	}
	defer logger.Close()

	logger.Infof("", "%s %s - Service started", ServiceName, Version)

	// Log configuration summary
	logConfigSummary(logger, cfg)

	// Initialize email notifier
	var notifier *EmailNotifier
	if IsEmailConfigValid(cfg.EmailConfig) {
		notifier = NewEmailNotifier(cfg.EmailConfig, logger)
		logger.Info("", "Email notifications enabled")
	} else {
		logger.Info("", "Email notification disabled: email_config is missing or invalid.")
	}

	// Initialize deployer
	deployer := NewDeployer(logger)
	deployer.SetNotifier(notifier)

	// Initialize webhook handler
	handler := NewWebhookHandler(cfg, logger)
	handler.SetDeployer(deployer)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, getShutdownSignals()...)

	// Start HTTP server in goroutine
	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		logger.Infof("", "Server starting on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("", "Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Infof("", "Received signal %v, shutting down...", sig)

	// Graceful shutdown
	if err := server.Close(); err != nil {
		logger.Errorf("", "Error during shutdown: %v", err)
	}

	logger.Infof("", "%s %s - Service terminated", ServiceName, Version)
}

// logConfigSummary logs all configuration settings on startup
func logConfigSummary(logger *Logger, cfg *Config) {
	logger.Info("", "Configuration loaded:")
	logger.Infof("", "  Listen Port: %d", cfg.ListenPort)
	if cfg.LogFilepath != "" {
		logger.Infof("", "  Log File: %s", cfg.LogFilepath)
	}
	if IsEmailConfigValid(cfg.EmailConfig) {
		logger.Info("", "  Email Notifications: enabled")
	} else {
		logger.Info("", "  Email Notifications: disabled")
	}

	for i, project := range cfg.Projects {
		logger.Infof("", "Project [%d]: %s", i+1, project.Name)
		logger.Infof("", "  - Webhook Path: %s", project.WebhookPath)
		// Print Webhook URL with curl example
		logger.Infof("", "  - Webhook URL: curl -X POST \"http://<YOUR_HOST>:%d%s?secret=%s\" -d '{\"ref\":\"refs/heads/%s\"}'",
			cfg.ListenPort, project.WebhookPath, project.WebhookSecret, project.GitBranch)
		// Order: Git Repo, Git Branch, Git Update, Local Path, Execute Path, Execute Command
		if project.GitRepo != "" {
			logger.Infof("", "  - Git Repo: %s", project.GitRepo)
		}
		logger.Infof("", "  - Git Branch: %s", project.GitBranch)
		logger.Infof("", "  - Git Update: %t", project.GitUpdate)
		if project.LocalPath != "" {
			logger.Infof("", "  - Local Path: %s", project.LocalPath)
		}
		if project.ExecutePath != "" {
			logger.Infof("", "  - Execute Path: %s", project.ExecutePath)
		}
		logger.Infof("", "  - Execute Command: %s", project.ExecuteCommand)
		// Show run as user/group if configured
		runAsUser := project.RunAsUser
		if runAsUser == "" {
			runAsUser = "www-data"
		}
		runAsGroup := project.RunAsGroup
		if runAsGroup == "" {
			runAsGroup = "www-data"
		}
		logger.Infof("", "  - Run As: %s:%s", runAsUser, runAsGroup)
		if project.TimeoutSeconds > 0 {
			logger.Infof("", "  - Timeout: %ds", project.TimeoutSeconds)
		}
		logger.Infof("", "  - Email Recipients: %d", len(project.EmailRecipients))
	}
}

// printUsage prints the help message
func printUsage() {
	fmt.Printf("%s %s - Simple Webhook Deployment Daemon\n", ServiceName, Version)
	fmt.Println()
	fmt.Println("Usage: sdeploy [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -c <path>  Path to config file")
	fmt.Println("  -d         Run as daemon (background service)")
	fmt.Println("  -h         Show this help message")
	fmt.Println()
	fmt.Println("Config file search order:")
	fmt.Println("  1. Path from -c flag")
	fmt.Println("  2. /etc/sdeploy/config.json")
	fmt.Println("  3. ./config.json")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sdeploy              # Run in console mode")
	fmt.Println("  sdeploy -d           # Run as daemon")
	fmt.Println("  sdeploy -c /path/to/config.json -d")
}
