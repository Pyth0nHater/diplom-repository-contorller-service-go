package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"repository-contorller-service-go/internal/middleware"
	"repository-contorller-service-go/internal/repository"
	"repository-contorller-service-go/internal/service"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type DeploymentController struct {
	deploymentService *service.DeploymentService
}

// DeploymentEventResponse is one NDJSON line sent to the client.
type DeploymentEventResponse struct {
	Level         string `json:"level"`
	Stage         string `json:"stage"`
	Message       string `json:"message"`
	ImageName     string `json:"image_name,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Domain        string `json:"domain,omitempty"`
	Operation     string `json:"operation,omitempty"`
	RepositoryID  string `json:"repository_id,omitempty"`
}

func NewDeploymentController(deploymentService *service.DeploymentService) *DeploymentController {
	return &DeploymentController{deploymentService: deploymentService}
}

// Deploy godoc
// @Summary Deploy repository
// @Description Stream deployment events as NDJSON (application/x-ndjson). Each line is a DeploymentEventResponse JSON object.
// @Tags deployments
// @Security BearerAuth
// @Produce application/x-ndjson
// @Param id path string true "Repository ID"
// @Success 200 {object} DeploymentEventResponse
// @Failure 400 {object} DeploymentEventResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /repositories/{id}/deploy [post]
func (d *DeploymentController) Deploy(c *gin.Context) {
	d.run(c, "deploy")
}

// BootstrapRepository godoc
// @Summary Bootstrap repository
// @Description Stream bootstrap events as NDJSON (application/x-ndjson). Each line is a DeploymentEventResponse JSON object.
// @Tags deployments
// @Security BearerAuth
// @Produce application/x-ndjson
// @Param id path string true "Repository ID"
// @Success 200 {object} DeploymentEventResponse
// @Failure 400 {object} DeploymentEventResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /repositories/{id}/bootstrap [post]
func (d *DeploymentController) BootstrapRepository(c *gin.Context) {
	d.run(c, "bootstrap")
}

func (d *DeploymentController) run(c *gin.Context, operation string) {
	userID := c.GetString(middleware.UserIDContextKey)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "unauthorized"})
		return
	}

	repoIDParam := c.Param("id")

	var (
		repoID   string
		streamFn func(service.EventCallback) error
		cancel   func()
		err      error
	)

	switch operation {
	case "bootstrap":
		repoID, streamFn, cancel, err = d.deploymentService.PrepareBootstrapStream(c.Request.Context(), userID, repoIDParam)
	default:
		repoID, streamFn, cancel, err = d.deploymentService.PrepareDeployStream(c.Request.Context(), userID, repoIDParam)
	}

	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "repository not found"})
			return
		}
		statusCode := http.StatusBadGateway
		if errors.Is(err, service.ErrGitHubTokenNotConfigured) {
			statusCode = http.StatusBadRequest
		} else {
			switch grpcstatus.Code(err) {
			case codes.InvalidArgument:
				statusCode = http.StatusBadRequest
			case codes.DeadlineExceeded:
				statusCode = http.StatusGatewayTimeout
			}
		}
		writeNDJSONError(c, statusCode, repoID, operation, err.Error())
		return
	}
	defer cancel()

	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(event service.DeploymentEvent) error {
		line, marshalErr := json.Marshal(DeploymentEventResponse{
			Level:         event.Level,
			Stage:         event.Stage,
			Message:       event.Message,
			ImageName:     event.ImageName,
			ContainerName: event.ContainerName,
			Domain:        event.Domain,
		})
		if marshalErr != nil {
			return marshalErr
		}
		if _, writeErr := fmt.Fprintf(c.Writer, "%s\n", line); writeErr != nil {
			return writeErr
		}
		c.Writer.Flush()
		return nil
	}

	if streamErr := streamFn(writeEvent); streamErr != nil {
		line, _ := json.Marshal(DeploymentEventResponse{
			Level:        "error",
			Stage:        "error",
			Message:      streamErr.Error(),
			RepositoryID: repoID,
			Operation:    operation,
		})
		fmt.Fprintf(c.Writer, "%s\n", line)
		c.Writer.Flush()
	}
}

func writeNDJSONError(c *gin.Context, status int, repoID, operation, message string) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Status(status)
	line, _ := json.Marshal(DeploymentEventResponse{
		Level:        "error",
		Stage:        "error",
		Message:      message,
		RepositoryID: repoID,
		Operation:    operation,
	})
	fmt.Fprintf(c.Writer, "%s\n", line)
}
