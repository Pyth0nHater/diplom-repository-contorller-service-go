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

type RepositoryController struct {
	repoService        *service.RepositoryService
	deploymentService  *service.DeploymentService
}

type SaveRepositoryRequest struct {
	Name        string `json:"name" example:"landing-page"`
	Provider    string `json:"provider" example:"github"`
	RepoURL     string `json:"repo_url" example:"https://github.com/acme/landing-page"`
	Branch      string `json:"branch" example:"main"`
	Domain      string `json:"domain" example:"landing.example.com"`
	ImageName   string `json:"image_name" example:"landing-page"`
	AppType     string `json:"app_type" example:"auto"`
	NodeVersion string `json:"node_version" example:"20"`
	Description string `json:"description" example:"Marketing landing"`
}

type RepositoryResponse struct {
	ID          string `json:"id" example:"repo_123"`
	Name        string `json:"name" example:"landing-page"`
	Provider    string `json:"provider" example:"github"`
	RepoURL     string `json:"repo_url" example:"https://github.com/acme/landing-page"`
	Branch      string `json:"branch" example:"main"`
	Domain      string `json:"domain" example:"landing.example.com"`
	ImageName   string `json:"image_name" example:"landing-page"`
	AppType     string `json:"app_type" example:"auto"`
	NodeVersion string `json:"node_version" example:"20"`
	Description string `json:"description" example:"Marketing landing"`
	CreatedAt   string `json:"created_at" example:"2026-04-17T12:00:00Z"`
	UpdatedAt   string `json:"updated_at" example:"2026-04-17T12:00:00Z"`
}

func NewRepositoryController(repoService *service.RepositoryService, deploymentService *service.DeploymentService) *RepositoryController {
	return &RepositoryController{repoService: repoService, deploymentService: deploymentService}
}

// Create godoc
// @Summary Create repository
// @Description Add a repository for the authenticated user
// @Tags repositories
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body SaveRepositoryRequest true "Repository payload"
// @Success 201 {object} RepositoryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /repositories [post]
func (r *RepositoryController) Create(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	var req SaveRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request payload"})
		return
	}

	repo, err := r.repoService.Create(userID, toSaveRepositoryInput(req))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, toRepositoryResponse(repo))
}

// List godoc
// @Summary List repositories
// @Description List repositories for the authenticated user
// @Tags repositories
// @Security BearerAuth
// @Produce json
// @Success 200 {array} RepositoryResponse
// @Failure 401 {object} ErrorResponse
// @Router /repositories [get]
func (r *RepositoryController) List(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	repos, err := r.repoService.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to list repositories"})
		return
	}

	response := make([]RepositoryResponse, 0, len(repos))
	for _, repo := range repos {
		response = append(response, toRepositoryResponse(repo))
	}

	c.JSON(http.StatusOK, response)
}

// Get godoc
// @Summary Get repository
// @Description Get one repository for the authenticated user
// @Tags repositories
// @Security BearerAuth
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} RepositoryResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /repositories/{id} [get]
func (r *RepositoryController) Get(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	repoID := c.Param("id")

	repo, err := r.repoService.Get(userID, repoID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "repository not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to get repository"})
		return
	}

	c.JSON(http.StatusOK, toRepositoryResponse(repo))
}

// Update godoc
// @Summary Update repository
// @Description Update repository for the authenticated user
// @Tags repositories
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param request body SaveRepositoryRequest true "Repository payload"
// @Success 200 {object} RepositoryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /repositories/{id} [put]
func (r *RepositoryController) Update(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	repoID := c.Param("id")

	var req SaveRepositoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request payload"})
		return
	}

	repo, err := r.repoService.Update(userID, repoID, toSaveRepositoryInput(req))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "repository not found"})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, toRepositoryResponse(repo))
}

// Delete godoc
// @Summary Delete repository
// @Description Delete repository for the authenticated user
// @Tags repositories
// @Security BearerAuth
// @Produce json
// @Param id path string true "Repository ID"
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /repositories/{id} [delete]
func (r *RepositoryController) Delete(c *gin.Context) {
	userID := c.GetString(middleware.UserIDContextKey)
	repoID := c.Param("id")

	// stop and remove the container before deleting the record
	_ = r.deploymentService.Undeploy(c.Request.Context(), userID, repoID)

	if err := r.repoService.Delete(userID, repoID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "repository not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to delete repository"})
		return
	}

	c.Status(http.StatusNoContent)
}

func toSaveRepositoryInput(req SaveRepositoryRequest) service.SaveRepositoryInput {
	return service.SaveRepositoryInput{
		Name:        req.Name,
		Provider:    req.Provider,
		RepoURL:     req.RepoURL,
		Branch:      req.Branch,
		Domain:      req.Domain,
		ImageName:   req.ImageName,
		AppType:     req.AppType,
		NodeVersion: req.NodeVersion,
		Description: req.Description,
	}
}

func toRepositoryResponse(repo model.ClientRepository) RepositoryResponse {
	return RepositoryResponse{
		ID:          repo.ID,
		Name:        repo.Name,
		Provider:    repo.Provider,
		RepoURL:     repo.RepoURL,
		Branch:      repo.Branch,
		Domain:      repo.Domain,
		ImageName:   repo.ImageName,
		AppType:     repo.AppType,
		NodeVersion: repo.NodeVersion,
		Description: repo.Description,
		CreatedAt:   repo.CreatedAt.Format(timeLayout),
		UpdatedAt:   repo.UpdatedAt.Format(timeLayout),
	}
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
