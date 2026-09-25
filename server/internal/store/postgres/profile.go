package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
)

// foreignKeyViolation is PostgreSQL's SQLSTATE for a foreign-key constraint
// violation (23503), used here to translate a privacy write for a
// nonexistent userID into the domain-level profile.ErrNotFound rather than
// a raw database error.
const foreignKeyViolation = "23503"

// onlineChecker is the narrow real-time capability ProfileRepository needs
// to populate Profile.Presence: whether userID currently has a live
// authenticated WebSocket connection. It exists as its own interface
// (rather than importing internal/realtime's concrete Hub type) purely to
// keep this package's dependency explicit and independently fakeable, the
// same way internal/workcontext.Service depends on an OnlineUsers
// interface instead of *realtime.Hub directly. *realtime.Hub satisfies
// this interface already (see internal/presence.Broadcaster).
type onlineChecker interface {
	IsOnline(userID string) bool
}

// ProfileRepository implements profile.Repository and profile.Membership
// against PostgreSQL.
type ProfileRepository struct {
	pool   *pgxpool.Pool
	online onlineChecker
}

// NewProfileRepository constructs a ProfileRepository backed by pool. online
// may be nil (e.g. in a context with no real-time Hub); Presence is then
// always reported "offline".
func NewProfileRepository(pool *pgxpool.Pool, online onlineChecker) *ProfileRepository {
	return &ProfileRepository{pool: pool, online: online}
}

// asContext recovers a context.Context from profile.Repository's `ctx any`
// parameters. Service always calls through with a real context.Context;
// context.Background() is only a defensive fallback so a malformed caller
// can't panic the Server.
func asContext(ctx any) context.Context {
	if c, ok := ctx.(context.Context); ok && c != nil {
		return c
	}
	return context.Background()
}

// GetProfile returns userID's full profile with no privacy filtering
// applied (Service.applyPrivacyFiltering does that afterwards). Live
// fields (current project/task/branch) are derived from the user's current
// task (internal/task's one-current-task-per-user model) and that task's
// Project work context; Presence comes from the live real-time Hub, not
// from PostgreSQL, since PostgreSQL has no notion of an open connection.
func (r *ProfileRepository) GetProfile(ctx any, userID string) (*profile.Profile, error) {
	c := asContext(ctx)

	var p profile.Profile
	var displayName, avatar, bio *string
	var currentProjectID, currentProjectName, currentTaskTitle, currentBranch *string
	var working *bool

	err := r.pool.QueryRow(c, `
		SELECT u.id, u.username, u.email, u.display_name, u.avatar, u.bio,
		       cp.id, cp.name, ct.title, wc.current_branch, wc.working
		FROM users u
		LEFT JOIN user_current_tasks uct ON uct.user_id = u.id
		LEFT JOIN project_tasks ct ON ct.id = uct.task_id
		LEFT JOIN projects cp ON cp.id = ct.project_id
		LEFT JOIN project_member_work_contexts wc
		       ON wc.project_id = ct.project_id AND wc.user_id = u.id
		WHERE u.id = $1
	`, userID).Scan(
		&p.UserID, &p.Username, &p.Email, &displayName, &avatar, &bio,
		&currentProjectID, &currentProjectName, &currentTaskTitle, &currentBranch, &working,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, profile.ErrNotFound
		}
		return nil, err
	}

	p.DisplayName = displayName
	p.Avatar = avatar
	p.Bio = bio
	p.CurrentProject = currentProjectName
	p.CurrentTask = currentTaskTitle
	// A branch is only meaningful while the user is actively working; a
	// stale branch from a previous work session should not be reported as
	// "current" once they've stopped.
	if working != nil && *working {
		p.CurrentBranch = currentBranch
	}
	// Repository connections are not modeled anywhere yet (no feature
	// writes them; GitHub/repository integration is a later checkpoint),
	// so there is nothing to populate here yet. See internal/project's
	// analogous comment about its own unmodeled Git repository columns.
	p.Repositories = []string{}

	presence := "offline"
	if r.online != nil && r.online.IsOnline(userID) {
		presence = "online"
	}
	p.Presence = &presence

	return &p, nil
}

// UpdateProfile updates only the fields whose pointer is non-nil;
// display_name/avatar/bio each independently distinguish "leave unchanged"
// (nil) from "set, possibly to empty to clear" (non-nil, per
// internal/profile/validate.go).
func (r *ProfileRepository) UpdateProfile(ctx any, userID string, displayName, avatar, bio *string) error {
	c := asContext(ctx)

	tag, err := r.pool.Exec(c, `
		UPDATE users SET
			display_name = CASE WHEN $2 THEN NULLIF($3, '') ELSE display_name END,
			avatar       = CASE WHEN $4 THEN NULLIF($5, '') ELSE avatar END,
			bio          = CASE WHEN $6 THEN NULLIF($7, '') ELSE bio END
		WHERE id = $1
	`, userID,
		displayName != nil, derefOrEmpty(displayName),
		avatar != nil, derefOrEmpty(avatar),
		bio != nil, derefOrEmpty(bio),
	)
	if err != nil {
		if isInvalidUUID(err) {
			return profile.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return profile.ErrNotFound
	}
	return nil
}

// GetPrivacy returns every field-level privacy row userID has explicitly
// set. A field absent from the returned map has no explicit override; the
// service layer falls back to its own default audience for it.
func (r *ProfileRepository) GetPrivacy(ctx any, userID string) (map[profile.Field]profile.Audience, error) {
	c := asContext(ctx)

	rows, err := r.pool.Query(c, `
		SELECT field, audience FROM profile_privacy WHERE user_id = $1
	`, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, profile.ErrNotFound
		}
		return nil, err
	}
	defer rows.Close()

	settings := make(map[profile.Field]profile.Audience)
	for rows.Next() {
		var field, audience string
		if err := rows.Scan(&field, &audience); err != nil {
			return nil, err
		}
		settings[profile.Field(field)] = profile.Audience(audience)
	}
	return settings, rows.Err()
}

// SetPrivacy upserts one field's audience for userID.
func (r *ProfileRepository) SetPrivacy(ctx any, userID string, field profile.Field, audience profile.Audience) error {
	c := asContext(ctx)

	_, err := r.pool.Exec(c, `
		INSERT INTO profile_privacy (user_id, field, audience)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, field) DO UPDATE SET
			audience = EXCLUDED.audience, updated_at = now()
	`, userID, string(field), string(audience))
	if err != nil {
		if isInvalidUUID(err) || isForeignKeyViolation(err) {
			return profile.ErrNotFound
		}
		return err
	}
	return nil
}

// defaultPrivacyRules mirrors profile.Service's own default-audience table
// (internal/profile/service.go's applyPrivacyFiltering), so a new user's
// privacy settings are materialized explicitly rather than only implied.
var defaultPrivacyRules = []struct {
	Field    profile.Field
	Audience profile.Audience
}{
	{profile.FieldDisplayName, profile.AudienceEveryone},
	{profile.FieldAvatar, profile.AudienceEveryone},
	{profile.FieldBio, profile.AudienceFriends},
	{profile.FieldCurrentProject, profile.AudienceEveryone},
	{profile.FieldCurrentTask, profile.AudienceFriends},
	{profile.FieldCurrentBranch, profile.AudienceFriends},
	{profile.FieldRepositories, profile.AudienceEveryone},
}

// SetDefaultPrivacy inserts the default audience for every profile field
// for a newly registered userID. It is idempotent (ON CONFLICT DO NOTHING)
// so it can never clobber a setting the user has already customized.
func (r *ProfileRepository) SetDefaultPrivacy(ctx any, userID string) error {
	c := asContext(ctx)

	batch := &pgx.Batch{}
	for _, rule := range defaultPrivacyRules {
		batch.Queue(`
			INSERT INTO profile_privacy (user_id, field, audience)
			VALUES ($1, $2, $3)
			ON CONFLICT (user_id, field) DO NOTHING
		`, userID, string(rule.Field), string(rule.Audience))
	}

	br := r.pool.SendBatch(c, batch)
	defer br.Close()
	for range defaultPrivacyRules {
		if _, err := br.Exec(); err != nil {
			if isInvalidUUID(err) || isForeignKeyViolation(err) {
				return profile.ErrNotFound
			}
			return err
		}
	}
	return nil
}

// AreFollowers returns whether aUserID and bUserID have a friend
// relationship. KMJG Hub does not yet have a friend system (Phase 2 of the
// roadmap — see IMPLEMENTATION-ROADMAP.md — introduces friend requests and
// friendships); until that schema exists there is nothing in PostgreSQL to
// query, so this conservatively reports "not friends" rather than querying
// a table that does not exist. Once Phase 2 lands its friendships table,
// this method is the one place that needs to change for profile privacy to
// pick it up automatically.
func (r *ProfileRepository) AreFollowers(ctx any, aUserID, bUserID string) (bool, error) {
	return false, nil
}

// ShareProject returns true if userID1 and userID2 are both current
// members of at least one Project. This is enforced entirely in SQL
// against project_members, the authoritative membership table, rather than
// on two separately-read membership lists that could race.
func (r *ProfileRepository) ShareProject(ctx any, userID1, userID2 string) (bool, error) {
	c := asContext(ctx)

	var shares bool
	err := r.pool.QueryRow(c, `
		SELECT EXISTS (
			SELECT 1 FROM project_members pm1
			JOIN project_members pm2 ON pm2.project_id = pm1.project_id
			WHERE pm1.user_id = $1 AND pm2.user_id = $2
		)
	`, userID1, userID2).Scan(&shares)
	if err != nil {
		if isInvalidUUID(err) {
			return false, nil
		}
		return false, err
	}
	return shares, nil
}

// IsBlocked returns whether blockerID has blocked userID. Like
// AreFollowers, KMJG Hub does not yet have a blocking system (also part of
// Phase 2's friend system), so this conservatively reports "not blocked"
// until that schema exists.
func (r *ProfileRepository) IsBlocked(ctx any, userID, blockerID string) (bool, error) {
	return false, nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
