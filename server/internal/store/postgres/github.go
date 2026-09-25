package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
)

// GitHubStore persists GitHub identities (columns on users) and Project
// repository connections (project_repositories). Access tokens arrive and
// leave already encrypted; this layer never sees plaintext tokens.
type GitHubStore struct {
	pool *pgxpool.Pool
}

var _ github.Store = (*GitHubStore)(nil)

func NewGitHubStore(pool *pgxpool.Pool) *GitHubStore {
	return &GitHubStore{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func (s *GitHubStore) SaveIdentity(ctx context.Context, userID string, githubUserID int64, login string, encryptedToken []byte) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET github_user_id = $2, github_login = $3,
		    github_access_token_encrypted = $4, github_connected_at = now()
		WHERE id = $1
	`, userID, githubUserID, login, encryptedToken)
	if err != nil {
		switch {
		case isUniqueViolation(err):
			return github.ErrIdentityInUse
		case isInvalidUUID(err):
			return github.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return github.ErrNotFound
	}
	return nil
}

func (s *GitHubStore) GetIdentity(ctx context.Context, userID string) (*github.Identity, error) {
	var id github.Identity
	var login *string
	var connectedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, github_login, github_connected_at
		FROM users WHERE id = $1 AND github_user_id IS NOT NULL
	`, userID).Scan(&id.UserID, &login, &connectedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, github.ErrNotConnected
		}
		return nil, err
	}
	id.GitHubLogin = derefOrEmpty(login)
	if connectedAt != nil {
		id.ConnectedAt = *connectedAt
	}
	return &id, nil
}

func (s *GitHubStore) GetEncryptedAccessToken(ctx context.Context, userID string) ([]byte, error) {
	var token []byte
	err := s.pool.QueryRow(ctx, `
		SELECT github_access_token_encrypted FROM users
		WHERE id = $1 AND github_user_id IS NOT NULL
	`, userID).Scan(&token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, github.ErrNotConnected
		}
		return nil, err
	}
	if len(token) == 0 {
		return nil, github.ErrNotConnected
	}
	return token, nil
}

func (s *GitHubStore) DisconnectIdentity(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET github_user_id = NULL, github_login = NULL,
		    github_access_token_encrypted = NULL, github_connected_at = NULL
		WHERE id = $1 AND github_user_id IS NOT NULL
	`, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return github.ErrNotConnected
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return github.ErrNotConnected
	}
	return nil
}

const repositoryColumns = `
	id, project_id, provider, external_repo_id, owner_login, repo_name, html_url,
	default_branch, COALESCE(connected_by_user_id::text, ''), connected_at,
	post_pushes_to_chat, notify_all_members, notify_all_branches`

func scanRepository(row pgx.Row) (*github.Repository, error) {
	var r github.Repository
	err := row.Scan(&r.ID, &r.ProjectID, &r.Provider, &r.ExternalID, &r.OwnerLogin, &r.Name, &r.HTMLURL,
		&r.DefaultBranch, &r.ConnectedByUserID, &r.ConnectedAt,
		&r.PostPushesToChat, &r.NotifyAllMembers, &r.NotifyAllBranches)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *GitHubStore) ConnectRepository(ctx context.Context, projectID, connectedByUserID string, repo github.AvailableRepository) (*github.Repository, error) {
	r, err := scanRepository(s.pool.QueryRow(ctx, `
		INSERT INTO project_repositories
			(project_id, provider, external_repo_id, owner_login, repo_name, html_url, default_branch, connected_by_user_id)
		VALUES ($1, 'github', $2, $3, $4, $5, $6, $7)
		RETURNING `+repositoryColumns,
		projectID, repo.ExternalID, repo.OwnerLogin, repo.Name, repo.HTMLURL, repo.DefaultBranch, connectedByUserID))
	if err != nil {
		switch {
		case isUniqueViolation(err):
			return nil, github.ErrAlreadyConnected
		case isInvalidUUID(err), isForeignKeyViolation(err):
			return nil, github.ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func (s *GitHubStore) GetRepository(ctx context.Context, projectID string) (*github.Repository, error) {
	r, err := scanRepository(s.pool.QueryRow(ctx,
		`SELECT `+repositoryColumns+` FROM project_repositories WHERE project_id = $1`, projectID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, github.ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

func (s *GitHubStore) ListRepositoriesByExternalID(ctx context.Context, externalID int64) ([]github.Repository, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+repositoryColumns+`
		FROM project_repositories
		WHERE provider = 'github' AND external_repo_id = $1`, externalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]github.Repository, 0)
	for rows.Next() {
		r, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *GitHubStore) DisconnectRepository(ctx context.Context, projectID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM project_repositories WHERE project_id = $1`, projectID)
	if err != nil {
		if isInvalidUUID(err) {
			return github.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return github.ErrNotFound
	}
	return nil
}

func (s *GitHubStore) UpdateNotificationConfig(ctx context.Context, projectID string, postToChat, allMembers, allBranches bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_repositories
		SET post_pushes_to_chat = $2, notify_all_members = $3, notify_all_branches = $4
		WHERE project_id = $1
	`, projectID, postToChat, allMembers, allBranches)
	if err != nil {
		if isInvalidUUID(err) {
			return github.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return github.ErrNotFound
	}
	return nil
}
