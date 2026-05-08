package model

import "time"

type User struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	PasswordHash      string    `json:"password_hash"`
	GitHubAccessToken string    `json:"github_access_token,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ClientRepository struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	RepoURL     string    `json:"repo_url"`
	Branch      string    `json:"branch"`
	Domain      string    `json:"domain"`
	ImageName   string    `json:"image_name"`
	AppType     string    `json:"app_type,omitempty"`
	NodeVersion string    `json:"node_version,omitempty"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
