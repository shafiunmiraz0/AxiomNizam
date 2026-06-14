package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"net/smtp"
	"time"
)

// EmailConfig holds email service configuration
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	FromEmail    string
	FromName     string
	RetryCount   int
	RetryDelay   time.Duration
	Timeout      time.Duration
}

// DefaultEmailConfig returns default email configuration
func DefaultEmailConfig() *EmailConfig {
	return &EmailConfig{
		SMTPHost:   "smtp.gmail.com",
		SMTPPort:   587,
		FromEmail:  "noreply@axiomnizam.com",
		FromName:   "Axiom Nizam",
		RetryCount: 3,
		RetryDelay: 5 * time.Second,
		Timeout:    30 * time.Second,
	}
}

// EmailService handles email sending
type EmailService struct {
	config  *EmailConfig
	manager JobManager
}

// NewEmailService creates a new email service
func NewEmailService(config *EmailConfig) *EmailService {
	if config == nil {
		config = DefaultEmailConfig()
	}

	return &EmailService{
		config: config,
	}
}

// SendEmail sends an email directly
func (es *EmailService) SendEmail(to string, subject string, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), es.config.Timeout)
	defer cancel()

	return es.sendWithRetry(ctx, to, subject, body)
}

// sendWithRetry sends email with retry logic
func (es *EmailService) sendWithRetry(ctx context.Context, to string, subject string, body string) error {
	var lastErr error

	for attempt := 0; attempt <= es.config.RetryCount; attempt++ {
		if attempt > 0 {
			logging.Z().Info(fmt.Sprintf("Retrying email send to %s (attempt %d)", to, attempt))
			select {
			case <-time.After(es.config.RetryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := es.send(ctx, to, subject, body)
		if err == nil {
			logging.Z().Info(fmt.Sprintf("Email sent successfully to: %s", to))
			return nil
		}

		lastErr = err
		logging.Z().Info(fmt.Sprintf("Error sending email (attempt %d): %v", attempt+1, err))
	}

	return lastErr
}

// send performs the actual email sending
func (es *EmailService) send(ctx context.Context, to string, subject string, body string) error {
	// Email message format
	message := fmt.Sprintf(
		"From: %s <%s>\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s",
		es.config.FromName,
		es.config.FromEmail,
		to,
		subject,
		body,
	)

	// SMTP server address
	addr := fmt.Sprintf("%s:%d", es.config.SMTPHost, es.config.SMTPPort)

	// Create context with timeout
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Send email
	done := make(chan error, 1)
	go func() {
		err := smtp.SendMail(
			addr,
			smtp.PlainAuth(
				"",
				es.config.SMTPUser,
				es.config.SMTPPassword,
				es.config.SMTPHost,
			),
			es.config.FromEmail,
			[]string{to},
			[]byte(message),
		)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-sendCtx.Done():
		return sendCtx.Err()
	}
}

// EmailJobHandler handles email job processing
type EmailJobHandler struct {
	service *EmailService
}

// NewEmailJobHandler creates a new email job handler
func NewEmailJobHandler(emailService *EmailService) *EmailJobHandler {
	return &EmailJobHandler{
		service: emailService,
	}
}

// Handle processes an email job
func (ejh *EmailJobHandler) Handle(ctx context.Context, job *Job) error {
	// Extract email data
	to, ok := job.Data["to"].(string)
	if !ok || to == "" {
		return fmt.Errorf("missing or invalid 'to' field")
	}

	subject, ok := job.Data["subject"].(string)
	if !ok || subject == "" {
		return fmt.Errorf("missing or invalid 'subject' field")
	}

	body, ok := job.Data["body"].(string)
	if !ok {
		body = ""
	}

	// Log attempt
	logging.Z().Info(fmt.Sprintf("Processing email job %s: sending to %s", job.ID, to))

	// Send email
	err := ejh.service.SendEmail(to, subject, body)
	if err != nil {
		logging.Z().Info(fmt.Sprintf("Email job %s failed: %v", job.ID, err))
		return err
	}

	// Store result
	job.Result = map[string]interface{}{
		"sent_at": time.Now(),
		"to":      to,
		"status":  "sent",
	}

	logging.Z().Info(fmt.Sprintf("Email job %s completed successfully", job.ID))
	return nil
}

// EmailTemplate represents an email template
type EmailTemplate struct {
	ID      string
	Subject string
	Body    string
}

// EmailTemplateService manages email templates
type EmailTemplateService struct {
	templates map[string]*EmailTemplate
}

// NewEmailTemplateService creates a new email template service
func NewEmailTemplateService() *EmailTemplateService {
	return &EmailTemplateService{
		templates: make(map[string]*EmailTemplate),
	}
}

// RegisterTemplate registers an email template
func (ets *EmailTemplateService) RegisterTemplate(id string, subject string, body string) {
	ets.templates[id] = &EmailTemplate{
		ID:      id,
		Subject: subject,
		Body:    body,
	}
	logging.Z().Info(fmt.Sprintf("Template registered: %s", id))
}

// GetTemplate retrieves an email template
func (ets *EmailTemplateService) GetTemplate(id string) (*EmailTemplate, error) {
	template, exists := ets.templates[id]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", id)
	}
	return template, nil
}

// ListTemplates lists all templates
func (ets *EmailTemplateService) ListTemplates() []string {
	var ids []string
	for id := range ets.templates {
		ids = append(ids, id)
	}
	return ids
}

// Common Email Templates
var CommonEmailTemplates = map[string]map[string]string{
	"welcome": {
		"subject": "Welcome to Axiom Nizam",
		"body": `<html>
<body style="font-family: Arial, sans-serif;">
  <h2>Welcome!</h2>
  <p>Thank you for joining Axiom Nizam.</p>
  <p>You can now access all features of our platform.</p>
  <p>Best regards,<br>The Axiom Nizam Team</p>
</body>
</html>`,
	},
	"password_reset": {
		"subject": "Password Reset Request",
		"body": `<html>
<body style="font-family: Arial, sans-serif;">
  <h2>Password Reset</h2>
  <p>You requested a password reset.</p>
  <p><a href="{{ .ResetLink }}">Click here to reset your password</a></p>
  <p>This link expires in 24 hours.</p>
  <p>Best regards,<br>The Axiom Nizam Team</p>
</body>
</html>`,
	},
	"verification": {
		"subject": "Verify Your Email Address",
		"body": `<html>
<body style="font-family: Arial, sans-serif;">
  <h2>Email Verification</h2>
  <p>Please verify your email address.</p>
  <p><a href="{{ .VerificationLink }}">Click here to verify</a></p>
  <p>Best regards,<br>The Axiom Nizam Team</p>
</body>
</html>`,
	},
	"notification": {
		"subject": "Important Notification",
		"body": `<html>
<body style="font-family: Arial, sans-serif;">
  <h2>{{ .Title }}</h2>
  <p>{{ .Message }}</p>
  <p>Best regards,<br>The Axiom Nizam Team</p>
</body>
</html>`,
	},
}

// InitializeEmailTemplates initializes common templates
func InitializeEmailTemplates(ets *EmailTemplateService) {
	for id, template := range CommonEmailTemplates {
		ets.RegisterTemplate(id, template["subject"], template["body"])
	}
}

// BulkEmailJob submits multiple email jobs
type BulkEmailJob struct {
	Recipients []string
	Subject    string
	Body       string
	Manager    JobManager
}

// NewBulkEmailJob creates a new bulk email job
func NewBulkEmailJob(recipients []string, subject string, body string, manager JobManager) *BulkEmailJob {
	return &BulkEmailJob{
		Recipients: recipients,
		Subject:    subject,
		Body:       body,
		Manager:    manager,
	}
}

// Submit submits all emails
func (bej *BulkEmailJob) Submit(ctx context.Context) ([]string, error) {
	var jobIDs []string

	for _, recipient := range bej.Recipients {
		job := CreateJobWithPriority(
			JobTypeEmail,
			map[string]interface{}{
				"to":      recipient,
				"subject": bej.Subject,
				"body":    bej.Body,
			},
			PriorityNormal,
		)

		if bej.Manager != nil {
			if _, err := bej.Manager.SubmitJob(job); err != nil {
				logging.Z().Info(fmt.Sprintf("Error submitting email to %s: %v", recipient, err))
				continue
			}
		}

		jobIDs = append(jobIDs, job.ID)
	}

	logging.Z().Info(fmt.Sprintf("Submitted %d email jobs out of %d recipients", len(jobIDs), len(bej.Recipients)))
	return jobIDs, nil
}
