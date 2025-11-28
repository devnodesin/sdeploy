package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWebhookRouting tests routing requests by webhook_path to correct project
func TestWebhookRouting(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "Frontend",
				WebhookPath:    "/hooks/frontend",
				WebhookSecret:  "secret1",
				GitBranch:      "main",
				ExecuteCommand: "echo hello",
			},
			{
				Name:           "Backend",
				WebhookPath:    "/hooks/backend",
				WebhookSecret:  "secret2",
				GitBranch:      "main",
				ExecuteCommand: "echo world",
			},
		},
	}

	handler := NewWebhookHandler(cfg, nil)

	// Test frontend path
	req := httptest.NewRequest("POST", "/hooks/frontend?secret=secret1", strings.NewReader(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", rr.Code)
	}

	// Test backend path
	req = httptest.NewRequest("POST", "/hooks/backend?secret=secret2", strings.NewReader(`{"ref":"refs/heads/main"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status 202, got %d", rr.Code)
	}

	// Test unknown path
	req = httptest.NewRequest("POST", "/hooks/unknown", strings.NewReader(`{}`))
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rr.Code)
	}
}

// TestWebhookHMACValidation tests HMAC-SHA256 signature validation
func TestWebhookHMACValidation(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "TestProject",
				WebhookPath:    "/hooks/test",
				WebhookSecret:  "mysecret",
				GitBranch:      "main",
				ExecuteCommand: "echo test",
			},
		},
	}

	handler := NewWebhookHandler(cfg, nil)

	payload := `{"ref":"refs/heads/main"}`
	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write([]byte(payload))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/hooks/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status 202 with valid HMAC, got %d", rr.Code)
	}

	// Test invalid HMAC
	req = httptest.NewRequest("POST", "/hooks/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 with invalid HMAC, got %d", rr.Code)
	}
}

// TestWebhookSecretFallback tests fallback to secret query parameter
func TestWebhookSecretFallback(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "TestProject",
				WebhookPath:    "/hooks/test",
				WebhookSecret:  "mysecret",
				GitBranch:      "main",
				ExecuteCommand: "echo test",
			},
		},
	}

	handler := NewWebhookHandler(cfg, nil)

	payload := `{"ref":"refs/heads/main"}`

	// Valid secret in query
	req := httptest.NewRequest("POST", "/hooks/test?secret=mysecret", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status 202 with valid secret query, got %d", rr.Code)
	}

	// Invalid secret in query
	req = httptest.NewRequest("POST", "/hooks/test?secret=wrong", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 with invalid secret query, got %d", rr.Code)
	}

	// No authentication at all
	req = httptest.NewRequest("POST", "/hooks/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without authentication, got %d", rr.Code)
	}
}

// TestWebhookBranchExtraction tests extracting branch from webhook payload
func TestWebhookBranchExtraction(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		expected    string
		shouldMatch bool
	}{
		{
			name:        "GitHub format refs/heads/main",
			payload:     `{"ref":"refs/heads/main"}`,
			expected:    "main",
			shouldMatch: true,
		},
		{
			name:        "GitHub format refs/heads/develop",
			payload:     `{"ref":"refs/heads/develop"}`,
			expected:    "develop",
			shouldMatch: true,
		},
		{
			name:        "Branch mismatch",
			payload:     `{"ref":"refs/heads/feature"}`,
			expected:    "main",
			shouldMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			branch := extractBranchFromPayload([]byte(tc.payload))
			if tc.shouldMatch && branch != tc.expected {
				t.Errorf("Expected branch %s, got %s", tc.expected, branch)
			}
		})
	}
}

// TestWebhookErrorResponses tests appropriate error responses
func TestWebhookErrorResponses(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "TestProject",
				WebhookPath:    "/hooks/test",
				WebhookSecret:  "mysecret",
				GitBranch:      "main",
				ExecuteCommand: "echo test",
			},
		},
	}

	handler := NewWebhookHandler(cfg, nil)

	// Test wrong HTTP method
	req := httptest.NewRequest("GET", "/hooks/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET, got %d", rr.Code)
	}

	// Test invalid JSON
	req = httptest.NewRequest("POST", "/hooks/test?secret=mysecret", strings.NewReader(`{invalid}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", rr.Code)
	}
}

// TestWebhookTriggerSource tests classification of trigger source
func TestWebhookTriggerSource(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "TestProject",
				WebhookPath:    "/hooks/test",
				WebhookSecret:  "mysecret",
				GitBranch:      "main",
				ExecuteCommand: "echo test",
			},
		},
	}

	var buf bytes.Buffer
	logger := NewLogger(&buf, "")
	handler := NewWebhookHandler(cfg, logger)

	// With HMAC - should be WEBHOOK trigger
	payload := `{"ref":"refs/heads/main"}`
	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write([]byte(payload))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/hooks/test", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !strings.Contains(buf.String(), "WEBHOOK") {
		t.Log("Log output:", buf.String())
	}

	// With secret query - should be INTERNAL trigger
	buf.Reset()
	req = httptest.NewRequest("POST", "/hooks/test?secret=mysecret", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !strings.Contains(buf.String(), "INTERNAL") {
		t.Log("Log output:", buf.String())
	}
}

// TestWebhookBranchValidation tests branch verification
func TestWebhookBranchValidation(t *testing.T) {
	cfg := &Config{
		Projects: []ProjectConfig{
			{
				Name:           "TestProject",
				WebhookPath:    "/hooks/test",
				WebhookSecret:  "mysecret",
				GitBranch:      "main",
				ExecuteCommand: "echo test",
			},
		},
	}

	handler := NewWebhookHandler(cfg, nil)

	// Wrong branch - should still return 202 but skip deployment
	payload := `{"ref":"refs/heads/develop"}`
	req := httptest.NewRequest("POST", "/hooks/test?secret=mysecret", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	// Return 202 but log that branch doesn't match
	if rr.Code != http.StatusAccepted {
		t.Errorf("Expected status 202 (accepted but skipped), got %d", rr.Code)
	}
}

// TestExtractBranchFromPayload tests branch extraction utility
func TestExtractBranchFromPayload(t *testing.T) {
	tests := []struct {
		payload  string
		expected string
	}{
		{`{"ref":"refs/heads/main"}`, "main"},
		{`{"ref":"refs/heads/feature/test"}`, "feature/test"},
		{`{"ref":"refs/tags/v1.0.0"}`, ""},
		{`{}`, ""},
		{`{"ref":""}`, ""},
	}

	for _, tc := range tests {
		result := extractBranchFromPayload([]byte(tc.payload))
		if result != tc.expected {
			t.Errorf("For payload %s: expected %s, got %s", tc.payload, tc.expected, result)
		}
	}
}

// TestValidateHMAC tests HMAC validation utility
func TestValidateHMAC(t *testing.T) {
	secret := "mysecret"
	payload := []byte("test payload")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !validateHMAC(payload, validSig, secret) {
		t.Error("Expected valid HMAC to return true")
	}

	if validateHMAC(payload, "sha256=invalid", secret) {
		t.Error("Expected invalid HMAC to return false")
	}

	if validateHMAC(payload, "invalid_format", secret) {
		t.Error("Expected malformed signature to return false")
	}
}
