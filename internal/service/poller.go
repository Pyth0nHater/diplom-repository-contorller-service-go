package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"repository-contorller-service-go/internal/model"
	"repository-contorller-service-go/internal/repository"
)

type Poller struct {
	repos      repository.ClientRepositoryRepository
	users      repository.UserRepository
	deployment *DeploymentService
	interval   time.Duration

	mu   sync.Mutex
	shas map[string]string // repoID -> last deployed SHA
}

func NewPoller(
	repos repository.ClientRepositoryRepository,
	users repository.UserRepository,
	deployment *DeploymentService,
	interval time.Duration,
) *Poller {
	return &Poller{
		repos:      repos,
		users:      users,
		deployment: deployment,
		interval:   interval,
		shas:       make(map[string]string),
	}
}

func (p *Poller) Start(ctx context.Context) {
	log.Printf("poller: started, interval=%s", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	repos, err := p.repos.ListAllRepositories()
	if err != nil {
		log.Printf("poller: list repos: %v", err)
		return
	}
	for _, repo := range repos {
		p.checkRepo(ctx, repo)
	}
}

func (p *Poller) checkRepo(ctx context.Context, repo model.ClientRepository) {
	user, err := p.users.GetUserByID(repo.UserID)
	if err != nil || user.GitHubAccessToken == "" {
		return
	}

	sha, err := fetchLatestSHA(ctx, user.GitHubAccessToken, repo.RepoURL, repo.Branch)
	if err != nil {
		log.Printf("poller: fetch SHA %s: %v", repo.Name, err)
		return
	}

	p.mu.Lock()
	prev := p.shas[repo.ID]
	p.mu.Unlock()

	if prev == sha {
		return
	}

	log.Printf("poller: new commit on %s (%s → %s), deploying", repo.Name, prev[:min(7, len(prev))], sha[:min(7, len(sha))])

	if _, err := p.deployment.Deploy(ctx, repo.UserID, repo.ID); err != nil {
		log.Printf("poller: deploy %s: %v", repo.Name, err)
		return
	}

	p.mu.Lock()
	p.shas[repo.ID] = sha
	p.mu.Unlock()

	log.Printf("poller: deployed %s successfully", repo.Name)
}

func fetchLatestSHA(ctx context.Context, token, repoURL, branch string) (string, error) {
	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repo, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API %s", resp.Status)
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.SHA, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
