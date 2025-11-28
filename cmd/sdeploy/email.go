package main

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// Email represents an email message
type Email struct {
	To      []string
	Subject string
	Body    string
}

// EmailNotifier handles sending email notifications
type EmailNotifier struct {
	config *EmailConfig
	logger *Logger
}

// NewEmailNotifier creates a new email notifier
func NewEmailNotifier(config *EmailConfig, logger *Logger) *EmailNotifier {
	return &EmailNotifier{
		config: config,
		logger: logger,
	}
}

// SendNotification sends a deployment notification email
func (n *EmailNotifier) SendNotification(project *ProjectConfig, result *DeployResult, triggerSource string) error {
	// Skip if no email config or no recipients
	if n.config == nil {
		return nil
	}

	if len(project.EmailRecipients) == 0 {
		return nil
	}

	email := composeDeploymentEmail(project, result, triggerSource)
	email.To = project.EmailRecipients

	return n.send(email)
}

// composeDeploymentEmail creates the email content for a deployment result
func composeDeploymentEmail(project *ProjectConfig, result *DeployResult, triggerSource string) *Email {
	status := "SUCCESS"
	if !result.Success {
		status = "FAILED"
	}

	subject := fmt.Sprintf("[SDeploy] %s - Deployment %s", project.Name, status)

	var body strings.Builder
	body.WriteString(fmt.Sprintf("Project: %s\n", project.Name))
	body.WriteString(fmt.Sprintf("Trigger Source: %s\n", triggerSource))
	body.WriteString(fmt.Sprintf("Branch: %s\n", project.GitBranch))
	body.WriteString(fmt.Sprintf("Status: %s\n", status))
	body.WriteString(fmt.Sprintf("Start Time: %s\n", result.StartTime.Format("2006-01-02 15:04:05")))
	body.WriteString(fmt.Sprintf("End Time: %s\n", result.EndTime.Format("2006-01-02 15:04:05")))
	body.WriteString(fmt.Sprintf("Duration: %v\n", result.Duration()))
	body.WriteString("\n")

	if result.Error != "" {
		body.WriteString(fmt.Sprintf("Error: %s\n", result.Error))
		body.WriteString("\n")
	}

	if result.Output != "" {
		body.WriteString("Output:\n")
		body.WriteString("----------------------------------------\n")
		body.WriteString(result.Output)
		body.WriteString("\n----------------------------------------\n")
	}

	return &Email{
		Subject: subject,
		Body:    body.String(),
	}
}

// send sends an email using SMTP
func (n *EmailNotifier) send(email *Email) error {
	if n.config == nil {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", n.config.SMTPHost, n.config.SMTPPort)

	// Build message
	headers := fmt.Sprintf("From: %s\r\n", n.config.EmailSender)
	headers += fmt.Sprintf("To: %s\r\n", strings.Join(email.To, ", "))
	headers += fmt.Sprintf("Subject: %s\r\n", email.Subject)
	headers += "MIME-Version: 1.0\r\n"
	headers += "Content-Type: text/plain; charset=\"utf-8\"\r\n"
	headers += "\r\n"

	message := headers + email.Body

	// Set up TLS config
	tlsConfig := &tls.Config{
		ServerName: n.config.SMTPHost,
	}

	// Connect to SMTP server
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		// Try without TLS (STARTTLS)
		return n.sendWithSTARTTLS(addr, email, message)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, n.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Authenticate
	auth := smtp.PlainAuth("", n.config.SMTPUser, n.config.SMTPPass, n.config.SMTPHost)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender and recipients
	if err := client.Mail(n.config.EmailSender); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, to := range email.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", to, err)
		}
	}

	// Send message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data connection: %w", err)
	}

	if _, err := w.Write([]byte(message)); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data connection: %w", err)
	}

	return client.Quit()
}

// sendWithSTARTTLS attempts to send using STARTTLS
func (n *EmailNotifier) sendWithSTARTTLS(addr string, email *Email, message string) error {
	auth := smtp.PlainAuth("", n.config.SMTPUser, n.config.SMTPPass, n.config.SMTPHost)

	return smtp.SendMail(addr, auth, n.config.EmailSender, email.To, []byte(message))
}
