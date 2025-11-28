package main

import (
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
	config   *Config
	logger   *Logger
	deployer *Deployer
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

// SetDeployer sets the deployer for handling deployments
func (h *WebhookHandler) SetDeployer(deployer *Deployer) {
	h.deployer = deployer
}

// ServeHTTP implements http.Handler
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find project by path
	project, exists := h.projects[r.URL.Path]
	if !exists {
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

	// Log the webhook receipt
	if h.logger != nil {
		h.logger.Infof(project.Name, "Received %s trigger for branch: %s", triggerSource, branch)
	}

	// Check branch match (for WEBHOOK triggers, we validate branch)
	if triggerSource == TriggerWebhook && project.GitBranch != "" && branch != "" && branch != project.GitBranch {
		if h.logger != nil {
			h.logger.Warnf(project.Name, "Branch mismatch: expected %s, got %s. Skipping.", project.GitBranch, branch)
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Accepted (branch mismatch, skipped)"))
		return
	}

	// Trigger deployment asynchronously
	go func() {
		if h.deployer != nil {
			result := h.deployer.Deploy(r.Context(), project, string(triggerSource))
			if h.logger != nil {
				if result.Skipped {
					h.logger.Warnf(project.Name, "Deployment skipped (already in progress)")
				} else if result.Success {
					h.logger.Infof(project.Name, "Deployment completed successfully")
				} else {
					h.logger.Errorf(project.Name, "Deployment failed: %s", result.Error)
				}
			}
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Accepted"))
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

// StartServer starts the webhook HTTP server
func StartServer(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

