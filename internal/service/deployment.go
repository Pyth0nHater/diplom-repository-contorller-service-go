package service

import (
	"context"
	"io"
	"time"

	"repository-contorller-service-go/internal/repository"

	"deploy-service/pkg/deploypb"

	"google.golang.org/grpc"
)

type DeploymentService struct {
	repos   repository.ClientRepositoryRepository
	users   repository.UserRepository
	client  deploypb.DeployServiceClient
	timeout time.Duration
}

type DeploymentResult struct {
	RepositoryID string
	Events       []DeploymentEvent
}

// EventCallback is invoked for each streaming event. Returning an error stops the stream.
type EventCallback func(DeploymentEvent) error

type DeploymentEvent struct {
	Level         string
	Stage         string
	Message       string
	ImageName     string
	ContainerName string
	Domain        string
}

func NewDeploymentService(repos repository.ClientRepositoryRepository, users repository.UserRepository, client deploypb.DeployServiceClient, timeout time.Duration) *DeploymentService {
	return &DeploymentService{
		repos:   repos,
		users:   users,
		client:  client,
		timeout: timeout,
	}
}

func (s *DeploymentService) Deploy(ctx context.Context, userID, repoID string) (DeploymentResult, error) {
	return s.run(ctx, userID, repoID, deploymentOperationDeploy)
}

func (s *DeploymentService) BootstrapRepository(ctx context.Context, userID, repoID string) (DeploymentResult, error) {
	return s.run(ctx, userID, repoID, deploymentOperationBootstrap)
}

// PrepareDeployStream validates prerequisites, establishes the gRPC stream, and returns
// a streamFn that drives the stream by calling onEvent for each received event.
// If preparation fails, a non-nil error is returned and streamFn is nil.
func (s *DeploymentService) PrepareDeployStream(ctx context.Context, userID, repoID string) (string, func(EventCallback) error, context.CancelFunc, error) {
	return s.prepareStream(ctx, userID, repoID, deploymentOperationDeploy)
}

// PrepareBootstrapStream is like PrepareDeployStream but for the bootstrap operation.
func (s *DeploymentService) PrepareBootstrapStream(ctx context.Context, userID, repoID string) (string, func(EventCallback) error, context.CancelFunc, error) {
	return s.prepareStream(ctx, userID, repoID, deploymentOperationBootstrap)
}

func (s *DeploymentService) prepareStream(ctx context.Context, userID, repoID string, operation deploymentOperation) (string, func(EventCallback) error, context.CancelFunc, error) {
	repo, err := s.repos.GetRepositoryByID(userID, repoID)
	if err != nil {
		return "", nil, nil, err
	}

	user, err := s.users.GetUserByID(userID)
	if err != nil {
		return repo.ID, nil, nil, err
	}
	if user.GitHubAccessToken == "" {
		return repo.ID, nil, nil, ErrGitHubTokenNotConfigured
	}

	requestCtx, cancel := context.WithTimeout(ctx, s.timeout)

	var grpcStream grpc.ServerStreamingClient[deploypb.DeployEvent]
	switch operation {
	case deploymentOperationBootstrap:
		grpcStream, err = s.client.BootstrapRepository(requestCtx, &deploypb.BootstrapRepositoryRequest{
			RepoUrl:     repo.RepoURL,
			AccessToken: user.GitHubAccessToken,
			ImageName:   repo.ImageName,
			Domain:      repo.Domain,
			Branch:      repo.Branch,
			AppType:     repo.AppType,
			NodeVersion: repo.NodeVersion,
		})
	default:
		grpcStream, err = s.client.Deploy(requestCtx, &deploypb.DeployRequest{
			RepoUrl:     repo.RepoURL,
			AccessToken: user.GitHubAccessToken,
			ImageName:   repo.ImageName,
			Domain:      repo.Domain,
			Branch:      repo.Branch,
			AppType:     repo.AppType,
			NodeVersion: repo.NodeVersion,
		})
	}
	if err != nil {
		cancel()
		return repo.ID, nil, nil, err
	}

	streamFn := func(onEvent EventCallback) error {
		for {
			event, recvErr := grpcStream.Recv()
			if recvErr == io.EOF {
				return nil
			}
			if recvErr != nil {
				return recvErr
			}
			if cbErr := onEvent(DeploymentEvent{
				Level:         toDeploymentEventLevel(event.GetLevel()),
				Stage:         event.GetStage(),
				Message:       event.GetMessage(),
				ImageName:     event.GetImageName(),
				ContainerName: event.GetContainerName(),
				Domain:        event.GetDomain(),
			}); cbErr != nil {
				return cbErr
			}
		}
	}

	return repo.ID, streamFn, cancel, nil
}

type deploymentOperation string

const (
	deploymentOperationDeploy    deploymentOperation = "deploy"
	deploymentOperationBootstrap deploymentOperation = "bootstrap"
)

func (s *DeploymentService) run(ctx context.Context, userID, repoID string, operation deploymentOperation) (DeploymentResult, error) {
	repo, err := s.repos.GetRepositoryByID(userID, repoID)
	if err != nil {
		return DeploymentResult{}, err
	}

	user, err := s.users.GetUserByID(userID)
	if err != nil {
		return DeploymentResult{}, err
	}
	if user.GitHubAccessToken == "" {
		return DeploymentResult{RepositoryID: repo.ID}, ErrGitHubTokenNotConfigured
	}

	requestCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var stream grpc.ServerStreamingClient[deploypb.DeployEvent]
	switch operation {
	case deploymentOperationBootstrap:
		stream, err = s.client.BootstrapRepository(requestCtx, &deploypb.BootstrapRepositoryRequest{
			RepoUrl:     repo.RepoURL,
			AccessToken: user.GitHubAccessToken,
			ImageName:   repo.ImageName,
			Domain:      repo.Domain,
			Branch:      repo.Branch,
			AppType:     repo.AppType,
			NodeVersion: repo.NodeVersion,
		})
	default:
		stream, err = s.client.Deploy(requestCtx, &deploypb.DeployRequest{
			RepoUrl:     repo.RepoURL,
			AccessToken: user.GitHubAccessToken,
			ImageName:   repo.ImageName,
			Domain:      repo.Domain,
			Branch:      repo.Branch,
			AppType:     repo.AppType,
			NodeVersion: repo.NodeVersion,
		})
	}
	if err != nil {
		return DeploymentResult{RepositoryID: repo.ID}, err
	}

	events, err := collectDeploymentEvents(stream)
	return DeploymentResult{
		RepositoryID: repo.ID,
		Events:       events,
	}, err
}

func collectDeploymentEvents(stream grpc.ServerStreamingClient[deploypb.DeployEvent]) ([]DeploymentEvent, error) {
	events := make([]DeploymentEvent, 0)
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return events, err
		}

		events = append(events, DeploymentEvent{
			Level:         toDeploymentEventLevel(event.GetLevel()),
			Stage:         event.GetStage(),
			Message:       event.GetMessage(),
			ImageName:     event.GetImageName(),
			ContainerName: event.GetContainerName(),
			Domain:        event.GetDomain(),
		})
	}
}

func toDeploymentEventLevel(level deploypb.DeployEvent_Level) string {
	switch level {
	case deploypb.DeployEvent_LEVEL_SUCCESS:
		return "success"
	case deploypb.DeployEvent_LEVEL_ERROR:
		return "error"
	default:
		return "info"
	}
}
