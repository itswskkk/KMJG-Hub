package postgres

import (
	"context"
	"errors"

	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkContextRepository struct{ pool *pgxpool.Pool }

func NewWorkContextRepository(pool *pgxpool.Pool) *WorkContextRepository {
	return &WorkContextRepository{pool: pool}
}

func (r *WorkContextRepository) Upsert(ctx context.Context, projectID, userID string, working bool, mode, branch string) (*workcontext.Context, error) {
	var value workcontext.Context
	err := r.pool.QueryRow(ctx, `
		INSERT INTO project_member_work_contexts(project_id,user_id,working,status_mode,current_branch)
		SELECT $1,$2,$3,$4,NULLIF($5,'')
		WHERE EXISTS (SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id WHERE pm.project_id=$1 AND pm.user_id=$2 AND p.deleted_at IS NULL)
		ON CONFLICT(project_id,user_id) DO UPDATE SET
			working=EXCLUDED.working,status_mode=EXCLUDED.status_mode,
			current_branch=EXCLUDED.current_branch,updated_at=now()
		RETURNING project_id,user_id,working,status_mode,COALESCE(current_branch,'')
	`, projectID, userID, working, mode, branch).Scan(&value.ProjectID, &value.UserID, &value.Working, &value.StatusMode, &value.CurrentBranch)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, workcontext.ErrNotFound
	}
	return &value, err
}

func (r *WorkContextRepository) List(ctx context.Context, projectID, viewerID string) ([]workcontext.Context, error) {
	var member bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members pm JOIN projects p ON p.id=pm.project_id WHERE pm.project_id=$1 AND pm.user_id=$2 AND p.deleted_at IS NULL)`, projectID, viewerID).Scan(&member)
	if err != nil || !member {
		if err == nil || isInvalidUUID(err) {
			return nil, workcontext.ErrNotFound
		}
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT project_id,user_id,working,status_mode,COALESCE(current_branch,'') FROM project_member_work_contexts WHERE project_id=$1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []workcontext.Context{}
	for rows.Next() {
		var v workcontext.Context
		if err := rows.Scan(&v.ProjectID, &v.UserID, &v.Working, &v.StatusMode, &v.CurrentBranch); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
