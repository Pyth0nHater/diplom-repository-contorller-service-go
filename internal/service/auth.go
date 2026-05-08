package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"repository-contorller-service-go/internal/model"
	"repository-contorller-service-go/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrGitHubTokenNotConfigured = errors.New("github access token is not connected")

type AuthService struct {
	users    repository.UserRepository
	secret   []byte
	tokenTTL time.Duration
}

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

func NewAuthService(users repository.UserRepository, secret string, tokenTTL time.Duration) *AuthService {
	return &AuthService{
		users:    users,
		secret:   []byte(secret),
		tokenTTL: tokenTTL,
	}
}

func (s *AuthService) Register(input RegisterInput) (model.User, string, error) {
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := strings.TrimSpace(input.Password)

	if name == "" || email == "" || password == "" {
		return model.User{}, "", errors.New("name, email and password are required")
	}
	if len(password) < 8 {
		return model.User{}, "", errors.New("password must be at least 8 characters")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, "", fmt.Errorf("hash password: %w", err)
	}

	user := model.User{
		ID:           newID("usr"),
		Name:         name,
		Email:        email,
		PasswordHash: string(passwordHash),
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.users.CreateUser(user); err != nil {
		return model.User{}, "", err
	}

	token, err := s.issueToken(user)
	if err != nil {
		return model.User{}, "", err
	}

	return user, token, nil
}

func (s *AuthService) Login(input LoginInput) (model.User, string, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	password := strings.TrimSpace(input.Password)

	if email == "" || password == "" {
		return model.User{}, "", errors.New("email and password are required")
	}

	user, err := s.users.GetUserByEmail(email)
	if err != nil {
		return model.User{}, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return model.User{}, "", errors.New("invalid credentials")
	}

	token, err := s.issueToken(user)
	if err != nil {
		return model.User{}, "", err
	}

	return user, token, nil
}

func (s *AuthService) GetUserByID(id string) (model.User, error) {
	return s.users.GetUserByID(id)
}

func (s *AuthService) SaveGitHubAccessToken(userID, accessToken string) error {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return errors.New("access_token is required")
	}

	return s.users.UpdateUserGitHubAccessToken(userID, accessToken)
}

func (s *AuthService) DeleteGitHubAccessToken(userID string) error {
	return s.users.UpdateUserGitHubAccessToken(userID, "")
}

func (s *AuthService) ParseToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}

	subject, err := claims.GetSubject()
	if err != nil || subject == "" {
		return "", errors.New("token subject is empty")
	}

	return subject, nil
}

func (s *AuthService) issueToken(user model.User) (string, error) {
	now := time.Now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   user.ID,
		ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(now),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func newID(prefix string) string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return prefix + "_" + hex.EncodeToString(buf)
}
