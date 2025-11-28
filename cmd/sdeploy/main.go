package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	logger.Info("SDeploy", "Starting SDeploy webhook daemon")
	logger.Infof("SDeploy", "Config loaded from: %s", cfgPath)
	logger.Infof("SDeploy", "Listening on port: %d", cfg.ListenPort)
	logger.Infof("SDeploy", "Projects configured: %d", len(cfg.Projects))

	// Initialize email notifier
	var notifier *EmailNotifier
	if cfg.EmailConfig != nil {
		notifier = NewEmailNotifier(cfg.EmailConfig, logger)
		logger.Info("SDeploy", "Email notifications enabled")
	}

	// Initialize deployer
	deployer := NewDeployer(logger)
	deployer.SetNotifier(notifier)

	// Initialize webhook handler
	handler := NewWebhookHandler(cfg, logger)
	handler.SetDeployer(deployer)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in goroutine
	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		logger.Infof("SDeploy", "Server starting on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("SDeploy", "Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Infof("SDeploy", "Received signal %v, shutting down...", sig)

	// Graceful shutdown
	if err := server.Close(); err != nil {
		logger.Errorf("SDeploy", "Error during shutdown: %v", err)
	}

	logger.Info("SDeploy", "SDeploy stopped")
}

// printUsage prints the help message
func printUsage() {
	fmt.Println("SDeploy - Simple Webhook Deployment Daemon")
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

