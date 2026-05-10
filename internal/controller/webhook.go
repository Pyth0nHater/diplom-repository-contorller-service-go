package controller

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"repository-contorller-service-go/internal/repository"
	"repository-contorller-service-go/internal/service"

	"github.com/gin-gonic/gin"
)

type WebhookController struct {
	repos             repository.ClientRepositoryRepository
	deploymentService *service.DeploymentService
	secret            string
}

func NewWebhookController(
	repos repository.ClientRepositoryRepository,
	deploymentService *service.DeploymentService,
	secret string,
) *WebhookController {
	return &WebhookController{repos: repos, deploymentService: deploymentService, secret: secret}
}

type githubPushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

// GithubPush handles GitHub push webhook events and triggers deploy for matching repos.
func (w *WebhookController) GithubPush(c *gin.Context) {
	if c.GetHeader("X-GitHub-Event") != "push" {
		c.Status(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cannot read body"})
		return
	}

	if w.secret != "" {
		if !verifyGithubSignature(body, w.secret, c.GetHeader("X-Hub-Signature-256")) {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid signature"})
			return
		}
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid payload"})
		return
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	repoURL := payload.Repository.CloneURL
	if repoURL == "" {
		repoURL = payload.Repository.HTMLURL
	}

	repos, err := w.repos.FindRepositoriesByRepoURL(repoURL)
	if err != nil || len(repos) == 0 {
		c.JSON(http.StatusOK, gin.H{"deployed": 0})
		return
	}

	deployed := 0
	for _, repo := range repos {
		if repo.Branch != branch {
			continue
		}
		go func(userID, repoID string) {
			if _, err := w.deploymentService.Deploy(context.Background(), userID, repoID); err != nil {
				log.Printf("webhook deploy %s: %v", repoID, err)
			} else {
				log.Printf("webhook deploy %s: success", repoID)
			}
		}(repo.UserID, repo.ID)
		deployed++
	}

	c.JSON(http.StatusOK, gin.H{"deployed": deployed})
}

// DeployByToken handles deploy trigger via deploy token (no auth required).
// Token is read from "token" query param or "X-Deploy-Token" header.
func (w *WebhookController) DeployByToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("X-Deploy-Token")
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "deploy token required"})
		return
	}

	repo, err := w.repos.FindRepositoryByDeployToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid deploy token"})
		return
	}

	go func() {
		if _, err := w.deploymentService.Deploy(context.Background(), repo.UserID, repo.ID); err != nil {
			log.Printf("token deploy %s: %v", repo.ID, err)
		} else {
			log.Printf("token deploy %s: success", repo.ID)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"status": "deploy triggered", "repository_id": repo.ID})
}

func verifyGithubSignature(body []byte, secret, sigHeader string) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sigHeader))
}
