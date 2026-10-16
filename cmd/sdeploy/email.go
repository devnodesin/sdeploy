package main

import (
	"crypto/tls"
	"fmt"
	"net"
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
	if n.config.InsecureSkipVerify {
		// Skip CA and hostname verification (self-signed certs, e.g. an
		// in-container SMTP relay whose cert is only valid for "localhost").
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.ServerName = ""
	}

	// Connect to SMTP server
	var conn net.Conn
	if n.config.SMTPPort == 465 {
		// Implicit TLS (e.g. port 465): wrap the connection in TLS immediately.
		// The port forces implicit TLS regardless of the use_tls override.
		tlsConn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server over TLS: %w", err)
		}
		conn = tlsConn
	} else {
		// Plain TCP; STARTTLS will be attempted opportunistically below.
		plainConn, err := net.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		conn = plainConn
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, n.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Upgrade the connection to TLS via STARTTLS when enabled and supported.
	if n.config.UseTLSEnabled() {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	// Authenticate only when credentials are provided.
	if n.config.SMTPUser != "" && n.config.SMTPPass != "" {
		auth := smtp.PlainAuth("", n.config.SMTPUser, n.config.SMTPPass, n.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	} else if n.config.SMTPUser != "" || n.config.SMTPPass != "" {
		return fmt.Errorf("smtp_user and smtp_pass must be set together")
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
