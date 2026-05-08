package service

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"repository-contorller-service-go/internal/model"
	"repository-contorller-service-go/internal/repository"
)

type RepositoryService struct {
	repos repository.ClientRepositoryRepository
}

type SaveRepositoryInput struct {
	Name        string
	Provider    string
	RepoURL     string
	Branch      string
	Domain      string
	ImageName   string
	AppType     string
	NodeVersion string
	Description string
}

func NewRepositoryService(repos repository.ClientRepositoryRepository) *RepositoryService {
	return &RepositoryService{repos: repos}
}

func (s *RepositoryService) Create(userID string, input SaveRepositoryInput) (model.ClientRepository, error) {
	now := time.Now().UTC()
	repo, err := buildRepositoryModel("", userID, input, now, now)
	if err != nil {
		return model.ClientRepository{}, err
	}

	if err := s.repos.CreateRepository(repo); err != nil {
		return model.ClientRepository{}, err
	}

	return repo, nil
}

func (s *RepositoryService) List(userID string) ([]model.ClientRepository, error) {
	return s.repos.ListRepositoriesByUser(userID)
}

func (s *RepositoryService) Get(userID, repoID string) (model.ClientRepository, error) {
	return s.repos.GetRepositoryByID(userID, repoID)
}

func (s *RepositoryService) Update(userID, repoID string, input SaveRepositoryInput) (model.ClientRepository, error) {
	existing, err := s.repos.GetRepositoryByID(userID, repoID)
	if err != nil {
		return model.ClientRepository{}, err
	}

	repo, err := buildRepositoryModel(existing.ID, userID, input, existing.CreatedAt, time.Now().UTC())
	if err != nil {
		return model.ClientRepository{}, err
	}

	if err := s.repos.UpdateRepository(repo); err != nil {
		return model.ClientRepository{}, err
	}

	return repo, nil
}

func (s *RepositoryService) Delete(userID, repoID string) error {
	return s.repos.DeleteRepository(userID, repoID)
}

func buildRepositoryModel(id, userID string, input SaveRepositoryInput, createdAt, updatedAt time.Time) (model.ClientRepository, error) {
	name := strings.TrimSpace(input.Name)
	provider := strings.TrimSpace(input.Provider)
	repoURL := strings.TrimSpace(input.RepoURL)
	branch := strings.TrimSpace(input.Branch)
	domain := strings.TrimSpace(input.Domain)
	imageName := strings.TrimSpace(input.ImageName)
	appType := normalizeAppType(input.AppType)
	nodeVersion := strings.TrimSpace(input.NodeVersion)
	description := strings.TrimSpace(input.Description)

	if name == "" || repoURL == "" {
		return model.ClientRepository{}, errors.New("name and repo_url are required")
	}
	if branch == "" {
		branch = "main"
	}
	if provider == "" {
		provider = "github"
	}
	if imageName == "" {
		imageName = slugify(name)
	}
	if appType == "" {
		appType = "auto"
	}
	if err := validateURL(repoURL); err != nil {
		return model.ClientRepository{}, err
	}
	if err := validateAppType(appType); err != nil {
		return model.ClientRepository{}, err
	}

	repo := model.ClientRepository{
		ID:          id,
		UserID:      userID,
		Name:        name,
		Provider:    strings.ToLower(provider),
		RepoURL:     repoURL,
		Branch:      branch,
		Domain:      strings.ToLower(domain),
		ImageName:   imageName,
		AppType:     appType,
		NodeVersion: nodeVersion,
		Description: description,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
	if repo.ID == "" {
		repo.ID = newID("repo")
	}

	return repo, nil
}

func validateURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("repo_url must be a valid URL")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("repo_url must be a valid URL")
	}
	return nil
}

func validateAppType(value string) error {
	switch value {
	case "auto", "static", "react", "vue", "angular", "nextjs":
		return nil
	default:
		return errors.New(`app_type must be one of: auto, static, react, vue, angular, nextjs`)
	}
}

func normalizeAppType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")

	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-':
			builder.WriteRune(r)
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "repository"
	}
	return result
}
