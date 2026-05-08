package controller

import (
	"errors"
	"net/http"

	"repository-contorller-service-go/internal/middleware"
	"repository-contorller-service-go/internal/model"
	"repository-contorller-service-go/internal/repository"
	"repository-contorller-service-go/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *service.AuthService
}

type RegisterRequest struct {
	Name     string `json:"name" example:"Alice Doe"`
	Email    string `json:"email" example:"alice@example.com"`
	Password string `json:"password" example:"supersecret123"`
}

type LoginRequest struct {
	Email    string `json:"email" example:"alice@example.com"`
	Password string `json:"password" example:"supersecret123"`
}

type UserResponse struct {
	ID              string `json:"id" example:"usr_123"`
	Name            string `json:"name" example:"Alice Doe"`
	Email           string `json:"email" example:"alice@example.com"`
	GitHubConnected bool   `json:"github_connected" example:"true"`
	CreatedAt       string `json:"created_at" example:"2026-04-17T12:00:00Z"`
}

type AuthResponse struct {
	Token       string       `json:"token"`
	AccessToken string       `json:"access_token"`
	User        UserResponse `json:"user"`
}

type ErrorResponse struct {
	Error string `json:"error" example:"invalid credentials"`
}

type SaveGitHubTokenRequest struct {
	AccessToken string `json:"access_token" example:"gho_xxxxxxxxxxxx"`
}

type GitHubTokenStatusResponse struct {
	Connected bool `json:"connected" example:"true"`
}

func NewAuthController(authService *service.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

// Register godoc
// @Summary Register user
// @Description Create a new user account and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration payload"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /auth/register [post]
func (a *AuthController) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request payload"})
		return
	}

	user, token, err := a.authService.Register(service.RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			c.JSON(http.StatusConflict, ErrorResponse{Error: "user with this email already exists"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, AuthResponse{
		Token:       token,
		AccessToken: token,
		User:        toUserResponse(user),
	})
}

// Login godoc
// @Summary Login user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login payload"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/login [post]
func (a *AuthController) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request payload"})
		return
	}

	user, token, err := a.authService.Login(service.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || err.Error() == "invalid credentials" {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid credentials"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Token:       token,
		AccessToken: token,
		User:        toUserResponse(user),
	})
}

// Me godoc
// @Summary Current user
// @Description Return current authenticated user
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/me [get]
func (a *AuthController) Me(c *gin.Context) {
	userID, ok := c.Get(middleware.UserIDContextKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	user, err := a.authService.GetUserByID(userID.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, toUserResponse(user))
}

// SaveGitHubToken godoc
// @Summary Save GitHub token
// @Description Save or replace the GitHub OAuth token for the current authenticated user
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body SaveGitHubTokenRequest true "GitHub token payload"
// @Success 200 {object} GitHubTokenStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/github-token [put]
func (a *AuthController) SaveGitHubToken(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	var req SaveGitHubTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request payload"})
		return
	}

	if err := a.authService.SaveGitHubAccessToken(userID, req.AccessToken); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GitHubTokenStatusResponse{Connected: true})
}

// GetGitHubToken godoc
// @Summary GitHub token status
// @Description Return whether the current authenticated user has a GitHub OAuth token stored in backend
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} GitHubTokenStatusResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/github-token [get]
func (a *AuthController) GetGitHubToken(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	user, err := a.authService.GetUserByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, GitHubTokenStatusResponse{Connected: user.GitHubAccessToken != ""})
}

// DeleteGitHubToken godoc
// @Summary Delete GitHub token
// @Description Remove the stored GitHub OAuth token for the current authenticated user
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} GitHubTokenStatusResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/github-token [delete]
func (a *AuthController) DeleteGitHubToken(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	if err := a.authService.DeleteGitHubAccessToken(userID); err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, GitHubTokenStatusResponse{Connected: false})
}

func toUserResponse(user model.User) UserResponse {
	return UserResponse{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		GitHubConnected: user.GitHubAccessToken != "",
		CreatedAt:       user.CreatedAt.Format(timeLayout),
	}
}
