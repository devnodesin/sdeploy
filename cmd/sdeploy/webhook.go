package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// TriggerSource represents the source of a deployment trigger
type TriggerSource string

const (
	TriggerWebhook  TriggerSource = "WEBHOOK"
	TriggerInternal TriggerSource = "INTERNAL"
)

// WebhookHandler handles incoming webhook requests
type WebhookHandler struct {
	configManager *ConfigManager
	logger        *Logger
	deployer      *Deployer
	// Legacy fields for backward compatibility when ConfigManager is not used
	config   *Config
	projects map[string]*ProjectConfig
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(config *Config, logger *Logger) *WebhookHandler {
	h := &WebhookHandler{
		config:   config,
		logger:   logger,
		projects: make(map[string]*ProjectConfig),
	}

	// Build project lookup map by webhook path
	for i := range config.Projects {
		h.projects[config.Projects[i].WebhookPath] = &config.Projects[i]
	}

	return h
}

// NewWebhookHandlerWithConfigManager creates a webhook handler with hot reload support
func NewWebhookHandlerWithConfigManager(cm *ConfigManager, logger *Logger) *WebhookHandler {
	return &WebhookHandler{
		configManager: cm,
		logger:        logger,
	}
}

// SetDeployer sets the deployer for handling deployments
func (h *WebhookHandler) SetDeployer(deployer *Deployer) {
	h.deployer = deployer
}

// getProject looks up a project by webhook path, supporting both hot reload and legacy modes
func (h *WebhookHandler) getProject(path string) *ProjectConfig {
	if h.configManager != nil {
		return h.configManager.GetProject(path)
	}
	// Fallback to legacy static map
	return h.projects[path]
}

// ServeHTTP implements http.Handler
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find project by path (supports hot reload)
	project := h.getProject(r.URL.Path)
	if project == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate JSON (at least check it's valid)
	var jsonCheck map[string]interface{}
	if err := json.Unmarshal(body, &jsonCheck); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Authenticate and determine trigger source
	triggerSource, authenticated := h.authenticate(r, body, project)
	if !authenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract branch from payload
	branch := extractBranchFromPayload(body)

	// Determine enhanced trigger source
	var enhancedTriggerSource string
	if triggerSource == TriggerWebhook {
		source := determineTriggerSource(body)
		enhancedTriggerSource = string(triggerSource) + " (" + source + ")"
	} else if triggerSource == TriggerInternal {
		// For INTERNAL triggers, check if triggered_by is present in payload
		source := determineTriggerSource(body)
		if source != "" && source != "unknown" {
			// If triggered_by is present, format as WEBHOOK (triggered_by_value)
			enhancedTriggerSource = "WEBHOOK (" + source + ")"
		} else {
			enhancedTriggerSource = string(triggerSource)
		}
	} else {
		enhancedTriggerSource = string(triggerSource)
	}

	// Log the webhook receipt
	if h.logger != nil {
		h.logger.Infof(project.Name, "Received %s trigger for branch: %s", enhancedTriggerSource, branch)
		//print the full payload
		h.logger.Infof(project.Name, "Payload: %s", string(body))
	}

	// Check branch match (for WEBHOOK triggers, we validate branch)
	if triggerSource == TriggerWebhook && project.GitBranch != "" && branch != "" && branch != project.GitBranch {
		if h.logger != nil {
			h.logger.Warnf(project.Name, "Branch mismatch: expected %s, got %s. Skipping.", project.GitBranch, branch)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Accepted (branch mismatch, skipped)"))
		return
	}

	// Trigger deployment asynchronously
	go func() {
		if h.deployer != nil {
			// Use a background context since HTTP request context is canceled after response
			// Deploy already logs start/completion/failure, so no extra logging needed here
			h.deployer.Deploy(context.Background(), project, enhancedTriggerSource)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("Accepted"))
}

// authenticate checks request authentication
func (h *WebhookHandler) authenticate(r *http.Request, body []byte, project *ProjectConfig) (TriggerSource, bool) {
	// First check HMAC signature (X-Hub-Signature-256)
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature != "" {
		if validateHMAC(body, signature, project.WebhookSecret) {
			return TriggerWebhook, true
		}
		return "", false
	}

	// Fallback to secret query parameter
	secret := r.URL.Query().Get("secret")
	if secret != "" {
		if secret == project.WebhookSecret {
			return TriggerInternal, true
		}
		return "", false
	}

	return "", false
}

// validateHMAC validates HMAC-SHA256 signature
func validateHMAC(payload []byte, signature, secret string) bool {
	// Signature format: sha256=<hex>
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	providedMAC, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)

	return hmac.Equal(providedMAC, expectedMAC)
}

// extractBranchFromPayload extracts branch name from webhook payload
func extractBranchFromPayload(payload []byte) string {
	var data struct {
		Ref string `json:"ref"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return ""
	}

	// GitHub format: refs/heads/branch-name
	if strings.HasPrefix(data.Ref, "refs/heads/") {
		return strings.TrimPrefix(data.Ref, "refs/heads/")
	}

	return ""
}

// determineTriggerSource extracts and determines the trigger source from webhook payload
// Logic:
// 1. Use triggered_by if present and not empty
// 2. Else, if sender.url starts with "https://api.github.com/users/", return "Github"
// 3. Else, return "unknown"
func determineTriggerSource(payload []byte) string {
	var data struct {
		TriggeredBy string `json:"triggered_by"`
		Sender      struct {
			URL string `json:"url"`
		} `json:"sender"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return "unknown"
	}

	// Priority 1: Use triggered_by if present and not empty
	if data.TriggeredBy != "" {
		return data.TriggeredBy
	}

	// Priority 2: Check sender.url for GitHub user
	if strings.HasPrefix(data.Sender.URL, "https://api.github.com/users/") {
		return "Github"
	}

	// Default: unknown
	return "unknown"
}
