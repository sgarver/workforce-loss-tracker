package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"layoff-tracker/internal/database"
	"layoff-tracker/internal/models"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	db *database.DB
}

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrEmailNotVerified    = errors.New("email not verified")
	ErrEmailAlreadyInUse   = errors.New("email already in use")
	ErrVerificationInvalid = errors.New("verification token invalid")
	ErrVerificationExpired = errors.New("verification token expired")
	ErrResetInvalid        = errors.New("reset token invalid")
	ErrResetExpired        = errors.New("reset token expired")
)

const passwordCost = 12

func NewUserService(db *database.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) CreateUser(provider, providerID, email, name, avatarURL string) (*models.User, error) {
	provider = strings.TrimSpace(provider)
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || provider == "" {
		return nil, fmt.Errorf("provider and provider_id required")
	}
	email = normalizeEmail(email)
	emailVerified := provider != "email"

	user := &models.User{
		Provider:      provider,
		ProviderID:    providerID,
		Email:         email,
		Name:          name,
		AvatarURL:     avatarURL,
		EmailVerified: emailVerified,
		CreatedAt:     time.Now(),
	}

	sqlDB := s.db.DB
	query := `INSERT INTO users (provider, provider_id, email, name, avatar_url, email_verified, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(provider, provider_id) DO UPDATE SET
	          email = excluded.email, name = excluded.name, avatar_url = excluded.avatar_url, email_verified = excluded.email_verified
	          RETURNING id, created_at`

	err := sqlDB.QueryRow(
		query,
		user.Provider,
		user.ProviderID,
		user.Email,
		user.Name,
		user.AvatarURL,
		boolToInt(user.EmailVerified),
		user.CreatedAt,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("error creating/updating user: %w", err)
	}

	return user, nil
}

func (s *UserService) CreateEmailUser(email, password, name string) (*models.User, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password required")
	}

	if existing, err := s.GetUserByEmail(email); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrEmailAlreadyInUse
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return nil, fmt.Errorf("error hashing password: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("error generating verification token: %w", err)
	}
	verificationExpires := time.Now().Add(24 * time.Hour)

	provider := "email"
	providerID := email
	createdAt := time.Now()

	user := &models.User{
		Provider:              provider,
		ProviderID:            providerID,
		Email:                 email,
		Name:                  name,
		PasswordHash:          sql.NullString{String: string(passwordHash), Valid: true},
		EmailVerified:         false,
		VerificationToken:     sql.NullString{String: token, Valid: true},
		VerificationExpiresAt: sql.NullTime{Time: verificationExpires, Valid: true},
		CreatedAt:             createdAt,
	}

	sqlDB := s.db.DB
	query := `INSERT INTO users (provider, provider_id, email, name, password_hash, email_verified, verification_token, verification_expires_at, created_at)
	          VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)
	          RETURNING id, created_at`

	if err := sqlDB.QueryRow(
		query,
		provider,
		providerID,
		email,
		name,
		string(passwordHash),
		token,
		verificationExpires,
		createdAt,
	).Scan(&user.ID, &user.CreatedAt); err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrEmailAlreadyInUse
		}
		return nil, fmt.Errorf("error creating email user: %w", err)
	}

	return user, nil
}

func (s *UserService) GetUserByProviderID(provider, providerID string) (*models.User, error) {
	user := &models.User{}
	sqlDB := s.db.DB
	query := `SELECT id, provider, provider_id, email, name, avatar_url, is_admin, email_verified, created_at FROM users WHERE provider = ? AND provider_id = ?`
	var avatar sql.NullString
	err := sqlDB.QueryRow(query, provider, providerID).Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &avatar, &user.IsAdmin, &user.EmailVerified, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	if avatar.Valid {
		user.AvatarURL = avatar.String
	}
	return user, nil
}

func (s *UserService) GetUserByID(id int) (*models.User, error) {
	user := &models.User{}
	sqlDB := s.db.DB
	query := `SELECT id, provider, provider_id, email, name, avatar_url, is_admin, email_verified, created_at FROM users WHERE id = ?`
	var avatar sql.NullString
	err := sqlDB.QueryRow(query, id).Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &avatar, &user.IsAdmin, &user.EmailVerified, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	if avatar.Valid {
		user.AvatarURL = avatar.String
	}
	return user, nil
}

func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	email = normalizeEmail(email)
	if email == "" {
		return nil, nil
	}

	user := &models.User{}
	sqlDB := s.db.DB
	query := `SELECT id, provider, provider_id, email, name, avatar_url, is_admin, password_hash, email_verified, verification_token, verification_expires_at, reset_token, reset_expires_at, last_login_at, created_at
		FROM users WHERE email = ?`

	var avatar sql.NullString
	err := sqlDB.QueryRow(query, email).Scan(
		&user.ID,
		&user.Provider,
		&user.ProviderID,
		&user.Email,
		&user.Name,
		&avatar,
		&user.IsAdmin,
		&user.PasswordHash,
		&user.EmailVerified,
		&user.VerificationToken,
		&user.VerificationExpiresAt,
		&user.ResetToken,
		&user.ResetExpiresAt,
		&user.LastLoginAt,
		&user.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user by email: %w", err)
	}
	if avatar.Valid {
		user.AvatarURL = avatar.String
	}
	return user, nil
}

func (s *UserService) AuthenticateEmail(email, password string) (*models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Provider != "email" || !user.PasswordHash.Valid {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if !user.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	if err := s.UpdateLastLogin(user.ID); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) VerifyEmail(token string) (*models.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrVerificationInvalid
	}

	sqlDB := s.db.DB
	var userID int
	var emailVerified bool
	var expires sql.NullTime
	query := `SELECT id, email_verified, verification_expires_at FROM users WHERE verification_token = ?`
	if err := sqlDB.QueryRow(query, token).Scan(&userID, &emailVerified, &expires); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrVerificationInvalid
		}
		return nil, fmt.Errorf("error verifying token: %w", err)
	}

	if emailVerified {
		return s.GetUserByID(userID)
	}
	if !expires.Valid || time.Now().After(expires.Time) {
		return nil, ErrVerificationExpired
	}

	if _, err := sqlDB.Exec(`UPDATE users SET email_verified = 1, verification_token = NULL, verification_expires_at = NULL WHERE id = ?`, userID); err != nil {
		return nil, fmt.Errorf("error updating verification: %w", err)
	}

	return s.GetUserByID(userID)
}

func (s *UserService) StartPasswordReset(email string) (string, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return "", err
	}
	if user == nil || user.Provider != "email" {
		return "", nil
	}

	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("error generating reset token: %w", err)
	}
	resetExpires := time.Now().Add(1 * time.Hour)

	_, err = s.db.Exec(`UPDATE users SET reset_token = ?, reset_expires_at = ? WHERE id = ?`, token, resetExpires, user.ID)
	if err != nil {
		return "", fmt.Errorf("error setting reset token: %w", err)
	}

	return token, nil
}

func (s *UserService) ResetPassword(token, newPassword string) (*models.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrResetInvalid
	}

	sqlDB := s.db.DB
	var userID int
	var expires sql.NullTime
	query := `SELECT id, reset_expires_at FROM users WHERE reset_token = ?`
	if err := sqlDB.QueryRow(query, token).Scan(&userID, &expires); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrResetInvalid
		}
		return nil, fmt.Errorf("error reading reset token: %w", err)
	}

	if !expires.Valid || time.Now().After(expires.Time) {
		return nil, ErrResetExpired
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), passwordCost)
	if err != nil {
		return nil, fmt.Errorf("error hashing password: %w", err)
	}

	_, err = sqlDB.Exec(`UPDATE users SET password_hash = ?, reset_token = NULL, reset_expires_at = NULL WHERE id = ?`, string(passwordHash), userID)
	if err != nil {
		return nil, fmt.Errorf("error updating password: %w", err)
	}

	return s.GetUserByID(userID)
}

func (s *UserService) ResendVerification(email string) (string, *models.User, error) {
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return "", nil, err
	}
	if user == nil || user.Provider != "email" {
		return "", nil, nil
	}
	if user.EmailVerified {
		return "", user, nil
	}

	token, err := generateToken()
	if err != nil {
		return "", nil, fmt.Errorf("error generating verification token: %w", err)
	}
	expires := time.Now().Add(24 * time.Hour)

	if _, err := s.db.Exec(`UPDATE users SET verification_token = ?, verification_expires_at = ? WHERE id = ?`, token, expires, user.ID); err != nil {
		return "", nil, fmt.Errorf("error updating verification token: %w", err)
	}

	user.VerificationToken = sql.NullString{String: token, Valid: true}
	user.VerificationExpiresAt = sql.NullTime{Time: expires, Valid: true}
	return token, user, nil
}

func (s *UserService) UpdateLastLogin(userID int) error {
	_, err := s.db.Exec(`UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("error updating last login: %w", err)
	}
	return nil
}

func (s *UserService) GetAdminUsers() ([]*models.User, error) {
	rows, err := s.db.Query(`SELECT id, provider, provider_id, email, name, avatar_url, is_admin, email_verified, created_at FROM users WHERE is_admin = 1`)
	if err != nil {
		return nil, fmt.Errorf("error querying admin users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		var avatar sql.NullString
		if err := rows.Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &avatar, &user.IsAdmin, &user.EmailVerified, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("error scanning admin user: %w", err)
		}
		if avatar.Valid {
			user.AvatarURL = avatar.String
		}
		users = append(users, &user)
	}

	return users, nil
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

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func generateToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed: users.email") ||
		strings.Contains(message, "UNIQUE constraint failed: users.provider_id") ||
		strings.Contains(message, "UNIQUE constraint failed: users.provider, users.provider_id")
}
