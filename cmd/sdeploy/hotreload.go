package main

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ConfigManager manages configuration with hot reload support
type ConfigManager struct {
	mu          sync.RWMutex
	config      *Config
	configPaths []string
	// presentFiles records which listed config files existed at the last
	// successful load. It is used to detect a file that disappears on reload
	// (e.g. a secrets overlay removed or renamed) so operators are warned.
	presentFiles  map[string]bool
	logger        *Logger
	watcher       *fsnotify.Watcher
	stopChan      chan struct{}
	reloadPending atomic.Bool

	// Callback functions for notifying dependent components
	onReload func(*Config)
}

// NewConfigManager creates a new ConfigManager with hot reload support.
// It loads and merges the provided config file paths (later paths override
// earlier ones) and watches all of them for changes.
func NewConfigManager(configPaths []string, logger *Logger) (*ConfigManager, error) {
	// Normalize the path list once: trim whitespace and drop empties.
	normalized := normalizeConfigPaths(configPaths)

	cfg, err := LoadMergedConfig(normalized)
	if err != nil {
		return nil, err
	}

	cm := &ConfigManager{
		config:       cfg,
		configPaths:  normalized,
		presentFiles: presentFiles(normalized),
		logger:       logger,
		stopChan:     make(chan struct{}),
	}

	// Warn once at startup if any config file is writable by group/other, which
	// would let a local attacker create or modify an "optional" overlay and
	// override security-critical fields (webhook_secret, execute_command).
	warnOnInsecurePermissions(cm.configPaths, logger)

	return cm, nil
}

// normalizeConfigPaths trims whitespace and removes empty entries so that
// padded or trailing entries from a `-c` list never reach the file watcher.
func normalizeConfigPaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			normalized = append(normalized, p)
		}
	}
	return normalized
}

// presentFiles returns the subset of paths that currently exist on disk.
func presentFiles(paths []string) map[string]bool {
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			seen[p] = true
		}
	}
	return seen
}

// warnOnInsecurePermissions logs a warning for any config file that is writable
// by group or others, since such a file can be tampered with to alter the
// effective (merged) configuration.
func warnOnInsecurePermissions(paths []string, logger *Logger) {
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		mode := info.Mode().Perm()
		if mode&0002 != 0 || mode&0020 != 0 {
			if logger != nil {
				logger.Warnf("", "Config file %s is group/other writable (mode %o); any local user could modify it and alter the merged configuration", p, mode)
			}
		}
	}
}

// GetConfig returns the current configuration (thread-safe read)
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// GetProject returns a project config by webhook path (thread-safe read)
func (cm *ConfigManager) GetProject(webhookPath string) *ProjectConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for i := range cm.config.Projects {
		if cm.config.Projects[i].WebhookPath == webhookPath {
			return &cm.config.Projects[i]
		}
	}
	return nil
}

// SetOnReload sets the callback function to be called after successful reload
func (cm *ConfigManager) SetOnReload(callback func(*Config)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onReload = callback
}

// StartWatcher starts the file watcher for hot reload
func (cm *ConfigManager) StartWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	cm.watcher = watcher

	// Watch only files that currently exist. Missing files (e.g. an optional
	// secrets overlay that is not mounted) are skipped with a warning rather
	// than aborting the whole watcher, which would otherwise disable hot reload
	// for every file.
	watched := make([]string, 0, len(cm.configPaths))
	for _, p := range cm.configPaths {
		if _, err := os.Stat(p); err != nil {
			if cm.logger != nil {
				cm.logger.Warnf("", "Not watching config file (not found): %s", p)
			}
			continue
		}
		if err := watcher.Add(p); err != nil {
			watcher.Close()
			return err
		}
		watched = append(watched, p)
	}

	if cm.logger != nil {
		cm.logger.Infof("", "Hot reload enabled for config files: %s", strings.Join(watched, ", "))
	}

	go cm.watchLoop()
	return nil
}

// watchLoop handles file system events for the config file
func (cm *ConfigManager) watchLoop() {
	// Debounce timer to handle multiple rapid file changes
	var debounceTimer *time.Timer
	debounceDelay := 500 * time.Millisecond

	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}
			// Handle write and chmod events (editors may use different methods)
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				// Debounce rapid changes
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(debounceDelay, func() {
					cm.triggerReload()
				})
			}
		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			if cm.logger != nil {
				cm.logger.Errorf("", "File watcher error: %v", err)
			}
		case <-cm.stopChan:
			return
		}
	}
}

// triggerReload attempts to reload the configuration
func (cm *ConfigManager) triggerReload() {
	// Check if reload is already pending (due to active deployment)
	if cm.reloadPending.Load() {
		if cm.logger != nil {
			cm.logger.Info("", "Configuration change detected, reload already pending")
		}
		return
	}

	cm.reloadConfig()
}

// reloadConfig loads and validates the new configuration
func (cm *ConfigManager) reloadConfig() {
	if cm.logger != nil {
		cm.logger.Info("", "Reloading configuration...")
	}

	// Detect config files that were present at the last load but are now
	// missing (e.g. a secrets overlay removed or renamed). Silently skipping
	// them would degrade the merged config (dropped webhook_secret/smtp_pass)
	// without alerting anyone, so warn explicitly before re-merging.
	nowPresent := presentFiles(cm.configPaths)
	cm.mu.Lock()
	oldPresent := cm.presentFiles
	cm.presentFiles = nowPresent
	cm.mu.Unlock()
	for f := range oldPresent {
		if !nowPresent[f] {
			if cm.logger != nil {
				cm.logger.Warnf("", "Config file %s is no longer present during reload; its settings (secrets, etc.) will be dropped from the merged configuration", f)
			}
		}
	}

	newConfig, err := LoadMergedConfig(cm.configPaths)
	if err != nil {
		if cm.logger != nil {
			cm.logger.Errorf("", "Failed to reload configuration: %v", err)
		}
		return
	}

	// Check if listen_port changed (not hot-reloadable)
	cm.mu.RLock()
	oldPort := cm.config.ListenPort
	cm.mu.RUnlock()

	if newConfig.ListenPort != oldPort {
		if cm.logger != nil {
			cm.logger.Warnf("", "listen_port changed from %d to %d. Restart required for this change to take effect.", oldPort, newConfig.ListenPort)
		}
	}

	// Apply the new configuration
	cm.mu.Lock()
	cm.config = newConfig
	onReload := cm.onReload
	cm.mu.Unlock()

	if cm.logger != nil {
		cm.logger.Info("", "Configuration reloaded successfully")
		logConfigSummary(cm.logger, newConfig, cm.logger.IsDaemonMode())
	}

	// Notify dependent components
	if onReload != nil {
		onReload(newConfig)
	}
}

// SetReloadPending marks that a reload is pending (called when deployment starts)
func (cm *ConfigManager) SetReloadPending(pending bool) {
	cm.reloadPending.Store(pending)
}

// IsReloadPending returns whether a reload is pending
func (cm *ConfigManager) IsReloadPending() bool {
	return cm.reloadPending.Load()
}

// ProcessPendingReload processes any pending reload (called when deployment completes)
func (cm *ConfigManager) ProcessPendingReload() {
	// Use CompareAndSwap to ensure only one goroutine processes the reload
	if cm.reloadPending.CompareAndSwap(true, false) {
		if cm.logger != nil {
			cm.logger.Info("", "Processing deferred configuration reload")
		}
		cm.reloadConfig()
	}
}

// Stop stops the file watcher
func (cm *ConfigManager) Stop() {
	if cm.watcher != nil {
		close(cm.stopChan)
		cm.watcher.Close()
	}
}
