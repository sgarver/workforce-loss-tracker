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
	query := `SELECT id, provider, provider_id, email, name, avatar_url, created_at FROM users WHERE id = ?`
	err := sqlDB.QueryRow(query, id).Scan(&user.ID, &user.Provider, &user.ProviderID, &user.Email, &user.Name, &user.AvatarURL, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user: %w", err)
	}
	return user, nil
}
