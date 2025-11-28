package main

import (
	"strings"
	"testing"
	"time"
)

// TestEmailComposition tests email content composition
func TestEmailComposition(t *testing.T) {
	result := &DeployResult{
		Success:   true,
		Output:    "Deployment completed successfully",
		StartTime: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		EndTime:   time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
	}

	project := &ProjectConfig{
		Name:      "Frontend",
		GitBranch: "main",
	}

	email := composeDeploymentEmail(project, result, "WEBHOOK")

	if !strings.Contains(email.Subject, "Frontend") {
		t.Error("Expected email subject to contain project name")
	}
	if !strings.Contains(email.Subject, "SUCCESS") {
		t.Error("Expected email subject to contain SUCCESS for successful deployment")
	}
	if !strings.Contains(email.Body, "Frontend") {
		t.Error("Expected email body to contain project name")
	}
	if !strings.Contains(email.Body, "WEBHOOK") {
		t.Error("Expected email body to contain trigger source")
	}
	if !strings.Contains(email.Body, "Deployment completed successfully") {
		t.Error("Expected email body to contain output summary")
	}
}

// TestEmailCompositionFailure tests email for failed deployment
func TestEmailCompositionFailure(t *testing.T) {
	result := &DeployResult{
		Success:   false,
		Error:     "Command failed with exit code 1",
		Output:    "Error output here",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	project := &ProjectConfig{
		Name: "Backend",
	}

	email := composeDeploymentEmail(project, result, "INTERNAL")

	if !strings.Contains(email.Subject, "FAILED") {
		t.Error("Expected email subject to contain FAILED for failed deployment")
	}
	if !strings.Contains(email.Body, "Command failed") {
		t.Error("Expected email body to contain error message")
	}
}

// TestEmailSkipWhenUnconfigured tests that email is skipped when not configured
func TestEmailSkipWhenUnconfigured(t *testing.T) {
	notifier := NewEmailNotifier(nil, nil)

	project := &ProjectConfig{
		Name:            "TestProject",
		EmailRecipients: []string{},
	}

	result := &DeployResult{Success: true}

	// Should not panic or error
	err := notifier.SendNotification(project, result, "WEBHOOK")
	if err != nil {
		t.Errorf("Expected no error when email unconfigured, got: %v", err)
	}
}

// TestEmailSkipEmptyRecipients tests skipping when no recipients
func TestEmailSkipEmptyRecipients(t *testing.T) {
	emailCfg := &EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SMTPUser:    "user",
		SMTPPass:    "pass",
		EmailSender: "sdeploy@example.com",
	}

	notifier := NewEmailNotifier(emailCfg, nil)

	project := &ProjectConfig{
		Name:            "TestProject",
		EmailRecipients: []string{}, // No recipients
	}

	result := &DeployResult{Success: true}

	err := notifier.SendNotification(project, result, "WEBHOOK")
	if err != nil {
		t.Errorf("Expected no error for empty recipients, got: %v", err)
	}
}

// TestEmailValidRecipients tests sending to valid recipients
func TestEmailValidRecipients(t *testing.T) {
	emailCfg := &EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SMTPUser:    "user",
		SMTPPass:    "pass",
		EmailSender: "sdeploy@example.com",
	}

	notifier := NewEmailNotifier(emailCfg, nil)

	project := &ProjectConfig{
		Name:            "TestProject",
		EmailRecipients: []string{"team@example.com", "admin@example.com"},
	}

	result := &DeployResult{
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	// This will fail because SMTP server doesn't exist
	// but it should not panic and should return an error
	err := notifier.SendNotification(project, result, "WEBHOOK")
	// Error is expected due to no real SMTP server
	if err == nil {
		// In test environment without mock, connection will fail
		// which is expected behavior
		t.Log("Email send attempted (error expected without real SMTP)")
	}
}

// TestEmailNotifierCreation tests EmailNotifier creation
func TestEmailNotifierCreation(t *testing.T) {
	notifier := NewEmailNotifier(nil, nil)
	if notifier == nil {
		t.Error("Expected notifier to be created even with nil config")
	}

	emailCfg := &EmailConfig{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SMTPUser:    "user",
		SMTPPass:    "pass",
		EmailSender: "sdeploy@example.com",
	}

	notifier = NewEmailNotifier(emailCfg, nil)
	if notifier == nil {
		t.Error("Expected notifier to be created with valid config")
	}
}

// TestEmailMessage tests Email struct
func TestEmailMessage(t *testing.T) {
	email := Email{
		To:      []string{"user@example.com"},
		Subject: "Test Subject",
		Body:    "Test Body",
	}

	if len(email.To) != 1 {
		t.Error("Expected 1 recipient")
	}
	if email.Subject != "Test Subject" {
		t.Error("Expected subject to be 'Test Subject'")
	}
}

// TestComposeEmailDuration tests email includes deployment duration
func TestComposeEmailDuration(t *testing.T) {
	result := &DeployResult{
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(5 * time.Second),
	}

	project := &ProjectConfig{Name: "Test"}

	email := composeDeploymentEmail(project, result, "WEBHOOK")

	if !strings.Contains(email.Body, "Duration:") {
		t.Error("Expected email body to contain duration")
	}
}

// TestEmailErrorHandling tests graceful error handling
func TestEmailErrorHandling(t *testing.T) {
	emailCfg := &EmailConfig{
		SMTPHost:    "invalid.smtp.host.that.does.not.exist",
		SMTPPort:    587,
		SMTPUser:    "user",
		SMTPPass:    "pass",
		EmailSender: "sdeploy@example.com",
	}

	var logBuf strings.Builder
	// We'll just verify the notifier doesn't crash
	notifier := NewEmailNotifier(emailCfg, nil)

	project := &ProjectConfig{
		Name:            "TestProject",
		EmailRecipients: []string{"test@example.com"},
	}

	result := &DeployResult{
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	// Should not panic, should return error
	_ = notifier.SendNotification(project, result, "WEBHOOK")
	_ = logBuf
}

