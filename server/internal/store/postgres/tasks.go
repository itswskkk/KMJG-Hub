package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/task"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TaskRepository struct{ pool *pgxpool.Pool }

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository { return &TaskRepository{pool} }

func scanTask(row pgx.Row) (*task.Task, error) {
	var t task.Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.CreatorID, &t.CreatorUsername, &t.AssigneeID, &t.AssigneeUsername, &t.DueDate, &t.CreatedAt, &t.UpdatedAt)
	return &t, err
}

const taskFields = `t.id, t.project_id, t.title, t.description, t.status, t.creator_user_id, creator.username, t.assignee_user_id, assignee.username, t.due_date, t.created_at, t.updated_at`

func (r *TaskRepository) List(ctx context.Context, projectID, viewerID string) ([]task.Task, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+taskFields+` FROM project_tasks t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id WHERE t.project_id=$1 AND EXISTS (SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2) ORDER BY CASE t.status WHEN 'todo' THEN 1 WHEN 'in_progress' THEN 2 ELSE 3 END, t.created_at, t.id`, projectID, viewerID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, task.ErrNotFound
		}
		return nil, err
	}
	defer rows.Close()
	items := []task.Task{}
	for rows.Next() {
		t, e := scanTask(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, *t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// An empty result cannot distinguish an empty board from no access.
	var member bool
	err = r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, viewerID).Scan(&member)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, task.ErrNotFound
	}
	return items, nil
}
func (r *TaskRepository) Get(ctx context.Context, projectID, taskID, viewerID string) (*task.Task, error) {
	t, err := scanTask(r.pool.QueryRow(ctx, `SELECT `+taskFields+` FROM project_tasks t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id WHERE t.project_id=$1 AND t.id=$2 AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3)`, projectID, taskID, viewerID))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return t, err
}
func (r *TaskRepository) Create(ctx context.Context, projectID, creatorID, title, description string, dueDate *time.Time) (*task.Task, error) {
	t, err := scanTask(r.pool.QueryRow(ctx, `WITH inserted AS (INSERT INTO project_tasks(project_id,creator_user_id,title,description,due_date) SELECT $1,$2,$3,$4,$5 WHERE EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2) RETURNING *) SELECT `+taskFields+` FROM inserted t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id`, projectID, creatorID, title, description, dueDate))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return t, err
}
func (r *TaskRepository) SetStatus(ctx context.Context, projectID, taskID, actorID string, status task.Status) (*task.Task, error) {
	t, err := scanTask(r.pool.QueryRow(ctx, `WITH updated AS (UPDATE project_tasks SET status=$4,updated_at=now() WHERE id=$2 AND project_id=$1 AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3) RETURNING *) SELECT `+taskFields+` FROM updated t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id`, projectID, taskID, actorID, status))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return t, err
}
func (r *TaskRepository) AssignSelf(ctx context.Context, projectID, taskID, userID string) (*task.Task, error) {
	t, err := scanTask(r.pool.QueryRow(ctx, `WITH updated AS (UPDATE project_tasks SET assignee_user_id=$3,updated_at=now() WHERE id=$2 AND project_id=$1 AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3) RETURNING *) SELECT `+taskFields+` FROM updated t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id`, projectID, taskID, userID))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return t, err
}
func (r *TaskRepository) RequestAssignment(ctx context.Context, projectID, taskID, requesterID, recipientID string) (*task.AssignmentRequest, error) {
	var x task.AssignmentRequest
	err := r.pool.QueryRow(ctx, `INSERT INTO task_assignment_requests(task_id,project_id,requester_user_id,recipient_user_id) SELECT $2,$1,$3,$4 WHERE EXISTS(SELECT 1 FROM project_tasks WHERE id=$2 AND project_id=$1) AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3) AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$4) RETURNING id,task_id,project_id,requester_user_id,recipient_user_id,status,created_at`, projectID, taskID, requesterID, recipientID).Scan(&x.ID, &x.TaskID, &x.ProjectID, &x.RequesterID, &x.RecipientID, &x.Status, &x.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return &x, err
}
func (r *TaskRepository) RespondAssignment(ctx context.Context, requestID, recipientID string, accept bool) (*task.Task, error) {
	status := "declined"
	if accept {
		status = "accepted"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var taskID, projectID string
	err = tx.QueryRow(ctx, `UPDATE task_assignment_requests SET status=$3,responded_at=now() WHERE id=$1 AND recipient_user_id=$2 AND status='pending' RETURNING task_id,project_id`, requestID, recipientID, status).Scan(&taskID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !accept {
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
		return r.Get(ctx, projectID, taskID, recipientID)
	}
	t, err := scanTask(tx.QueryRow(ctx, `WITH updated AS (UPDATE project_tasks SET assignee_user_id=$2,updated_at=now() WHERE id=$1 RETURNING *) SELECT `+taskFields+` FROM updated t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id`, taskID, recipientID))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}
func (r *TaskRepository) SetCurrent(ctx context.Context, projectID, taskID, userID string) (*task.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	t, err := scanTask(tx.QueryRow(ctx, `WITH updated AS (UPDATE project_tasks SET status=CASE WHEN status='todo' THEN 'in_progress' ELSE status END,updated_at=now() WHERE id=$2 AND project_id=$1 AND assignee_user_id=$3 RETURNING *) SELECT `+taskFields+` FROM updated t JOIN users creator ON creator.id=t.creator_user_id LEFT JOIN users assignee ON assignee.id=t.assignee_user_id`, projectID, taskID, userID))
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_current_tasks(user_id,task_id) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET task_id=EXCLUDED.task_id,updated_at=now()`, userID, taskID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}
func (r *TaskRepository) ListComments(ctx context.Context, projectID, taskID, viewerID string) ([]task.Comment, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id,c.task_id,c.author_user_id,u.username,c.body,c.created_at FROM project_task_comments c JOIN users u ON u.id=c.author_user_id WHERE c.task_id=$2 AND EXISTS(SELECT 1 FROM project_tasks WHERE id=$2 AND project_id=$1) AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3) ORDER BY c.created_at,c.id`, projectID, taskID, viewerID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, task.ErrNotFound
		}
		return nil, err
	}
	defer rows.Close()
	out := []task.Comment{}
	for rows.Next() {
		var c task.Comment
		if err = rows.Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.AuthorUsername, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if _, err = r.Get(ctx, projectID, taskID, viewerID); err != nil {
		return nil, err
	}
	return out, nil
}
func (r *TaskRepository) AddComment(ctx context.Context, projectID, taskID, authorID, body string) (*task.Comment, error) {
	var c task.Comment
	err := r.pool.QueryRow(ctx, `WITH inserted AS (INSERT INTO project_task_comments(task_id,author_user_id,body) SELECT $2,$3,$4 WHERE EXISTS(SELECT 1 FROM project_tasks WHERE id=$2 AND project_id=$1) AND EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$3) RETURNING *) SELECT c.id,c.task_id,c.author_user_id,u.username,c.body,c.created_at FROM inserted c JOIN users u ON u.id=c.author_user_id`, projectID, taskID, authorID, body).Scan(&c.ID, &c.TaskID, &c.AuthorID, &c.AuthorUsername, &c.Body, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, task.ErrNotFound
	}
	return &c, err
}

func (r *TaskRepository) ListPendingAssignments(ctx context.Context, recipientID string) ([]task.AssignmentRequest, error) {
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.task_id,r.project_id,r.requester_user_id,r.recipient_user_id,t.title,u.username,r.status,r.created_at FROM task_assignment_requests r JOIN project_tasks t ON t.id=r.task_id JOIN users u ON u.id=r.requester_user_id WHERE r.recipient_user_id=$1 AND r.status='pending' ORDER BY r.created_at,r.id`, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []task.AssignmentRequest{}
	for rows.Next() {
		var item task.AssignmentRequest
		if err := rows.Scan(&item.ID, &item.TaskID, &item.ProjectID, &item.RequesterID, &item.RecipientID, &item.TaskTitle, &item.RequesterUsername, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
