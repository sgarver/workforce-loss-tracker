package services

import (
	"database/sql"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"time"
)

type UserService struct {
	db *database.DB
}

func NewUserService(db *database.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateUser(provider, providerID, email, name, avatarURL string) (*models.User, error) {
	user := &models.User{
		Provider:   provider,
		ProviderID: providerID,
		Email:      email,
		Name:       name,
		AvatarURL:  avatarURL,
		CreatedAt:  time.Now(),
	}

	sqlDB := s.db.DB
	query := `INSERT INTO users (provider, provider_id, email, name, avatar_url, created_at)
	          VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(provider, provider_id) DO UPDATE SET
	          email = excluded.email, name = excluded.name, avatar_url = excluded.avatar_url
	          RETURNING id, created_at`

	err := sqlDB.QueryRow(query, user.Provider, user.ProviderID, user.Email, user.Name, user.AvatarURL, user.CreatedAt).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("error creating/updating user: %w", err)
	}

	return user, nil
}

func (s *UserService) GetUserByProviderID(provider, providerID string) (*models.User, error) {
	user := &models.User{}
	sqlDB := s.db.DB
	query := `SELECT id, provider, provider_id, email, name, avatar_url, created_at FROM users WHERE provider = ? AND provider_id = ?`
	err := sqlDB.QueryRow(query, provider, providerID).Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &user.AvatarURL, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	return user, nil
}

func (s *UserService) GetUserByID(id int) (*models.User, error) {
	user := &models.User{}
	sqlDB := s.db.DB
	query := `SELECT id, provider, provider_id, email, name, avatar_url, is_admin, created_at FROM users WHERE id = ?`
	err := sqlDB.QueryRow(query, id).Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &user.AvatarURL, &user.IsAdmin, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	return user, nil
}

type UserAlertPrefs struct {
	UserID             int  `db:"user_id"`
	EmailAlertsEnabled bool `db:"email_alerts_enabled"`
	AlertNewData       bool `db:"alert_new_data"`
}

func (s *UserService) GetAlertPrefs(userID int) (*UserAlertPrefs, error) {
	prefs := &UserAlertPrefs{}
	sqlDB := s.db.DB
	query := `SELECT user_id, email_alerts_enabled, alert_new_data FROM user_alert_prefs WHERE user_id = ?`
	err := sqlDB.QueryRow(query, userID).Scan(&prefs.UserID, &prefs.EmailAlertsEnabled, &prefs.AlertNewData)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return defaults
			return &UserAlertPrefs{UserID: userID, EmailAlertsEnabled: true, AlertNewData: true}, nil
		}
		return nil, fmt.Errorf("error getting alert prefs: %w", err)
	}
	return prefs, nil
}

func (s *UserService) UpdateAlertPrefs(userID int, emailEnabled, alertNewData bool) error {
	sqlDB := s.db.DB
	query := `INSERT INTO user_alert_prefs (user_id, email_alerts_enabled, alert_new_data, updated_at)
	          VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	          ON CONFLICT(user_id) DO UPDATE SET
	          email_alerts_enabled = excluded.email_alerts_enabled,
	          alert_new_data = excluded.alert_new_data,
	          updated_at = CURRENT_TIMESTAMP`
	_, err := sqlDB.Exec(query, userID, emailEnabled, alertNewData)
	if err != nil {
		return fmt.Errorf("error updating alert prefs: %w", err)
	}
	return nil
}

func (s *UserService) GetSystemSetting(key string) (string, error) {
	sqlDB := s.db.DB
	var value string
	query := `SELECT value FROM system_settings WHERE key = ?`
	err := sqlDB.QueryRow(query, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("error getting system setting: %w", err)
	}
	return value, nil
}

func (s *UserService) SetSystemSetting(key, value string) error {
	sqlDB := s.db.DB
	query := `INSERT INTO system_settings (key, value, updated_at)
	          VALUES (?, ?, CURRENT_TIMESTAMP)
	          ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`
	_, err := sqlDB.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("error setting system setting: %w", err)
	}
	return nil
}

func (s *UserService) GetUsersForNewDataAlerts() ([]int, error) {
	sqlDB := s.db.DB
	// Include opted-in users OR admins
	rows, err := sqlDB.Query(`
		SELECT p.user_id FROM user_alert_prefs p
		JOIN users u ON p.user_id = u.id
		WHERE (p.email_alerts_enabled = 1 AND p.alert_new_data = 1) OR u.is_admin = 1
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying users for alerts: %w", err)
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

func (s *UserService) LogSessionEvent(userID int, action, ip, userAgent string) error {
	sqlDB := s.db.DB
	_, err := sqlDB.Exec(`INSERT INTO session_logs (user_id, action, ip_address, user_agent) VALUES (?, ?, ?, ?)`,
		userID, action, ip, userAgent)
	return err
}
