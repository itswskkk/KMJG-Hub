package postgres

import (
	"context"
	"errors"
	"time"

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
		WHERE p.deleted_at IS NULL
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
		WHERE p.id = $1 AND p.deleted_at IS NULL
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
		SELECT u.id, u.username, pm.role, pm.joined_at, t.title
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		LEFT JOIN user_current_tasks ct ON ct.user_id = pm.user_id
		LEFT JOIN project_tasks t ON t.id = ct.task_id AND t.project_id = pm.project_id
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
		if err := rows.Scan(&m.UserID, &m.Username, &memberRole, &m.JoinedAt, &m.CurrentTaskTitle); err != nil {
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
// A soft-deleted Project has no members for real-time fan-out purposes.
func (r *ProjectRepository) ListMemberUserIDs(ctx context.Context, projectID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT pm.user_id FROM project_members pm
		JOIN projects p ON p.id = pm.project_id AND p.deleted_at IS NULL
		WHERE pm.project_id = $1
	`, projectID)
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

// RemoveMember enforces the Project role rules in the same SQL statement as
// the delete. This prevents an authorization decision made by Service from
// becoming stale if membership or roles change between its read and delete.
func (r *ProjectRepository) RemoveMember(ctx context.Context, projectID, actorUserID, targetUserID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM project_members AS target
		USING project_members AS actor
		WHERE target.project_id = $1
		  AND target.user_id = $2
		  AND actor.project_id = target.project_id
		  AND actor.user_id = $3
		  AND (
			(actor.role = 'owner' AND target.role IN ('admin', 'member'))
			OR (actor.role = 'admin' AND target.role = 'member')
		  )
		  AND EXISTS (SELECT 1 FROM projects p WHERE p.id = target.project_id AND p.deleted_at IS NULL)
	`, projectID, targetUserID, actorUserID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation {
			return project.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return project.ErrForbidden
	}
	return nil
}

// lockActiveProject row-locks projectID's projects row for the rest of tx,
// serializing every lifecycle change to one Project (transfer, role change,
// leave, delete, restore), and fails with project.ErrNotFound if the
// Project doesn't exist or is soft-deleted.
func lockActiveProject(ctx context.Context, tx pgx.Tx, projectID string) error {
	var one int
	err := tx.QueryRow(ctx, `SELECT 1 FROM projects WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, projectID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return project.ErrNotFound
	}
	return err
}

// memberRoleForUpdate returns userID's role in projectID, row-locking the
// membership, or project.ErrNotFound if userID isn't a member.
func memberRoleForUpdate(ctx context.Context, tx pgx.Tx, projectID, userID string) (project.Role, error) {
	var role string
	err := tx.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2 FOR UPDATE`, projectID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return "", project.ErrNotFound
	}
	return project.Role(role), err
}

// TransferOwnership runs in one transaction holding the Project row lock
// and both membership row locks, so the Project has exactly one Owner
// before and after (also guaranteed by the one-owner unique index from
// migration 0015; the demotion therefore runs before the promotion).
func (r *ProjectRepository) TransferOwnership(ctx context.Context, projectID, actorUserID, newOwnerID string, previousOwnerRole project.Role) error {
	if previousOwnerRole != project.RoleAdmin && previousOwnerRole != project.RoleMember {
		return project.ErrForbidden
	}
	if actorUserID == newOwnerID {
		return project.ErrCannotTransferToSelf
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockActiveProject(ctx, tx, projectID); err != nil {
		return err
	}
	actorRole, err := memberRoleForUpdate(ctx, tx, projectID, actorUserID)
	if errors.Is(err, project.ErrNotFound) {
		return project.ErrForbidden
	}
	if err != nil {
		return err
	}
	if actorRole != project.RoleOwner {
		return project.ErrForbidden
	}
	if _, err := memberRoleForUpdate(ctx, tx, projectID, newOwnerID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE project_members SET role = $3
		WHERE project_id = $1 AND user_id = $2 AND role = 'owner'
	`, projectID, actorUserID, string(previousOwnerRole)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE project_members SET role = 'owner'
		WHERE project_id = $1 AND user_id = $2
	`, projectID, newOwnerID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateMemberRole enforces "only the Owner changes roles, never the
// Owner's own role" in the same statement as the update.
func (r *ProjectRepository) UpdateMemberRole(ctx context.Context, projectID, actorUserID, targetUserID string, newRole project.Role) error {
	if newRole != project.RoleAdmin && newRole != project.RoleMember {
		return project.ErrForbidden
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE project_members AS target SET role = $4
		FROM project_members AS actor, projects AS p
		WHERE target.project_id = $1
		  AND target.user_id = $2
		  AND target.role IN ('admin', 'member')
		  AND actor.project_id = target.project_id
		  AND actor.user_id = $3
		  AND actor.role = 'owner'
		  AND p.id = target.project_id
		  AND p.deleted_at IS NULL
	`, projectID, targetUserID, actorUserID, string(newRole))
	if err != nil {
		if isInvalidUUID(err) {
			return project.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return project.ErrForbidden
	}
	return nil
}

func (r *ProjectRepository) Leave(ctx context.Context, projectID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockActiveProject(ctx, tx, projectID); err != nil {
		return err
	}
	role, err := memberRoleForUpdate(ctx, tx, projectID, userID)
	if err != nil {
		return err
	}
	if role == project.RoleOwner {
		return project.ErrOwnerCannotLeave
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Delete soft-deletes in one statement that also checks the actor is the
// current Owner.
func (r *ProjectRepository) Delete(ctx context.Context, projectID, actorUserID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE projects AS p
		SET deleted_at = now(), deleted_by_owner_user_id = $2
		WHERE p.id = $1
		  AND p.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM project_members pm
			WHERE pm.project_id = p.id AND pm.user_id = $2 AND pm.role = 'owner'
		  )
	`, projectID, actorUserID)
	if err != nil {
		if isInvalidUUID(err) {
			return project.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return project.ErrForbidden
	}
	return nil
}

// Restore un-deletes projectID for its deleting Owner and makes that user
// the Owner again, per docs/PRD.md "Project Recovery". Ownership can't
// normally move while a Project is deleted (every lifecycle write requires
// an active Project), so the ownership repair below is a safeguard that
// keeps "exactly one Owner" true regardless.
func (r *ProjectRepository) Restore(ctx context.Context, projectID, actorUserID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var deletedBy *string
	var deleted, withinWindow bool
	err = tx.QueryRow(ctx, `
		SELECT deleted_at IS NOT NULL,
		       deleted_by_owner_user_id::text,
		       COALESCE(deleted_at > now() - make_interval(secs => $2), false)
		FROM projects WHERE id = $1 FOR UPDATE
	`, projectID, project.RestoreWindow.Seconds()).Scan(&deleted, &deletedBy, &withinWindow)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return project.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !deleted || deletedBy == nil || *deletedBy != actorUserID {
		return project.ErrNotFound
	}
	if !withinWindow {
		return project.ErrRestoreWindowExpired
	}

	if _, err := tx.Exec(ctx, `
		UPDATE project_members SET role = 'admin'
		WHERE project_id = $1 AND role = 'owner' AND user_id <> $2
	`, projectID, actorUserID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, 'owner')
		ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'owner'
	`, projectID, actorUserID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET deleted_at = NULL, deleted_by_owner_user_id = NULL WHERE id = $1
	`, projectID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *ProjectRepository) ListDeleted(ctx context.Context, userID string) ([]project.DeletedSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.name, COALESCE(p.description, ''), p.created_by, p.created_at, p.deleted_at,
		       (SELECT COUNT(*) FROM project_members m WHERE m.project_id = p.id)
		FROM projects p
		WHERE p.deleted_by_owner_user_id = $1
		  AND p.deleted_at IS NOT NULL
		  AND p.deleted_at > now() - make_interval(secs => $2)
		ORDER BY p.deleted_at DESC
	`, userID, project.RestoreWindow.Seconds())
	if err != nil {
		if isInvalidUUID(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []project.DeletedSummary
	for rows.Next() {
		var d project.DeletedSummary
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.CreatedBy, &d.CreatedAt, &d.DeletedAt, &d.MemberCount); err != nil {
			return nil, err
		}
		d.RestoreDeadline = d.DeletedAt.Add(project.RestoreWindow)
		out = append(out, d)
	}
	return out, rows.Err()
}

// PurgeDeletedBefore hard-deletes expired soft-deleted Projects. Every
// table referencing projects(id) is ON DELETE CASCADE (migrations
// 0002-0007, 0012), so one DELETE removes all their KMJG Hub data. It
// never touches an associated external Git repository (docs/PRD.md).
func (r *ProjectRepository) PurgeDeletedBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the expired rows first so a concurrent Restore can't slip in
	// between collecting attachment IDs and deleting the Projects.
	var ids []string
	if err := func() error {
		rows, err := tx.Query(ctx, `
			SELECT id::text FROM projects
			WHERE deleted_at IS NOT NULL AND deleted_at < $1
			FOR UPDATE
		`, cutoff)
		if err != nil {
			return err
		}
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `SELECT storage_id FROM project_chat_attachments WHERE project_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	storageIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = ANY($1::uuid[])`, ids); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return storageIDs, nil
}
