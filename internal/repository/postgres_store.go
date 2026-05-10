package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"repository-contorller-service-go/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id                  TEXT PRIMARY KEY,
	name                TEXT NOT NULL,
	email               TEXT NOT NULL UNIQUE,
	password_hash       TEXT NOT NULL,
	github_access_token TEXT NOT NULL DEFAULT '',
	created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS repositories (
	id           TEXT PRIMARY KEY,
	user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	provider     TEXT NOT NULL DEFAULT 'github',
	repo_url     TEXT NOT NULL,
	branch       TEXT NOT NULL DEFAULT 'main',
	domain       TEXT NOT NULL DEFAULT '',
	image_name   TEXT NOT NULL DEFAULT '',
	app_type     TEXT NOT NULL DEFAULT 'auto',
	node_version TEXT NOT NULL DEFAULT '',
	description  TEXT NOT NULL DEFAULT '',
	deploy_token TEXT NOT NULL DEFAULT '',
	created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE repositories ADD COLUMN IF NOT EXISTS deploy_token TEXT NOT NULL DEFAULT '';
`

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// ── UserRepository ────────────────────────────────────────────────────────────

func (s *PostgresStore) CreateUser(user model.User) error {
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO users (id, name, email, password_hash, github_access_token, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		user.ID, user.Name, strings.ToLower(user.Email),
		user.PasswordHash, user.GitHubAccessToken, user.CreatedAt,
	)
	if err != nil && strings.Contains(err.Error(), "unique") {
		return ErrConflict
	}
	return err
}

func (s *PostgresStore) GetUserByEmail(email string) (model.User, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, email, password_hash, github_access_token, created_at
		 FROM users WHERE lower(email) = lower($1)`, email)
	return scanUser(row)
}

func (s *PostgresStore) GetUserByID(id string) (model.User, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, name, email, password_hash, github_access_token, created_at
		 FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (s *PostgresStore) UpdateUserGitHubAccessToken(userID, accessToken string) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE users SET github_access_token = $1 WHERE id = $2`,
		strings.TrimSpace(accessToken), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanUser(row pgx.Row) (model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.GitHubAccessToken, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	return u, err
}

// ── ClientRepositoryRepository ────────────────────────────────────────────────

func (s *PostgresStore) CreateRepository(repo model.ClientRepository) error {
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO repositories
		 (id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		repo.ID, repo.UserID, repo.Name, repo.Provider, repo.RepoURL,
		repo.Branch, repo.Domain, repo.ImageName, repo.AppType,
		repo.NodeVersion, repo.Description, repo.DeployToken, repo.CreatedAt, repo.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) ListAllRepositories() ([]model.ClientRepository, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at
		 FROM repositories ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []model.ClientRepository
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	if repos == nil {
		repos = []model.ClientRepository{}
	}
	return repos, rows.Err()
}

func (s *PostgresStore) ListRepositoriesByUser(userID string) ([]model.ClientRepository, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at
		 FROM repositories WHERE user_id = $1 ORDER BY updated_at DESC, name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.ClientRepository
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	if repos == nil {
		repos = []model.ClientRepository{}
	}
	return repos, rows.Err()
}

func (s *PostgresStore) GetRepositoryByID(userID, repoID string) (model.ClientRepository, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at
		 FROM repositories WHERE id = $1 AND user_id = $2`, repoID, userID)
	r, err := scanRepo(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClientRepository{}, ErrNotFound
	}
	return r, err
}

func (s *PostgresStore) UpdateRepository(repo model.ClientRepository) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE repositories SET
		 name=$1, provider=$2, repo_url=$3, branch=$4, domain=$5,
		 image_name=$6, app_type=$7, node_version=$8, description=$9, updated_at=$10
		 WHERE id=$11 AND user_id=$12`,
		repo.Name, repo.Provider, repo.RepoURL, repo.Branch, repo.Domain,
		repo.ImageName, repo.AppType, repo.NodeVersion, repo.Description,
		repo.UpdatedAt, repo.ID, repo.UserID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteRepository(userID, repoID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM repositories WHERE id = $1 AND user_id = $2`, repoID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) FindRepositoriesByRepoURL(repoURL string) ([]model.ClientRepository, error) {
	normalized := strings.TrimSuffix(strings.ToLower(repoURL), ".git")
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at
		 FROM repositories WHERE lower(rtrim(repo_url, '.git')) = $1`, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]model.ClientRepository, 0)
	for rows.Next() {
		repo, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, repo)
	}
	return result, rows.Err()
}

func (s *PostgresStore) GetRepositoryImageName(userID, repoID string) (string, error) {
	var imageName string
	err := s.pool.QueryRow(context.Background(),
		`SELECT image_name FROM repositories WHERE id = $1 AND user_id = $2`, repoID, userID,
	).Scan(&imageName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return imageName, err
}

func (s *PostgresStore) FindRepositoryByDeployToken(token string) (model.ClientRepository, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, user_id, name, provider, repo_url, branch, domain, image_name, app_type, node_version, description, deploy_token, created_at, updated_at
		 FROM repositories WHERE deploy_token = $1`, token)
	r, err := scanRepo(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ClientRepository{}, ErrNotFound
	}
	return r, err
}

func scanRepo(row interface {
	Scan(...any) error
}) (model.ClientRepository, error) {
	var r model.ClientRepository
	var createdAt, updatedAt time.Time
	err := row.Scan(
		&r.ID, &r.UserID, &r.Name, &r.Provider, &r.RepoURL,
		&r.Branch, &r.Domain, &r.ImageName, &r.AppType,
		&r.NodeVersion, &r.Description, &r.DeployToken, &createdAt, &updatedAt,
	)
	r.CreatedAt = createdAt
	r.UpdatedAt = updatedAt
	return r, err
}
