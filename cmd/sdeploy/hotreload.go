package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ConfigManager manages configuration with hot reload support
type ConfigManager struct {
	mu            sync.RWMutex
	config        *Config
	configPath    string
	logger        *Logger
	watcher       *fsnotify.Watcher
	stopChan      chan struct{}
	reloadPending atomic.Bool

	// Callback functions for notifying dependent components
	onReload func(*Config)
}

// NewConfigManager creates a new ConfigManager with hot reload support
func NewConfigManager(configPath string, logger *Logger) (*ConfigManager, error) {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	cm := &ConfigManager{
		config:     cfg,
		configPath: configPath,
		logger:     logger,
		stopChan:   make(chan struct{}),
	}

	return cm, nil
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

	// Start watching the config file
	if err := watcher.Add(cm.configPath); err != nil {
		watcher.Close()
		return err
	}

	if cm.logger != nil {
		cm.logger.Infof("", "Hot reload enabled for config file: %s", cm.configPath)
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

	newConfig, err := LoadConfig(cm.configPath)
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
		logConfigSummary(cm.logger, newConfig)
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
