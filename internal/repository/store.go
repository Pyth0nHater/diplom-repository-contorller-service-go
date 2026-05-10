package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"repository-contorller-service-go/internal/model"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type UserRepository interface {
	CreateUser(user model.User) error
	GetUserByEmail(email string) (model.User, error)
	GetUserByID(id string) (model.User, error)
	UpdateUserGitHubAccessToken(userID, accessToken string) error
}

type ClientRepositoryRepository interface {
	CreateRepository(repo model.ClientRepository) error
	ListRepositoriesByUser(userID string) ([]model.ClientRepository, error)
	ListAllRepositories() ([]model.ClientRepository, error)
	GetRepositoryByID(userID, repoID string) (model.ClientRepository, error)
	UpdateRepository(repo model.ClientRepository) error
	DeleteRepository(userID, repoID string) error
	FindRepositoriesByRepoURL(repoURL string) ([]model.ClientRepository, error)
	FindRepositoryByDeployToken(token string) (model.ClientRepository, error)
}

type FileStore struct {
	path string
	mu   sync.RWMutex
	data storeData
}

type storeData struct {
	Users        []model.User             `json:"users"`
	Repositories []model.ClientRepository `json:"repositories"`
}

func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path: path,
		data: storeData{
			Users:        []model.User{},
			Repositories: []model.ClientRepository{},
		},
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *FileStore) CreateUser(user model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.data.Users {
		if strings.EqualFold(existing.Email, user.Email) {
			return ErrConflict
		}
	}

	s.data.Users = append(s.data.Users, user)
	return s.saveLocked()
}

func (s *FileStore) GetUserByEmail(email string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.data.Users {
		if strings.EqualFold(user.Email, email) {
			return user, nil
		}
	}

	return model.User{}, ErrNotFound
}

func (s *FileStore) GetUserByID(id string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.data.Users {
		if user.ID == id {
			return user, nil
		}
	}

	return model.User{}, ErrNotFound
}

func (s *FileStore) UpdateUserGitHubAccessToken(userID, accessToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Users {
		if s.data.Users[i].ID == userID {
			s.data.Users[i].GitHubAccessToken = strings.TrimSpace(accessToken)
			return s.saveLocked()
		}
	}

	return ErrNotFound
}

func (s *FileStore) CreateRepository(repo model.ClientRepository) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Repositories = append(s.data.Repositories, repo)
	return s.saveLocked()
}

func (s *FileStore) ListAllRepositories() ([]model.ClientRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]model.ClientRepository, len(s.data.Repositories))
	copy(result, s.data.Repositories)
	return result, nil
}

func (s *FileStore) ListRepositoriesByUser(userID string) ([]model.ClientRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repos := make([]model.ClientRepository, 0)
	for _, repo := range s.data.Repositories {
		if repo.UserID == userID {
			repos = append(repos, repo)
		}
	}

	slices.SortFunc(repos, func(a, b model.ClientRepository) int {
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			return strings.Compare(a.Name, b.Name)
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		return 1
	})

	return repos, nil
}

func (s *FileStore) GetRepositoryByID(userID, repoID string) (model.ClientRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, repo := range s.data.Repositories {
		if repo.ID == repoID && repo.UserID == userID {
			return repo, nil
		}
	}

	return model.ClientRepository{}, ErrNotFound
}

func (s *FileStore) UpdateRepository(repo model.ClientRepository) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Repositories {
		if s.data.Repositories[i].ID == repo.ID && s.data.Repositories[i].UserID == repo.UserID {
			s.data.Repositories[i] = repo
			return s.saveLocked()
		}
	}

	return ErrNotFound
}

func (s *FileStore) FindRepositoryByDeployToken(token string) (model.ClientRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, repo := range s.data.Repositories {
		if repo.DeployToken == token {
			return repo, nil
		}
	}
	return model.ClientRepository{}, ErrNotFound
}

func (s *FileStore) FindRepositoriesByRepoURL(repoURL string) ([]model.ClientRepository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := strings.TrimSuffix(strings.ToLower(repoURL), ".git")
	result := make([]model.ClientRepository, 0)
	for _, repo := range s.data.Repositories {
		if strings.TrimSuffix(strings.ToLower(repo.RepoURL), ".git") == normalized {
			result = append(result, repo)
		}
	}
	return result, nil
}

func (s *FileStore) DeleteRepository(userID, repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, repo := range s.data.Repositories {
		if repo.ID == repoID && repo.UserID == userID {
			s.data.Repositories = append(s.data.Repositories[:i], s.data.Repositories[i+1:]...)
			return s.saveLocked()
		}
	}

	return ErrNotFound
}

func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	content, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.saveLocked()
		}
		return err
	}

	if len(content) == 0 {
		return nil
	}

	return json.Unmarshal(content, &s.data)
}

func (s *FileStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	content, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, s.path)
}
