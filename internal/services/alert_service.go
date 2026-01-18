package services

import (
	"fmt"
	"log"
	"net/smtp"
)

type AlertService struct {
	userService *UserService
	smtpHost    string
	smtpPort    int
	fromEmail   string
}

func NewAlertService(userService *UserService, smtpHost string, smtpPort int, fromEmail string) *AlertService {
	return &AlertService{
		userService: userService,
		smtpHost:    smtpHost,
		smtpPort:    smtpPort,
		fromEmail:   fromEmail,
	}
}

func (a *AlertService) SendNewDataAlert(userID int, newCount int, lastUpdated string) error {
	prefs, err := a.userService.GetAlertPrefs(userID)
	if err != nil {
		return fmt.Errorf("failed to get alert prefs: %w", err)
	}
	if !prefs.EmailAlertsEnabled || !prefs.AlertNewData {
		return nil // User opted out
	}

	user, err := a.userService.GetUserByID(userID)
	if err != nil || user == nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	log.Printf("Sending new data alert to user %d (%s) for %d new layoffs", userID, user.Email, newCount)

	subject := "New Layoff Data Available on Layoff Tracker"
	body := fmt.Sprintf(`Hi %s,

New layoff data has been imported and is now available on Layoff Tracker!

📊 Update Summary:
- %d new layoffs added
- Last updated: %s

🔍 Check It Out:
Browse the latest data at: http://localhost:8080/tracker

You received this alert because you opted in for new data notifications. To manage your preferences, visit your profile: http://localhost:8080/profile

Best,
Layoff Tracker Team`, user.Name, newCount, lastUpdated)

	err = a.sendEmail(user.Email, subject, body)
	if err != nil {
		log.Printf("Failed to send email to user %d: %v", userID, err)
	} else {
		log.Printf("Successfully sent email alert to user %d", userID)
	}
	return err
}

func (a *AlertService) sendEmail(to, subject, body string) error {
	// Self-hosted SMTP (localhost:25)
	addr := fmt.Sprintf("%s:%d", a.smtpHost, a.smtpPort)
	auth := smtp.PlainAuth("", "", "", a.smtpHost) // No auth for local

	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s\r\n", to, subject, body))

	err := smtp.SendMail(addr, auth, a.fromEmail, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	return nil
}
