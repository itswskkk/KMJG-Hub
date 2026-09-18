package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// invalidTextRepresentation is PostgreSQL's SQLSTATE for input that cannot
// be parsed as the target column type (22P02) — here, a project id that
// isn't a valid UUID. Treated the same as "not found" rather than a 500,
// since a malformed id is no more accessible than a nonexistent one.
const invalidTextRepresentation = "22P02"

// ProjectRepository implements project.Repository against PostgreSQL.
type ProjectRepository struct {
	pool *pgxpool.Pool
}

// NewProjectRepository constructs a ProjectRepository backed by pool.
func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

func (r *ProjectRepository) CreateWithOwner(ctx context.Context, p *project.Project, ownerID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	err = tx.QueryRow(ctx, `
		INSERT INTO projects (name, description, created_by)
		VALUES ($1, NULLIF($2, ''), $3)
		RETURNING id, created_at
	`, p.Name, p.Description, ownerID).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, p.ID, ownerID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *ProjectRepository) ListForUser(ctx context.Context, userID string) ([]project.Summary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.name, COALESCE(p.description, ''), p.created_by, p.created_at, pm.role,
		       (SELECT COUNT(*) FROM project_members m WHERE m.project_id = p.id)
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $1
		ORDER BY p.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []project.Summary
	for rows.Next() {
		var s project.Summary
		var role string
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.CreatedBy, &s.CreatedAt, &role, &s.MemberCount); err != nil {
			return nil, err
		}
		s.ViewerRole = project.Role(role)
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func (r *ProjectRepository) GetDetailForUser(ctx context.Context, projectID, userID string) (*project.Detail, error) {
	var d project.Detail
	var role string
	err := r.pool.QueryRow(ctx, `
		SELECT p.id, p.name, COALESCE(p.description, ''), p.created_by, p.created_at, pm.role
		FROM projects p
		JOIN project_members pm ON pm.project_id = p.id AND pm.user_id = $2
		WHERE p.id = $1
	`, projectID, userID).Scan(&d.ID, &d.Name, &d.Description, &d.CreatedBy, &d.CreatedAt, &role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, project.ErrNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation {
			return nil, project.ErrNotFound
		}
		return nil, err
	}
	d.ViewerRole = project.Role(role)

	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.username, pm.role, pm.joined_at
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1
		ORDER BY pm.joined_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m project.Member
		var memberRole string
		if err := rows.Scan(&m.UserID, &m.Username, &memberRole, &m.JoinedAt); err != nil {
			return nil, err
		}
		m.Role = project.Role(memberRole)
		d.Members = append(d.Members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &d, nil
}

// ListMemberUserIDs is only ever called by the presence system with a
// projectID it already read from this same repository (see
// project.Service.ProjectIDsForUser), never with Client-supplied input, so
// unlike GetDetailForUser it does not need its own malformed-UUID handling.
func (r *ProjectRepository) ListMemberUserIDs(ctx context.Context, projectID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM project_members WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
