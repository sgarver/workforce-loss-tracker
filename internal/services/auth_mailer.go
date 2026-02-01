package services

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

type AuthMailer struct {
	smtpHost  string
	smtpPort  int
	username  string
	password  string
	fromEmail string
	baseURL   string
}

func NewAuthMailerFromEnv() *AuthMailer {
	port := 587
	if rawPort := strings.TrimSpace(os.Getenv("SMTP_PORT")); rawPort != "" {
		if parsed, err := strconv.Atoi(rawPort); err == nil {
			port = parsed
		}
	}

	baseURL := strings.TrimSpace(os.Getenv("BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &AuthMailer{
		smtpHost:  strings.TrimSpace(os.Getenv("SMTP_HOST")),
		smtpPort:  port,
		username:  strings.TrimSpace(os.Getenv("SMTP_USER")),
		password:  strings.TrimSpace(os.Getenv("SMTP_PASS")),
		fromEmail: strings.TrimSpace(os.Getenv("SMTP_FROM")),
		baseURL:   baseURL,
	}
}

func (m *AuthMailer) Configured() bool {
	return m.smtpHost != "" && m.fromEmail != ""
}

func (m *AuthMailer) BaseURL() string {
	return m.baseURL
}

func (m *AuthMailer) SendVerificationEmail(to, name, token string) error {
	verifyLink := fmt.Sprintf("%s/auth/verify?token=%s", m.baseURL, token)
	subject := "Verify your Workforce Loss Tracker account"
	textBody := fmt.Sprintf("Hi %s,\n\nPlease verify your email by clicking the link below:\n%s\n\nIf you did not create an account, you can ignore this email.\n", safeName(name, to), verifyLink)
	htmlBody := fmt.Sprintf("<p>Hi %s,</p><p>Please verify your email by clicking the link below:</p><p><a href=\"%s\">Verify your email</a></p><p>If you did not create an account, you can ignore this email.</p>", safeName(name, to), verifyLink)

	return m.sendEmail(to, subject, textBody, htmlBody)
}

func (m *AuthMailer) SendResetEmail(to, name, token string) error {
	resetLink := fmt.Sprintf("%s/auth/reset?token=%s", m.baseURL, token)
	subject := "Reset your Workforce Loss Tracker password"
	textBody := fmt.Sprintf("Hi %s,\n\nReset your password using the link below:\n%s\n\nIf you did not request a reset, you can ignore this email.\n", safeName(name, to), resetLink)
	htmlBody := fmt.Sprintf("<p>Hi %s,</p><p>Reset your password using the link below:</p><p><a href=\"%s\">Reset password</a></p><p>If you did not request a reset, you can ignore this email.</p>", safeName(name, to), resetLink)

	return m.sendEmail(to, subject, textBody, htmlBody)
}

func (m *AuthMailer) SendFlaggedCommentEmail(to, commentAuthor, companyName, reason, details, commentLink string) error {
	subject := "Comment flagged for review"
	textBody := fmt.Sprintf("A comment was flagged for review.\n\nAuthor: %s\nCompany: %s\nReason: %s\nDetails: %s\nLink: %s\n", commentAuthor, companyName, reason, details, commentLink)
	htmlBody := fmt.Sprintf("<p>A comment was flagged for review.</p><ul><li><strong>Author:</strong> %s</li><li><strong>Company:</strong> %s</li><li><strong>Reason:</strong> %s</li><li><strong>Details:</strong> %s</li><li><strong>Link:</strong> <a href=\"%s\">View comment</a></li></ul>", commentAuthor, companyName, reason, details, commentLink)

	return m.sendEmail(to, subject, textBody, htmlBody)
}

func (m *AuthMailer) sendEmail(to, subject, textBody, htmlBody string) error {
	if m.smtpHost == "" || m.fromEmail == "" {
		log.Printf("SMTP not configured, skipping auth email to %s", to)
		return nil
	}

	boundary := "auth-boundary"
	headers := []string{
		fmt.Sprintf("From: %s", m.fromEmail),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s", boundary),
		"",
	}

	var builder strings.Builder
	builder.WriteString(strings.Join(headers, "\r\n"))
	builder.WriteString("\r\n--" + boundary + "\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	builder.WriteString(textBody)
	builder.WriteString("\r\n--" + boundary + "\r\n")
	builder.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	builder.WriteString(htmlBody)
	builder.WriteString("\r\n--" + boundary + "--\r\n")

	addr := fmt.Sprintf("%s:%d", m.smtpHost, m.smtpPort)
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.smtpHost)
	}

	if err := smtp.SendMail(addr, auth, m.fromEmail, []string{to}, []byte(builder.String())); err != nil {
		return fmt.Errorf("failed to send auth email: %w", err)
	}
	return nil
}

func safeName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}
